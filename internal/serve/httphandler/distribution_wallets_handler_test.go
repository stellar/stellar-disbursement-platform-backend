package httphandler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/db/dbtest"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/sdpcontext"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/middleware"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/services"
	svcMocks "github.com/stellar/stellar-disbursement-platform-backend/internal/services/mocks"
	sigMocks "github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine/signing/mocks"
	"github.com/stellar/stellar-disbursement-platform-backend/pkg/schema"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-auth/pkg/auth"
)

type mockDistributionWalletService struct {
	createFn          func(ctx context.Context, insert data.DistributionWalletInsert) (*data.DistributionWallet, error)
	getFn             func(ctx context.Context, id string) (*data.DistributionWallet, error)
	listFn            func(ctx context.Context, includeArchived bool) ([]data.DistributionWallet, error)
	archiveFn         func(ctx context.Context, id string) (*data.DistributionWallet, error)
	promoteFn         func(ctx context.Context, id string, scope []string) (*data.DistributionWallet, error)
	listMembershipsFn func(ctx context.Context, walletID string) ([]data.WalletMembership, error)
	listAuditFn       func(ctx context.Context, walletID string) ([]data.WalletMembershipAuditEntry, error)
	grantFn           func(ctx context.Context, walletID, userID string, role data.UserRole, grantedBy string) (*data.WalletMembership, error)
	revokeFn          func(ctx context.Context, walletID, membershipID, revokedBy string) error
}

func (m *mockDistributionWalletService) ArchiveWallet(ctx context.Context, id string) (*data.DistributionWallet, error) {
	return m.archiveFn(ctx, id)
}

func (m *mockDistributionWalletService) ListMemberships(ctx context.Context, walletID string) ([]data.WalletMembership, error) {
	return m.listMembershipsFn(ctx, walletID)
}

func (m *mockDistributionWalletService) ListMembershipAudit(ctx context.Context, walletID string) ([]data.WalletMembershipAuditEntry, error) {
	return m.listAuditFn(ctx, walletID)
}

func (m *mockDistributionWalletService) GrantMembership(ctx context.Context, walletID, userID string, role data.UserRole, grantedBy string) (*data.WalletMembership, error) {
	return m.grantFn(ctx, walletID, userID, role, grantedBy)
}

func (m *mockDistributionWalletService) RevokeMembership(ctx context.Context, walletID, membershipID, revokedBy string) error {
	return m.revokeFn(ctx, walletID, membershipID, revokedBy)
}

func (m *mockDistributionWalletService) PromoteToDefault(ctx context.Context, id string, scope []string) (*data.DistributionWallet, error) {
	return m.promoteFn(ctx, id, scope)
}

func (m *mockDistributionWalletService) CreateWallet(ctx context.Context, insert data.DistributionWalletInsert) (*data.DistributionWallet, error) {
	return m.createFn(ctx, insert)
}

func (m *mockDistributionWalletService) GetWallet(ctx context.Context, id string) (*data.DistributionWallet, error) {
	return m.getFn(ctx, id)
}

func (m *mockDistributionWalletService) ListWallets(ctx context.Context, includeArchived bool) ([]data.DistributionWallet, error) {
	return m.listFn(ctx, includeArchived)
}

var _ services.DistributionWalletManagementServiceInterface = (*mockDistributionWalletService)(nil)

func Test_DistributionWalletsHandler_PostDistributionWallet(t *testing.T) {
	wallet := &data.DistributionWallet{ID: "dw-123", Name: "program-a", Status: data.ActiveDistributionWalletStatus}

	testCases := []struct {
		name           string
		reqBody        string
		serviceErr     error
		wantStatusCode int
		wantContains   string
	}{
		{
			name:           "🎉 created",
			reqBody:        `{"name": "program-a"}`,
			wantStatusCode: http.StatusCreated,
			wantContains:   `"id": "dw-123"`,
		},
		{
			name:           "invalid JSON body returns 400",
			reqBody:        `{invalid`,
			wantStatusCode: http.StatusBadRequest,
			wantContains:   "invalid request body",
		},
		{
			name:           "duplicate name returns 409",
			reqBody:        `{"name": "program-a"}`,
			serviceErr:     fmt.Errorf("inserting: %w", data.ErrRecordAlreadyExists),
			wantStatusCode: http.StatusConflict,
			wantContains:   "already exists",
		},
		{
			name:           "cap exceeded returns 400",
			reqBody:        `{"name": "one-too-many"}`,
			serviceErr:     services.ErrDistributionWalletCapExceeded,
			wantStatusCode: http.StatusBadRequest,
			wantContains:   "maximum",
		},
		{
			name:           "unsupported type returns 400",
			reqBody:        `{"name": "circle"}`,
			serviceErr:     fmt.Errorf("creating: %w", services.ErrUnsupportedDistributionWalletType),
			wantStatusCode: http.StatusBadRequest,
			wantContains:   "DB_VAULT",
		},
		{
			name:           "ineligible (non-DB_VAULT) tenant returns 400",
			reqBody:        `{"name": "program-a"}`,
			serviceErr:     fmt.Errorf("creating: %w", services.ErrTenantNotEligibleForMultiWallet),
			wantStatusCode: http.StatusBadRequest,
			wantContains:   "only available to tenants",
		},
		{
			name:           "missing name returns 400",
			reqBody:        `{}`,
			serviceErr:     fmt.Errorf("validating: %w", data.ErrMissingInput),
			wantStatusCode: http.StatusBadRequest,
			wantContains:   "name is required",
		},
		{
			name:           "unexpected error returns 500",
			reqBody:        `{"name": "program-a"}`,
			serviceErr:     errors.New("boom"),
			wantStatusCode: http.StatusInternalServerError,
			wantContains:   "Cannot create distribution wallet",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			handler := DistributionWalletsHandler{AuthManager: newWalletScopeOwnerMock(), Service: &mockDistributionWalletService{
				createFn: func(_ context.Context, insert data.DistributionWalletInsert) (*data.DistributionWallet, error) {
					if tc.serviceErr != nil {
						return nil, tc.serviceErr
					}
					assert.Equal(t, "program-a", insert.Name)
					return wallet, nil
				},
			}}

			req := httptest.NewRequest(http.MethodPost, "/distribution-wallets", strings.NewReader(tc.reqBody))
			req = req.WithContext(sdpcontext.SetUserIDInContext(req.Context(), "payments-test-owner"))
			rr := httptest.NewRecorder()
			handler.PostDistributionWallet(rr, req)

			assert.Equal(t, tc.wantStatusCode, rr.Code)
			assert.Contains(t, rr.Body.String(), tc.wantContains)
		})
	}
}

