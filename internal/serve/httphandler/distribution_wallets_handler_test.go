package httphandler

import (
	"context"
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
	promoteFn         func(ctx context.Context, id string) (*data.DistributionWallet, error)
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

func (m *mockDistributionWalletService) PromoteToDefault(ctx context.Context, id string) (*data.DistributionWallet, error) {
	return m.promoteFn(ctx, id)
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

// Test_DistributionWalletsHandler_grantIsNotInert pins the grant invariant: a grant is rejected
// exactly when it confers nothing whatsoever, which is only true for a global owner.
//
// It is NOT "the grantee must already hold the requested role globally" — the two 🎉 cases are
// the ones such a rule would wrongly forbid. Nor is it "the write-capability set is empty": a
// membership is also the read-visibility grant (GetWalletIDsForUser ignores role), and for a
// developer it is the ONLY way they ever see an account, so the read-only cases below are real
// grants and must succeed. See walletGrantIsInert for the full reasoning.
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
			// Confers no WRITE capability, but it is the only way a developer sees the account
			// at all — serve.go admits developers to the membership-scoped reads, and those
			// filter on membership existence regardless of role.
			name:           "global developer + approver: read visibility is a real grant",
			granteeRoles:   []string{string(data.DeveloperUserRole)},
			role:           "approver",
			wantStatusCode: http.StatusCreated,
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
			// The one genuinely inert grant: owners short-circuit both the action gate and the
			// read scope, so the row changes nothing and the operator cannot tell.
			name:           "global owner grantee is rejected as inert",
			granteeRoles:   []string{string(data.OwnerUserRole)},
			role:           "financial_controller",
			wantStatusCode: http.StatusBadRequest,
			wantContains:   "grants nothing to an owner",
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
				promoteFn: func(_ context.Context, id string) (*data.DistributionWallet, error) {
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

// Test_DistributionWalletsHandler_writesRequireOwnerOnAPIKeyPath pins the fix for the write
// group's authorization hole. middleware.RequirePermission short-circuits straight to the handler
// once an API key carries the permission and never invokes the AnyRoleMiddleware(OwnerUserRole)
// the route handed it, so an owner-only write surface was reachable by any key — including one
// created by a non-owner, and including write:all, which HasPermission treats as a wildcard.
// Every probe on those routes mutates, so a bypass is both an escalation and an existence oracle.
func Test_DistributionWalletsHandler_writesRequireOwnerOnAPIKeyPath(t *testing.T) {
	keyCreator := &auth.User{ID: "key-creator", Roles: []string{string(data.DeveloperUserRole)}}

	routes := []struct {
		name   string
		method string
		path   string
		body   string
	}{
		{"create", http.MethodPost, "/distribution-wallets", `{"name": "escalation"}`},
		{"archive", http.MethodPost, "/distribution-wallets/dw-1/archive", ""},
		{"promote-to-default", http.MethodPost, "/distribution-wallets/dw-1/promote-to-default", ""},
		{"grant membership", http.MethodPost, "/distribution-wallets/dw-1/memberships", `{"user_id": "user-x", "role": "approver"}`},
		{"revoke membership", http.MethodDelete, "/distribution-wallets/dw-1/memberships/m-1", ""},
	}

	// write:distribution_wallets and the write:all wildcard both clear RequirePermission.
	permissionSets := []struct {
		name        string
		permissions data.APIKeyPermissions
	}{
		{"write:distribution_wallets", data.APIKeyPermissions{data.WriteDistributionWallets}},
		{"write:all wildcard", data.APIKeyPermissions{data.WriteAll}},
	}

	for _, permissionSet := range permissionSets {
		for _, route := range routes {
			t.Run(permissionSet.name+" / "+route.name, func(t *testing.T) {
				authManagerMock := &auth.AuthManagerMock{}
				authManagerMock.On("GetUserByID", mock.Anything, keyCreator.ID).Return(keyCreator, nil).Maybe()

				failIfCalled := func(operation string) {
					t.Errorf("%s reached the service: the write must be denied before it mutates", operation)
				}
				handler := DistributionWalletsHandler{
					AuthManager: authManagerMock,
					Service: &mockDistributionWalletService{
						createFn: func(context.Context, data.DistributionWalletInsert) (*data.DistributionWallet, error) {
							failIfCalled("CreateWallet")
							return nil, nil
						},
						archiveFn: func(context.Context, string) (*data.DistributionWallet, error) {
							failIfCalled("ArchiveWallet")
							return nil, nil
						},
						promoteFn: func(context.Context, string) (*data.DistributionWallet, error) {
							failIfCalled("PromoteToDefault")
							return nil, nil
						},
						grantFn: func(context.Context, string, string, data.UserRole, string) (*data.WalletMembership, error) {
							failIfCalled("GrantMembership")
							return nil, nil
						},
						revokeFn: func(context.Context, string, string, string) error {
							failIfCalled("RevokeMembership")
							return nil
						},
						getFn: func(context.Context, string) (*data.DistributionWallet, error) {
							failIfCalled("GetWallet")
							return nil, nil
						},
					},
				}

				// The route gate is wired exactly as serve.go wires the write group.
				r := chi.NewRouter()
				r.Group(func(r chi.Router) {
					r.Use(middleware.RequirePermission(
						data.WriteDistributionWallets,
						middleware.AnyRoleMiddleware(authManagerMock, data.OwnerUserRole),
					))
					r.Post("/distribution-wallets", handler.PostDistributionWallet)
					r.Post("/distribution-wallets/{id}/archive", handler.PostArchiveDistributionWallet)
					r.Post("/distribution-wallets/{id}/promote-to-default", handler.PostPromoteDistributionWalletToDefault)
					r.Post("/distribution-wallets/{id}/memberships", handler.PostDistributionWalletMembership)
					r.Delete("/distribution-wallets/{id}/memberships/{membershipID}", handler.DeleteDistributionWalletMembership)
				})

				var body io.Reader
				if route.body != "" {
					body = strings.NewReader(route.body)
				}
				req := httptest.NewRequest(route.method, route.path, body)
				// Exactly what APIKeyAuthenticator.Middleware puts in the context: the key, and
				// its creator as the acting user.
				ctx := sdpcontext.SetAPIKeyInContext(req.Context(), &data.APIKey{
					ID:          "ak-1",
					Permissions: permissionSet.permissions,
					CreatedBy:   keyCreator.ID,
				})
				ctx = sdpcontext.SetUserIDInContext(ctx, keyCreator.ID)
				rr := httptest.NewRecorder()
				r.ServeHTTP(rr, req.WithContext(ctx))

				assert.Equal(t, http.StatusForbidden, rr.Code,
					"a key whose creator is not an Owner must not reach an owner-only write")
				assert.NotContains(t, rr.Body.String(), "dw-1", "the response discloses no wallet detail")
			})
		}
	}

	t.Run("a key whose creator no longer exists is rejected, not passed through", func(t *testing.T) {
		authManagerMock := &auth.AuthManagerMock{}
		authManagerMock.On("GetUserByID", mock.Anything, "deleted-user").
			Return(nil, fmt.Errorf("getting: %w", auth.ErrUserNotFound))

		handler := DistributionWalletsHandler{
			AuthManager: authManagerMock,
			Service: &mockDistributionWalletService{
				createFn: func(context.Context, data.DistributionWalletInsert) (*data.DistributionWallet, error) {
					t.Error("CreateWallet reached the service with no resolvable acting user")
					return nil, nil
				},
			},
		}

		req := httptest.NewRequest(http.MethodPost, "/distribution-wallets", strings.NewReader(`{"name": "orphan"}`))
		ctx := sdpcontext.SetAPIKeyInContext(req.Context(), &data.APIKey{
			ID: "ak-2", Permissions: data.APIKeyPermissions{data.WriteAll}, CreatedBy: "deleted-user",
		})
		ctx = sdpcontext.SetUserIDInContext(ctx, "deleted-user")
		rr := httptest.NewRecorder()
		handler.PostDistributionWallet(rr, req.WithContext(ctx))

		assert.Equal(t, http.StatusUnauthorized, rr.Code)
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