func Test_DistributionWalletsHandler_GetDistributionWallets(t *testing.T) {
	t.Run("🎉 lists wallets and forwards include_archived", func(t *testing.T) {
		var gotIncludeArchived bool
		handler := DistributionWalletsHandler{Service: &mockDistributionWalletService{
			listFn: func(_ context.Context, includeArchived bool) ([]data.DistributionWallet, error) {
				gotIncludeArchived = includeArchived
				return []data.DistributionWallet{{ID: "dw-1", Name: "default", IsDefault: true}}, nil
			},
		}, AuthManager: newWalletScopeOwnerMock()}

		req := httptest.NewRequest(http.MethodGet, "/distribution-wallets?include_archived=true", nil)
		req = req.WithContext(sdpcontext.SetUserIDInContext(req.Context(), "payments-test-owner"))
		rr := httptest.NewRecorder()
		handler.GetDistributionWallets(rr, req)

		assert.Equal(t, http.StatusOK, rr.Code)
		assert.True(t, gotIncludeArchived)
		assert.Contains(t, rr.Body.String(), `"name": "default"`)
	})

	t.Run("service failure returns 500", func(t *testing.T) {
		handler := DistributionWalletsHandler{Service: &mockDistributionWalletService{
			listFn: func(_ context.Context, _ bool) ([]data.DistributionWallet, error) {
				return nil, errors.New("boom")
			},
		}}

		req := httptest.NewRequest(http.MethodGet, "/distribution-wallets", nil)
		rr := httptest.NewRecorder()
		handler.GetDistributionWallets(rr, req)

		assert.Equal(t, http.StatusInternalServerError, rr.Code)
	})
}

func Test_DistributionWalletsHandler_GetDistributionWallet(t *testing.T) {
	newRouter := func(handler DistributionWalletsHandler) *chi.Mux {
		r := chi.NewRouter()
		r.Get("/distribution-wallets/{id}", handler.GetDistributionWallet)
		return r
	}

	t.Run("🎉 returns the wallet", func(t *testing.T) {
		handler := DistributionWalletsHandler{AuthManager: newWalletScopeOwnerMock(), Service: &mockDistributionWalletService{
			getFn: func(_ context.Context, id string) (*data.DistributionWallet, error) {
				require.Equal(t, "dw-123", id)
				return &data.DistributionWallet{ID: "dw-123", Name: "program-a"}, nil
			},
		}}

		req := httptest.NewRequest(http.MethodGet, "/distribution-wallets/dw-123", nil)
		req = req.WithContext(sdpcontext.SetUserIDInContext(req.Context(), "payments-test-owner"))
		rr := httptest.NewRecorder()
		newRouter(handler).ServeHTTP(rr, req)

		assert.Equal(t, http.StatusOK, rr.Code)
		assert.Contains(t, rr.Body.String(), `"id": "dw-123"`)
	})

	t.Run("missing wallet returns 404", func(t *testing.T) {
		handler := DistributionWalletsHandler{AuthManager: newWalletScopeOwnerMock(), Service: &mockDistributionWalletService{
			getFn: func(_ context.Context, _ string) (*data.DistributionWallet, error) {
				return nil, fmt.Errorf("getting: %w", data.ErrRecordNotFound)
			},
		}}

		req := httptest.NewRequest(http.MethodGet, "/distribution-wallets/nope", nil)
		req = req.WithContext(sdpcontext.SetUserIDInContext(req.Context(), "payments-test-owner"))
		rr := httptest.NewRecorder()
		newRouter(handler).ServeHTTP(rr, req)

		assert.Equal(t, http.StatusNotFound, rr.Code)
	})
}

// scopedKeyContext is what APIKeyAuthenticator.Middleware puts in the context for a key.
func scopedKeyContext(ctx context.Context, key *data.APIKey) context.Context {
	return sdpcontext.SetUserIDInContext(sdpcontext.SetAPIKeyInContext(ctx, key), key.CreatedBy)
}

// Test_APIKeyWalletWriteScope pins the API-key path of the distribution-wallet writes: the key's permission and scope
// decide (404 outside it; promote also needs the demoted default in scope), never its creator's role.
func Test_APIKeyWalletWriteScope(t *testing.T) {
	const inScope, outOfScope = "dw-a", "dw-b"

	type calls struct {
		get, create, archive, grant, revoke int
		promoteScope                        []string
		promoted                            bool
		grantedBy, revokedBy                string
	}
	newHandler := func(t *testing.T, c *calls, promoteErr error) DistributionWalletsHandler {
		t.Helper()
		authManagerMock := &auth.AuthManagerMock{}
		authManagerMock.On("GetUserByID", mock.Anything, "grantee-x").
			Return(&auth.User{ID: "grantee-x", Roles: []string{string(data.BusinessUserRole)}}, nil).Maybe()

		return DistributionWalletsHandler{
			AuthManager: authManagerMock,
			Service: &mockDistributionWalletService{
				getFn: func(_ context.Context, id string) (*data.DistributionWallet, error) {
					c.get++
					return &data.DistributionWallet{ID: id, Status: data.ActiveDistributionWalletStatus}, nil
				},
				createFn: func(_ context.Context, insert data.DistributionWalletInsert) (*data.DistributionWallet, error) {
					c.create++
					return &data.DistributionWallet{ID: "dw-new", Name: insert.Name}, nil
				},
				archiveFn: func(_ context.Context, id string) (*data.DistributionWallet, error) {
					c.archive++
					return &data.DistributionWallet{ID: id, Status: data.ArchivedDistributionWalletStatus}, nil
				},
				promoteFn: func(_ context.Context, id string, scope []string) (*data.DistributionWallet, error) {
					c.promoted, c.promoteScope = true, scope
					if promoteErr != nil {
						return nil, promoteErr
					}
					return &data.DistributionWallet{ID: id, IsDefault: true}, nil
				},
				grantFn: func(_ context.Context, walletID, userID string, role data.UserRole, grantedBy string) (*data.WalletMembership, error) {
					c.grant++
					c.grantedBy = grantedBy
					return &data.WalletMembership{ID: "m-1", WalletID: walletID, UserID: userID, Role: role}, nil
				},
				revokeFn: func(_ context.Context, _, _, revokedBy string) error {
					c.revoke++
					c.revokedBy = revokedBy
					return nil
				},
			},
		}
	}
	// Wired as serve.go wires the Owner-only write group.
	newRouter := func(handler DistributionWalletsHandler) *chi.Mux {
		r := chi.NewRouter()
		r.Group(func(r chi.Router) {
			r.Use(middleware.RequirePermission(data.WriteDistributionWallets, middleware.AnyRoleMiddleware(handler.AuthManager, data.OwnerUserRole)))
			r.Post("/distribution-wallets", handler.PostDistributionWallet)
			r.Post("/distribution-wallets/{id}/archive", handler.PostArchiveDistributionWallet)
			r.Post("/distribution-wallets/{id}/promote-to-default", handler.PostPromoteDistributionWalletToDefault)
			r.Post("/distribution-wallets/{id}/memberships", handler.PostDistributionWalletMembership)
			r.Delete("/distribution-wallets/{id}/memberships/{membershipID}", handler.DeleteDistributionWalletMembership)
		})
		return r
	}
	// The creator is a developer who no longer exists: neither fact matters to the key.
	key := &data.APIKey{
		ID:                    "ak-1",
		Permissions:           data.APIKeyPermissions{data.ReadAll, data.WriteAll},
		DistributionWalletIDs: []string{inScope},
		CreatedBy:             "deleted-developer",
	}
	do := func(handler DistributionWalletsHandler, method, path, body string) *httptest.ResponseRecorder {
		var reader io.Reader
		if body != "" {
			reader = strings.NewReader(body)
		}
		req := httptest.NewRequest(method, path, reader)
		rr := httptest.NewRecorder()
		newRouter(handler).ServeHTTP(rr, req.WithContext(scopedKeyContext(req.Context(), key)))
		return rr
	}

	routes := []struct {
		name, method, path, body string
		okStatus                 int
		reached                  func(c *calls) bool
	}{
		{"archive", http.MethodPost, "/distribution-wallets/%s/archive", "", http.StatusOK, func(c *calls) bool { return c.archive > 0 }},
		{"grant", http.MethodPost, "/distribution-wallets/%s/memberships", `{"user_id": "grantee-x", "role": "approver"}`, http.StatusCreated, func(c *calls) bool { return c.get > 0 || c.grant > 0 }},
		{"revoke", http.MethodDelete, "/distribution-wallets/%s/memberships/m-1", "", http.StatusNoContent, func(c *calls) bool { return c.revoke > 0 }},
		{"promote", http.MethodPost, "/distribution-wallets/%s/promote-to-default", "", http.StatusOK, func(c *calls) bool { return c.promoted }},
	}

	for _, route := range routes {
		t.Run(route.name+": out of scope is 404 and never reaches the service", func(t *testing.T) {
			c := &calls{}
			rr := do(newHandler(t, c, nil), route.method, fmt.Sprintf(route.path, outOfScope), route.body)
			assert.Equal(t, http.StatusNotFound, rr.Code)
			assert.False(t, route.reached(c))
		})

		t.Run(route.name+": in scope is served", func(t *testing.T) {
			c := &calls{}
			rr := do(newHandler(t, c, nil), route.method, fmt.Sprintf(route.path, inScope), route.body)
			assert.Equal(t, route.okStatus, rr.Code, rr.Body.String())
			assert.True(t, route.reached(c))
		})
	}

	t.Run("grant and revoke record the key's creator as the actor", func(t *testing.T) {
		c := &calls{}
		handler := newHandler(t, c, nil)
		do(handler, http.MethodPost, "/distribution-wallets/dw-a/memberships", `{"user_id": "grantee-x", "role": "approver"}`)
		do(handler, http.MethodDelete, "/distribution-wallets/dw-a/memberships/m-1", "")
		assert.Equal(t, key.CreatedBy, c.grantedBy)
		assert.Equal(t, key.CreatedBy, c.revokedBy)
	})

	t.Run("promote passes the key's scope so the demoted account is checked under the lock", func(t *testing.T) {
		c := &calls{}
		do(newHandler(t, c, nil), http.MethodPost, "/distribution-wallets/dw-a/promote-to-default", "")
		assert.Equal(t, []string{inScope}, c.promoteScope)
	})

	t.Run("promote is 403 when the account being demoted is out of scope", func(t *testing.T) {
		c := &calls{}
		denied := fmt.Errorf("demoting: %w", services.ErrWalletActionForbidden)
		rr := do(newHandler(t, c, denied), http.MethodPost, "/distribution-wallets/dw-a/promote-to-default", "")
		assert.Equal(t, http.StatusForbidden, rr.Code)
		assert.NotContains(t, rr.Body.String(), outOfScope)
	})

	t.Run("create has no wallet to scope: any key with the permission creates", func(t *testing.T) {
		c := &calls{}
		rr := do(newHandler(t, c, nil), http.MethodPost, "/distribution-wallets", `{"name": "program-c"}`)
		assert.Equal(t, http.StatusCreated, rr.Code)
		assert.Equal(t, 1, c.create)
	})

	t.Run("a key without the permission is refused by the route", func(t *testing.T) {
		c := &calls{}
		readOnly := *key
		readOnly.Permissions = data.APIKeyPermissions{data.ReadDistributionWallets}
		req := httptest.NewRequest(http.MethodPost, "/distribution-wallets/dw-a/archive", nil)
		rr := httptest.NewRecorder()
		newRouter(newHandler(t, c, nil)).ServeHTTP(rr, req.WithContext(scopedKeyContext(req.Context(), &readOnly)))
		assert.Equal(t, http.StatusForbidden, rr.Code)
		assert.Zero(t, c.archive)
	})
}

// Test_DistributionWalletsHandler_ownersAreTenantWide pins that a JWT owner's writes are unscoped and
// that promote hands the service a nil scope, so no demotion check applies.
func Test_DistributionWalletsHandler_ownersAreTenantWide(t *testing.T) {
	var promoteScope []string
	promoteCalled := false
	handler := DistributionWalletsHandler{
		AuthManager: newWalletScopeOwnerMock(),
		Service: &mockDistributionWalletService{
			archiveFn: func(_ context.Context, id string) (*data.DistributionWallet, error) {
				return &data.DistributionWallet{ID: id}, nil
			},
			promoteFn: func(_ context.Context, id string, scope []string) (*data.DistributionWallet, error) {
				promoteCalled, promoteScope = true, scope
				return &data.DistributionWallet{ID: id, IsDefault: true}, nil
			},
			revokeFn: func(context.Context, string, string, string) error { return nil },
		},
	}
	r := chi.NewRouter()
	r.Post("/distribution-wallets/{id}/archive", handler.PostArchiveDistributionWallet)
	r.Post("/distribution-wallets/{id}/promote-to-default", handler.PostPromoteDistributionWalletToDefault)
	r.Delete("/distribution-wallets/{id}/memberships/{membershipID}", handler.DeleteDistributionWalletMembership)

	for _, tc := range []struct{ method, path string }{
		{http.MethodPost, "/distribution-wallets/dw-any/archive"},
		{http.MethodPost, "/distribution-wallets/dw-any/promote-to-default"},
		{http.MethodDelete, "/distribution-wallets/dw-any/memberships/m-1"},
	} {
		req := httptest.NewRequest(tc.method, tc.path, nil)
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req.WithContext(sdpcontext.SetUserIDInContext(req.Context(), "payments-test-owner")))
		assert.Lessf(t, rr.Code, 300, "%s %s: %s", tc.method, tc.path, rr.Body.String())
	}
	require.True(t, promoteCalled)
	assert.Nil(t, promoteScope)
}

// Test_DistributionWalletsHandler_grantIsNotInert pins that a grant is rejected exactly when the grantee
// is tenant-wide; read-only grants (the membership is the read-visibility grant) still succeed.
func Test_DistributionWalletsHandler_grantIsNotInert(t *testing.T) {
	testCases := []struct {
		name           string
		granteeRoles   []string
		granteeMissing bool
		role           string
		wantStatusCode int
		wantContains   string
	}{
		{
			name:           "🎉 global financial controller + approver here: approve but do not create",
			granteeRoles:   []string{string(data.FinancialControllerUserRole)},
			role:           "approver",
			wantStatusCode: http.StatusCreated,
		},
		{
			name:           "🎉 global initiator + financial controller here: create on this account",
			granteeRoles:   []string{string(data.InitiatorUserRole)},
			role:           "financial_controller",
			wantStatusCode: http.StatusCreated,
		},
		{
			name:           "global developer grantee is rejected as inert",
			granteeRoles:   []string{string(data.DeveloperUserRole)},
			role:           "approver",
			wantStatusCode: http.StatusBadRequest,
			wantContains:   "an approver membership grants nothing to a developer: developers already have tenant-wide access",
		},
		{
			name:           "global approver + initiator: read visibility is a real grant",
			granteeRoles:   []string{string(data.ApproverUserRole)},
			role:           "initiator",
			wantStatusCode: http.StatusCreated,
		},
		{
			name:           "global initiator + approver: read visibility is a real grant",
			granteeRoles:   []string{string(data.InitiatorUserRole)},
			role:           "approver",
			wantStatusCode: http.StatusCreated,
		},
		{
			name:           "global business + approver: read visibility is a real grant",
			granteeRoles:   []string{string(data.BusinessUserRole)},
			role:           "approver",
			wantStatusCode: http.StatusCreated,
		},
		{
			name:           "global owner grantee is rejected as inert",
			granteeRoles:   []string{string(data.OwnerUserRole)},
			role:           "financial_controller",
			wantStatusCode: http.StatusBadRequest,
			wantContains:   "grants nothing to an owner",
		},
		{
			name:           "developer is not a grantable role",
			granteeRoles:   []string{string(data.BusinessUserRole)},
			role:           "developer",
			wantStatusCode: http.StatusBadRequest,
			wantContains:   "unexpected value for role",
		},
		{
			name:           "unknown grantee returns 400",
			granteeMissing: true,
			role:           "approver",
			wantStatusCode: http.StatusBadRequest,
			wantContains:   `user \"user-x\" not found`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			authManagerMock := &auth.AuthManagerMock{}
			authManagerMock.On("GetUserByID", mock.Anything, "owner-1").
				Return(&auth.User{ID: "owner-1", IsOwner: true}, nil).Maybe()
			if tc.granteeMissing {
				authManagerMock.On("GetUserByID", mock.Anything, "user-x").
					Return(nil, fmt.Errorf("getting: %w", auth.ErrUserNotFound))
			} else {
				authManagerMock.On("GetUserByID", mock.Anything, "user-x").
					Return(&auth.User{ID: "user-x", Roles: tc.granteeRoles}, nil)
			}

			handler := DistributionWalletsHandler{
				AuthManager: authManagerMock,
				Service: &mockDistributionWalletService{
					getFn: func(_ context.Context, id string) (*data.DistributionWallet, error) {
						return &data.DistributionWallet{ID: id, Status: data.ActiveDistributionWalletStatus}, nil
					},
					grantFn: func(_ context.Context, walletID, userID string, role data.UserRole, _ string) (*data.WalletMembership, error) {
						return &data.WalletMembership{ID: "m-1", WalletID: walletID, UserID: userID, Role: role}, nil
					},
				},
			}

			r := chi.NewRouter()
			r.Post("/distribution-wallets/{id}/memberships", handler.PostDistributionWalletMembership)
			req := httptest.NewRequest(http.MethodPost, "/distribution-wallets/dw-1/memberships",
				strings.NewReader(fmt.Sprintf(`{"user_id": "user-x", "role": %q}`, tc.role)))
			req = req.WithContext(sdpcontext.SetUserIDInContext(req.Context(), "owner-1"))
			rr := httptest.NewRecorder()
			r.ServeHTTP(rr, req)

			assert.Equal(t, tc.wantStatusCode, rr.Code)
			if tc.wantContains != "" {
				assert.Contains(t, rr.Body.String(), tc.wantContains)
			}
		})
	}
}

func Test_DistributionWalletsHandler_GetDistributionWalletCapabilities(t *testing.T) {
	newRouter := func(handler DistributionWalletsHandler) *chi.Mux {
		r := chi.NewRouter()
		r.Get("/distribution-wallets/{id}/capabilities", handler.GetDistributionWalletCapabilities)
		return r
	}
	do := func(handler DistributionWalletsHandler, path string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req = req.WithContext(sdpcontext.SetUserIDInContext(req.Context(), "payments-test-owner"))
		rr := httptest.NewRecorder()
		newRouter(handler).ServeHTTP(rr, req)
		return rr
	}

	t.Run("🎉 owners hold every capability on every account, with no membership", func(t *testing.T) {
		handler := DistributionWalletsHandler{AuthManager: newWalletScopeOwnerMock(), Service: &mockDistributionWalletService{
			getFn: func(_ context.Context, id string) (*data.DistributionWallet, error) {
				return &data.DistributionWallet{ID: id, Name: "program-a"}, nil
			},
		}}

		rr := do(handler, "/distribution-wallets/dw-1/capabilities")
		require.Equal(t, http.StatusOK, rr.Code)
		body := rr.Body.String()
		assert.Contains(t, body, `"wallet_id": "dw-1"`)
		for _, capability := range walletCapabilityMatrix {
			assert.Contains(t, body, fmt.Sprintf("%q: true", capability.name))
		}
		assert.NotContains(t, body, "effective_role", "there is no effective role to report")
	})

	t.Run("unknown wallet returns 404", func(t *testing.T) {
		handler := DistributionWalletsHandler{AuthManager: newWalletScopeOwnerMock(), Service: &mockDistributionWalletService{
			getFn: func(_ context.Context, _ string) (*data.DistributionWallet, error) {
				return nil, fmt.Errorf("getting: %w", data.ErrRecordNotFound)
			},
		}}

		rr := do(handler, "/distribution-wallets/nope/capabilities")
		assert.Equal(t, http.StatusNotFound, rr.Code)
	})

	t.Run("no parameters: the caller's own set, with no subject echoed back", func(t *testing.T) {
		handler := DistributionWalletsHandler{AuthManager: newWalletScopeOwnerMock(), Service: &mockDistributionWalletService{
			getFn: func(_ context.Context, id string) (*data.DistributionWallet, error) {
				return &data.DistributionWallet{ID: id, Name: "program-a"}, nil
			},
		}}

		rr := do(handler, "/distribution-wallets/dw-1/capabilities")
		require.Equal(t, http.StatusOK, rr.Code)
		// The shape the frontend already consumes: wallet_id + capabilities, nothing else.
		assert.NotContains(t, rr.Body.String(), "user_id")
		assert.NotContains(t, rr.Body.String(), `"role"`)
	})
}

// Test_DistributionWalletsHandler_GetDistributionWalletCapabilities_forSubject covers the grant
// picker's data source: ?user_id= reports a named user's capabilities on this account, and the
// optional ?role= turns that into the hypothetical the picker needs — "what would THIS grant give
// THIS user here" — with the matrix staying server-side so the client never becomes a second
// authority on it.
func Test_DistributionWalletsHandler_GetDistributionWalletCapabilities_forSubject(t *testing.T) {
	owner := &auth.User{ID: "owner-1", IsOwner: true}

	do := func(handler DistributionWalletsHandler, callerID, query string) *httptest.ResponseRecorder {
		r := chi.NewRouter()
		r.Get("/distribution-wallets/{id}/capabilities", handler.GetDistributionWalletCapabilities)
		req := httptest.NewRequest(http.MethodGet, "/distribution-wallets/dw-1/capabilities"+query, nil)
		req = req.WithContext(sdpcontext.SetUserIDInContext(req.Context(), callerID))
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)
		return rr
	}
	decode := func(t *testing.T, rr *httptest.ResponseRecorder) WalletCapabilitiesResponse {
		t.Helper()
		var got WalletCapabilitiesResponse
		require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &got))
		return got
	}
	servingWallet := func() *mockDistributionWalletService {
		return &mockDistributionWalletService{
			getFn: func(_ context.Context, id string) (*data.DistributionWallet, error) {
				return &data.DistributionWallet{ID: id, Name: "program-a"}, nil
			},
		}
	}
	// ownerCallerFor resolves the owner caller plus (optionally) one subject, and nobody else.
	ownerCallerFor := func(subject *auth.User) *auth.AuthManagerMock {
		m := &auth.AuthManagerMock{}
		m.On("GetUserByID", mock.Anything, owner.ID).Return(owner, nil).Maybe()
		if subject != nil {
			m.On("GetUserByID", mock.Anything, subject.ID).Return(subject, nil).Maybe()
		}
		return m
	}
	noCapabilities := func() map[string]bool {
		none := make(map[string]bool, len(walletCapabilityMatrix))
		for _, capability := range walletCapabilityMatrix {
			none[capability.name] = false
		}
		return none
	}

	// A developer clears no capability's global gate, so no membership role could give them a write here.
	t.Run("a global developer: every role yields the same nothing", func(t *testing.T) {
		developer := &auth.User{ID: "dev-1", Roles: []string{string(data.DeveloperUserRole)}}
		handler := DistributionWalletsHandler{AuthManager: ownerCallerFor(developer), Service: servingWallet()}

		for _, role := range []data.UserRole{
			data.FinancialControllerUserRole, data.ApproverUserRole,
			data.InitiatorUserRole, data.BusinessUserRole,
		} {
			rr := do(handler, owner.ID, "?user_id=dev-1&role="+role.String())
			require.Equalf(t, http.StatusOK, rr.Code, "role=%s", role)

			got := decode(t, rr)
			assert.Equal(t, "dw-1", got.WalletID)
			assert.Equal(t, "dev-1", got.UserID)
			assert.Equal(t, role.String(), got.Role)
			assert.Equalf(t, noCapabilities(), got.Capabilities,
				"granting %s to a global developer yields nothing: the picker must be able to say so", role)
		}
	})

	t.Run("a developer subject is tenant-wide: no membership lookup, and still nothing to write", func(t *testing.T) {
		developer := &auth.User{ID: "dev-1", Roles: []string{string(data.DeveloperUserRole)}}
		handler := DistributionWalletsHandler{AuthManager: ownerCallerFor(developer), Service: servingWallet()}

		rr := do(handler, owner.ID, "?user_id=dev-1")
		require.Equal(t, http.StatusOK, rr.Code)
		assert.Equal(t, noCapabilities(), decode(t, rr).Capabilities)
	})

	t.Run("an API key reaches the subject readings through its permission, not its creator's role", func(t *testing.T) {
		controller := &auth.User{ID: "fc-1", Roles: []string{string(data.FinancialControllerUserRole)}}
		handler := DistributionWalletsHandler{AuthManager: ownerCallerFor(controller), Service: servingWallet()}

		r := chi.NewRouter()
		r.Get("/distribution-wallets/{id}/capabilities", handler.GetDistributionWalletCapabilities)
		req := httptest.NewRequest(http.MethodGet, "/distribution-wallets/dw-1/capabilities?user_id=fc-1&role=approver", nil)
		ctx := scopedKeyContext(req.Context(), &data.APIKey{
			ID: "ak-1", Permissions: data.APIKeyPermissions{data.ReadDistributionWallets},
			DistributionWalletIDs: []string{"dw-1"}, CreatedBy: "developer-1",
		})
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req.WithContext(ctx))
		require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
		assert.True(t, decode(t, rr).Capabilities["can_start_disbursement"])
	})

	t.Run("🎉 a global financial controller granted approver here: approve, but do not create", func(t *testing.T) {
		controller := &auth.User{ID: "fc-1", Roles: []string{string(data.FinancialControllerUserRole)}}
		handler := DistributionWalletsHandler{AuthManager: ownerCallerFor(controller), Service: servingWallet()}

		rr := do(handler, owner.ID, "?user_id=fc-1&role=approver")
		require.Equal(t, http.StatusOK, rr.Code)
		assert.Equal(t, map[string]bool{
			"can_create_disbursement": false, "can_start_disbursement": true,
			"can_pause_disbursement": true, "can_cancel_disbursement": true,
			"can_create_payment": false, "can_retry_payment": false, "can_cancel_payment": false,
		}, decode(t, rr).Capabilities)
	})

	t.Run("🎉 a global initiator granted financial controller here: still capped at create", func(t *testing.T) {
		initiator := &auth.User{ID: "init-1", Roles: []string{string(data.InitiatorUserRole)}}
		handler := DistributionWalletsHandler{AuthManager: ownerCallerFor(initiator), Service: servingWallet()}

		rr := do(handler, owner.ID, "?user_id=init-1&role=financial_controller")
		require.Equal(t, http.StatusOK, rr.Code)
		assert.Equal(t, map[string]bool{
			"can_create_disbursement": true, "can_start_disbursement": false,
			"can_pause_disbursement": false, "can_cancel_disbursement": false,
			"can_create_payment": false, "can_retry_payment": false, "can_cancel_payment": false,
		}, decode(t, rr).Capabilities)
	})

	// An Owner subject needs no membership lookup at all — they short-circuit both gates — which
	// is also why granting them a membership is the one grant PostDistributionWalletMembership
	// rejects as inert.
	t.Run("an owner subject holds everything, with no membership row", func(t *testing.T) {
		otherOwner := &auth.User{ID: "owner-2", IsOwner: true}
		handler := DistributionWalletsHandler{AuthManager: ownerCallerFor(otherOwner), Service: servingWallet()}

		rr := do(handler, owner.ID, "?user_id=owner-2")
		require.Equal(t, http.StatusOK, rr.Code)
		got := decode(t, rr)
		assert.Equal(t, "owner-2", got.UserID)
		assert.Empty(t, got.Role)
		for name, allowed := range got.Capabilities {
			assert.Truef(t, allowed, "%s must be allowed for an owner", name)
		}
	})

	// Fail closed: the parameterized readings disclose a third party's authority, and only Owners
	// can grant, so a non-owner is refused before the subject or the wallet is touched.
	t.Run("a non-owner caller is refused and learns nothing about the subject or the wallet", func(t *testing.T) {
		authManagerMock := &auth.AuthManagerMock{}
		authManagerMock.On("GetUserByID", mock.Anything, "member-1").
			Return(&auth.User{ID: "member-1", Roles: []string{string(data.FinancialControllerUserRole)}}, nil).Maybe()
		authManagerMock.On("GetUserByID", mock.Anything, "dev-1").
			Run(func(mock.Arguments) { t.Error("the subject was looked up for a non-owner caller") }).
			Return(nil, fmt.Errorf("getting: %w", auth.ErrUserNotFound)).Maybe()

		handler := DistributionWalletsHandler{
			AuthManager: authManagerMock,
			Service: &mockDistributionWalletService{
				getFn: func(context.Context, string) (*data.DistributionWallet, error) {
					t.Error("the wallet was loaded for a non-owner caller")
					return nil, nil
				},
			},
		}

		for _, query := range []string{"?user_id=dev-1", "?user_id=dev-1&role=approver", "?role=approver"} {
			rr := do(handler, "member-1", query)
			assert.Equalf(t, http.StatusForbidden, rr.Code, "query=%s", query)
			assert.NotContains(t, rr.Body.String(), "dev-1")
			assert.NotContains(t, rr.Body.String(), "program-a")
		}
	})

	t.Run("role without user_id has no subject to be hypothetical about", func(t *testing.T) {
		handler := DistributionWalletsHandler{
			AuthManager: ownerCallerFor(nil),
			Service: &mockDistributionWalletService{
				getFn: func(context.Context, string) (*data.DistributionWallet, error) {
					t.Error("the wallet was loaded for a malformed request")
					return nil, nil
				},
			},
		}

		rr := do(handler, owner.ID, "?role=approver")
		assert.Equal(t, http.StatusBadRequest, rr.Code)
		assert.Contains(t, rr.Body.String(), "user_id is required when role is provided")
	})

	t.Run("an unscopable or unknown role is rejected with the grantable set", func(t *testing.T) {
		developer := &auth.User{ID: "dev-1", Roles: []string{string(data.DeveloperUserRole)}}
		handler := DistributionWalletsHandler{
			AuthManager: ownerCallerFor(developer),
			Service: &mockDistributionWalletService{
				getFn: func(context.Context, string) (*data.DistributionWallet, error) {
					t.Error("the wallet was loaded for an invalid role")
					return nil, nil
				},
			},
		}

		for _, role := range []string{"nonsense", "owner", "developer"} {
			rr := do(handler, owner.ID, "?user_id=dev-1&role="+role)
			assert.Equalf(t, http.StatusBadRequest, rr.Code, "role=%s", role)
			assert.Contains(t, rr.Body.String(), "unexpected value for role")
		}
	})

	t.Run("an unknown subject returns 400, after the wallet has been established", func(t *testing.T) {
		authManagerMock := ownerCallerFor(nil)
		authManagerMock.On("GetUserByID", mock.Anything, "ghost").
			Return(nil, fmt.Errorf("getting: %w", auth.ErrUserNotFound))

		handler := DistributionWalletsHandler{AuthManager: authManagerMock, Service: servingWallet()}

		rr := do(handler, owner.ID, "?user_id=ghost")
		assert.Equal(t, http.StatusBadRequest, rr.Code)
		assert.Contains(t, rr.Body.String(), `user \"ghost\" not found`)
	})
}

// Test_DistributionWalletsHandler_capabilitiesForSubject_realMemberships pins the two readings
// against real membership rows: ?user_id= reports what the subject holds TODAY, while
// ?user_id=&role= reports what the proposed role would yield ON ITS OWN. The distinction matters
// for the grant picker — computing the hypothetical as "existing rows ∪ the new role" would make
// an inert option look productive to anyone who already holds a stronger row here.
func Test_DistributionWalletsHandler_capabilitiesForSubject_realMemberships(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()
	models, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)

	wallet := data.EnsureDefaultDistributionWalletFixture(t, ctx, dbConnectionPool)
	subject := &auth.User{ID: "subject-1", Roles: []string{string(data.FinancialControllerUserRole)}}
	_, err = models.WalletMemberships.Insert(ctx, dbConnectionPool, subject.ID, wallet.ID, data.ApproverUserRole, nil)
	require.NoError(t, err)

	owner := &auth.User{ID: "owner-1", IsOwner: true}
	authManagerMock := &auth.AuthManagerMock{}
	authManagerMock.On("GetUserByID", mock.Anything, owner.ID).Return(owner, nil).Maybe()
	authManagerMock.On("GetUserByID", mock.Anything, subject.ID).Return(subject, nil).Maybe()

	handler := DistributionWalletsHandler{
		AuthManager: authManagerMock,
		Models:      models,
		Service: &mockDistributionWalletService{
			getFn: func(_ context.Context, id string) (*data.DistributionWallet, error) {
				return &data.DistributionWallet{ID: id, Name: "program-a"}, nil
			},
		},
	}

	do := func(query string) WalletCapabilitiesResponse {
		r := chi.NewRouter()
		r.Get("/distribution-wallets/{id}/capabilities", handler.GetDistributionWalletCapabilities)
		req := httptest.NewRequest(http.MethodGet, "/distribution-wallets/"+wallet.ID+"/capabilities"+query, nil)
		req = req.WithContext(sdpcontext.SetUserIDInContext(req.Context(), owner.ID))
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)
		require.Equal(t, http.StatusOK, rr.Code)

		var got WalletCapabilitiesResponse
		require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &got))
		return got
	}

	t.Run("🎉 ?user_id= reads the subject's real rows: global FC narrowed to approver", func(t *testing.T) {
		got := do("?user_id=" + subject.ID)
		assert.Empty(t, got.Role)
		assert.Equal(t, map[string]bool{
			"can_create_disbursement": false, "can_start_disbursement": true,
			"can_pause_disbursement": true, "can_cancel_disbursement": true,
			"can_create_payment": false, "can_retry_payment": false, "can_cancel_payment": false,
		}, got.Capabilities)
	})

	t.Run("🎉 ?role= is what THAT grant would yield, not the union with what they already hold", func(t *testing.T) {
		got := do("?user_id=" + subject.ID + "&role=initiator")
		assert.Equal(t, "initiator", got.Role)
		assert.Equal(t, map[string]bool{
			"can_create_disbursement": true, "can_start_disbursement": false,
			"can_pause_disbursement": false, "can_cancel_disbursement": false,
			"can_create_payment": false, "can_retry_payment": false, "can_cancel_payment": false,
		}, got.Capabilities,
			"the subject's existing approver row must not leak into the hypothetical")
	})
}

func Test_DistributionWalletsHandler_lifecycleEndpoints(t *testing.T) {
	newRouter := func(handler DistributionWalletsHandler) *chi.Mux {
		r := chi.NewRouter()
		r.Post("/distribution-wallets/{id}/archive", handler.PostArchiveDistributionWallet)
		r.Post("/distribution-wallets/{id}/promote-to-default", handler.PostPromoteDistributionWalletToDefault)
		return r
	}
	do := func(handler DistributionWalletsHandler, path string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, path, nil)
		req = req.WithContext(sdpcontext.SetUserIDInContext(req.Context(), "payments-test-owner"))
		rr := httptest.NewRecorder()
		newRouter(handler).ServeHTTP(rr, req)
		return rr
	}

	archiveCases := []struct {
		name           string
		serviceErr     error
		wantStatusCode int
	}{
		{"🎉 archived", nil, http.StatusOK},
		{"missing wallet returns 404", fmt.Errorf("x: %w", data.ErrRecordNotFound), http.StatusNotFound},
		{"default wallet returns 400", services.ErrCannotArchiveDefaultWallet, http.StatusBadRequest},
		{"last active returns 400", services.ErrCannotArchiveLastActiveWallet, http.StatusBadRequest},
		{"unexpected error returns 500", errors.New("boom"), http.StatusInternalServerError},
	}
	for _, tc := range archiveCases {
		t.Run("archive: "+tc.name, func(t *testing.T) {
			handler := DistributionWalletsHandler{AuthManager: newWalletScopeOwnerMock(), Service: &mockDistributionWalletService{
				archiveFn: func(_ context.Context, id string) (*data.DistributionWallet, error) {
					assert.Equal(t, "dw-1", id)
					if tc.serviceErr != nil {
						return nil, tc.serviceErr
					}
					return &data.DistributionWallet{ID: "dw-1", Status: data.ArchivedDistributionWalletStatus}, nil
				},
			}}
			rr := do(handler, "/distribution-wallets/dw-1/archive")
			assert.Equal(t, tc.wantStatusCode, rr.Code)
		})
	}

	membershipCases := []struct {
		name           string
		serviceErr     error
		wantStatusCode int
	}{
		{"🎉 granted", nil, http.StatusCreated},
		{"archived wallet returns 409", fmt.Errorf("x: %w", data.ErrWalletArchivedForMembership), http.StatusConflict},
		{"duplicate grant returns 409", fmt.Errorf("x: %w", data.ErrRecordAlreadyExists), http.StatusConflict},
		{"missing wallet returns 404", fmt.Errorf("x: %w", data.ErrRecordNotFound), http.StatusNotFound},
		{"invalid input returns 400", fmt.Errorf("x: %w", data.ErrMissingInput), http.StatusBadRequest},
	}
	for _, tc := range membershipCases {
		t.Run("grant: "+tc.name, func(t *testing.T) {
			grantorMock := &auth.AuthManagerMock{}
			grantorMock.On("GetUserByID", mock.Anything, "owner-1").
				Return(&auth.User{ID: "owner-1", IsOwner: true}, nil)
			// A global financial controller granted approver here: narrowed to start/pause/
			// cancel, so the grant does confer something and passes the inert-grant check.
			grantorMock.On("GetUserByID", mock.Anything, "user-x").
				Return(&auth.User{ID: "user-x", Roles: []string{string(data.FinancialControllerUserRole)}}, nil)
			handler := DistributionWalletsHandler{
				AuthManager: grantorMock,
				Service: &mockDistributionWalletService{
					getFn: func(_ context.Context, id string) (*data.DistributionWallet, error) {
						return &data.DistributionWallet{ID: id, Status: data.ActiveDistributionWalletStatus}, nil
					},
					grantFn: func(_ context.Context, walletID, userID string, role data.UserRole, grantedBy string) (*data.WalletMembership, error) {
						assert.Equal(t, "dw-1", walletID)
						assert.Equal(t, "user-x", userID)
						assert.Equal(t, data.ApproverUserRole, role)
						assert.Equal(t, "owner-1", grantedBy)
						if tc.serviceErr != nil {
							return nil, tc.serviceErr
						}
						return &data.WalletMembership{ID: "m-1", WalletID: walletID, UserID: userID, Role: role}, nil
					},
				},
			}

			r := chi.NewRouter()
			r.Post("/distribution-wallets/{id}/memberships", handler.PostDistributionWalletMembership)
			req := httptest.NewRequest(http.MethodPost, "/distribution-wallets/dw-1/memberships",
				strings.NewReader(`{"user_id": "user-x", "role": "approver"}`))
			req = req.WithContext(sdpcontext.SetUserIDInContext(req.Context(), "owner-1"))
			rr := httptest.NewRecorder()
			r.ServeHTTP(rr, req)
			assert.Equal(t, tc.wantStatusCode, rr.Code)
		})
	}

	t.Run("revoke: 204 on success, 404 when missing", func(t *testing.T) {
		for _, tc := range []struct {
			serviceErr     error
			wantStatusCode int
		}{
			{nil, http.StatusNoContent},
			{fmt.Errorf("x: %w", data.ErrRecordNotFound), http.StatusNotFound},
		} {
			revokerMock := &auth.AuthManagerMock{}
			revokerMock.On("GetUserByID", mock.Anything, "owner-1").
				Return(&auth.User{ID: "owner-1", IsOwner: true}, nil)
			handler := DistributionWalletsHandler{
				AuthManager: revokerMock,
				Service: &mockDistributionWalletService{
					revokeFn: func(_ context.Context, walletID, membershipID, revokedBy string) error {
						assert.Equal(t, "dw-1", walletID)
						assert.Equal(t, "m-1", membershipID)
						assert.Equal(t, "owner-1", revokedBy)
						return tc.serviceErr
					},
				},
			}
			r := chi.NewRouter()
			r.Delete("/distribution-wallets/{id}/memberships/{membershipID}", handler.DeleteDistributionWalletMembership)
			req := httptest.NewRequest(http.MethodDelete, "/distribution-wallets/dw-1/memberships/m-1", nil)
			req = req.WithContext(sdpcontext.SetUserIDInContext(req.Context(), "owner-1"))
			rr := httptest.NewRecorder()
			r.ServeHTTP(rr, req)
			assert.Equal(t, tc.wantStatusCode, rr.Code)
		}
	})

	t.Run("list memberships: 200 + 404", func(t *testing.T) {
		handler := DistributionWalletsHandler{AuthManager: newWalletScopeOwnerMock(), Service: &mockDistributionWalletService{
			listMembershipsFn: func(_ context.Context, walletID string) ([]data.WalletMembership, error) {
				if walletID == "missing" {
					return nil, fmt.Errorf("x: %w", data.ErrRecordNotFound)
				}
				return []data.WalletMembership{{ID: "m-1", WalletID: walletID, UserID: "user-x", Role: data.ApproverUserRole}}, nil
			},
		}}
		r := chi.NewRouter()
		r.Get("/distribution-wallets/{id}/memberships", handler.GetDistributionWalletMemberships)

		req := httptest.NewRequest(http.MethodGet, "/distribution-wallets/dw-1/memberships", nil)
		req = req.WithContext(sdpcontext.SetUserIDInContext(req.Context(), "payments-test-owner"))
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)
		assert.Equal(t, http.StatusOK, rr.Code)
		assert.Contains(t, rr.Body.String(), `"m-1"`)

		req = httptest.NewRequest(http.MethodGet, "/distribution-wallets/missing/memberships", nil)
		req = req.WithContext(sdpcontext.SetUserIDInContext(req.Context(), "payments-test-owner"))
		rr = httptest.NewRecorder()
		r.ServeHTTP(rr, req)
		assert.Equal(t, http.StatusNotFound, rr.Code)
	})

	promoteCases := []struct {
		name           string
		serviceErr     error
		wantStatusCode int
	}{
		{"🎉 promoted", nil, http.StatusOK},
		{"archived/missing candidate returns 400", fmt.Errorf("x: %w", services.ErrCannotPromoteWallet), http.StatusBadRequest},
		{"not found returns 404", fmt.Errorf("x: %w", data.ErrRecordNotFound), http.StatusNotFound},
		{"unexpected error returns 500", errors.New("boom"), http.StatusInternalServerError},
	}
	for _, tc := range promoteCases {
		t.Run("promote: "+tc.name, func(t *testing.T) {
			handler := DistributionWalletsHandler{AuthManager: newWalletScopeOwnerMock(), Service: &mockDistributionWalletService{
				promoteFn: func(_ context.Context, id string, _ []string) (*data.DistributionWallet, error) {
					assert.Equal(t, "dw-2", id)
					if tc.serviceErr != nil {
						return nil, tc.serviceErr
					}
					return &data.DistributionWallet{ID: "dw-2", IsDefault: true}, nil
				},
			}}
			rr := do(handler, "/distribution-wallets/dw-2/promote-to-default")
			assert.Equal(t, tc.wantStatusCode, rr.Code)
		})
	}
}

// Test_DistributionWalletsHandler_walletBalances_resolvesCircleFromTenant pins the balance read
// to the shared account resolver. A Circle tenant's distribution_wallets row is blank and
// PENDING_USER_ACTIVATION by design, so reading Address/Type/Status off the row returned an empty
// balance map for every Circle tenant — while the legacy GET /balances, which resolves from the
// tenant context, returned the real one. Two endpoints, same tenant, disagreeing.
func Test_DistributionWalletsHandler_walletBalances_resolvesCircleFromTenant(t *testing.T) {
	usdc := data.Asset{Code: "USDC", Issuer: "GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5"}
	circleAccount := schema.TransactionAccount{
		CircleWalletID: "circle-wallet-1",
		Type:           schema.DistributionAccountCircleDBVault,
		Status:         schema.AccountStatusActive,
	}
	// Exactly the row provisionDistributionAccount leaves behind for a Circle tenant.
	circleWalletRow := &data.DistributionWallet{
		ID:            "dw-circle",
		Name:          "default",
		AccountType:   schema.DistributionAccountCircleDBVault,
		AccountStatus: schema.AccountStatusPendingUserActivation,
		Status:        data.ActiveDistributionWalletStatus,
	}

	newHandler := func(t *testing.T, wallet *data.DistributionWallet) DistributionWalletsHandler {
		t.Helper()

		resolverMock := sigMocks.NewMockDistributionAccountResolver(t)
		resolverMock.On("DistributionAccountFromContext", mock.Anything).Return(circleAccount, nil).Maybe()

		distAccSvcMock := &svcMocks.MockDistributionAccountService{}
		distAccSvcMock.On("GetBalances", mock.Anything, mock.MatchedBy(func(account *schema.TransactionAccount) bool {
			// The Circle service locates the money by wallet ID, which only the tenant's
			// account carries — the wallet row has no column for it.
			return account.CircleWalletID == circleAccount.CircleWalletID
		})).Return(map[data.Asset]decimal.Decimal{usdc: decimal.NewFromInt(30)}, nil).Maybe()

		return DistributionWalletsHandler{
			AuthManager:                 newWalletScopeOwnerMock(),
			DistributionAccountService:  distAccSvcMock,
			DistributionAccountResolver: resolverMock,
			Service: &mockDistributionWalletService{
				getFn: func(_ context.Context, _ string) (*data.DistributionWallet, error) {
					return wallet, nil
				},
				listFn: func(_ context.Context, _ bool) ([]data.DistributionWallet, error) {
					return []data.DistributionWallet{*wallet}, nil
				},
			},
		}
	}

	get := func(path string, route string, h http.HandlerFunc) *httptest.ResponseRecorder {
		r := chi.NewRouter()
		r.Get(route, h)
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req = req.WithContext(sdpcontext.SetUserIDInContext(req.Context(), "payments-test-owner"))
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)
		return rr
	}

	t.Run("🎉 per-wallet balance reports the tenant's Circle account, not the blank row", func(t *testing.T) {
		handler := newHandler(t, circleWalletRow)
		rr := get("/distribution-wallets/dw-circle/balance",
			"/distribution-wallets/{id}/balance", handler.GetDistributionWalletBalance)

		require.Equal(t, http.StatusOK, rr.Code)
		assert.Contains(t, rr.Body.String(), `"USDC:GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5": "30"`)
	})

	t.Run("🎉 the Total Balance tile aggregates it too", func(t *testing.T) {
		handler := newHandler(t, circleWalletRow)
		rr := get("/distribution-wallets/balance",
			"/distribution-wallets/balance", handler.GetDistributionWalletsTotalBalance)

		require.Equal(t, http.StatusOK, rr.Code)
		assert.Contains(t, rr.Body.String(), `"USDC:GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5": "30"`)
	})

	t.Run("an unprovisioned Stellar wallet still reports an empty balance set, not an error", func(t *testing.T) {
		// A read moves no funds, so the 400 the write path raises for a wallet with no funded
		// account would be wrong here: pending-activation wallets simply hold nothing.
		handler := newHandler(t, &data.DistributionWallet{
			ID:            "dw-pending",
			AccountType:   schema.DistributionAccountStellarDBVault,
			AccountStatus: schema.AccountStatusPendingUserActivation,
			Status:        data.ActiveDistributionWalletStatus,
		})
		rr := get("/distribution-wallets/dw-pending/balance",
			"/distribution-wallets/{id}/balance", handler.GetDistributionWalletBalance)

		require.Equal(t, http.StatusOK, rr.Code)
		assert.Contains(t, rr.Body.String(), `"balances": {}`)
	})
}

// Test_DistributionWalletsHandler_PostDistributionWalletMembership_checkOrder pins the order the
// grant handler establishes facts in: the caller first, then the wallet, then the grantee. The
// grantee lookup answers questions about a request-supplied user_id (existence, global role) on a
// route whose permission does not imply read:users, so it must never run for a caller who has not
// been authenticated, nor before the wallet named in the path has been shown to exist.
func Test_DistributionWalletsHandler_PostDistributionWalletMembership_checkOrder(t *testing.T) {
	post := func(handler DistributionWalletsHandler, walletID, userID string) *httptest.ResponseRecorder {
		r := chi.NewRouter()
		r.Post("/distribution-wallets/{id}/memberships", handler.PostDistributionWalletMembership)
		req := httptest.NewRequest(http.MethodPost, "/distribution-wallets/"+walletID+"/memberships",
			strings.NewReader(fmt.Sprintf(`{"user_id": %q, "role": "approver"}`, userID)))
		req = req.WithContext(sdpcontext.SetUserIDInContext(req.Context(), "caller-1"))
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)
		return rr
	}

	t.Run("a caller who no longer exists gets 401, not an answer about the grantee", func(t *testing.T) {
		authManagerMock := &auth.AuthManagerMock{}
		authManagerMock.On("GetUserByID", mock.Anything, "caller-1").
			Return(nil, fmt.Errorf("getting: %w", auth.ErrUserNotFound))
		authManagerMock.On("GetUserByID", mock.Anything, "grantee-x").
			Run(func(mock.Arguments) { t.Error("the grantee was looked up for an unauthenticated caller") }).
			Return(nil, fmt.Errorf("getting: %w", auth.ErrUserNotFound)).Maybe()

		handler := DistributionWalletsHandler{AuthManager: authManagerMock, Service: &mockDistributionWalletService{}}

		rr := post(handler, "dw-1", "grantee-x")
		require.Equal(t, http.StatusUnauthorized, rr.Code)
		assert.NotContains(t, rr.Body.String(), "grantee-x", "no user-existence oracle before authentication")
	})

	t.Run("an unknown wallet is reported as such, not as a verdict on the grantee", func(t *testing.T) {
		authManagerMock := &auth.AuthManagerMock{}
		authManagerMock.On("GetUserByID", mock.Anything, "caller-1").
			Return(&auth.User{ID: "caller-1", IsOwner: true}, nil)
		authManagerMock.On("GetUserByID", mock.Anything, "grantee-x").
			Run(func(mock.Arguments) { t.Error("the grantee was looked up before the wallet was validated") }).
			Return(nil, fmt.Errorf("getting: %w", auth.ErrUserNotFound)).Maybe()

		handler := DistributionWalletsHandler{AuthManager: authManagerMock, Service: &mockDistributionWalletService{
			getFn: func(context.Context, string) (*data.DistributionWallet, error) {
				return nil, fmt.Errorf("getting: %w", data.ErrRecordNotFound)
			},
		}}

		rr := post(handler, "nope", "grantee-x")
		require.Equal(t, http.StatusNotFound, rr.Code)
		assert.Contains(t, rr.Body.String(), "distribution wallet not found")
		assert.NotContains(t, rr.Body.String(), "grantee-x")
	})
}
