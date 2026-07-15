package httphandler

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/sdpcontext"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/services"
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
			handler := DistributionWalletsHandler{Service: &mockDistributionWalletService{
				createFn: func(_ context.Context, insert data.DistributionWalletInsert) (*data.DistributionWallet, error) {
					if tc.serviceErr != nil {
						return nil, tc.serviceErr
					}
					assert.Equal(t, "program-a", insert.Name)
					return wallet, nil
				},
			}}

			req := httptest.NewRequest(http.MethodPost, "/distribution-wallets", strings.NewReader(tc.reqBody))
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
		handler := DistributionWalletsHandler{Service: &mockDistributionWalletService{
			getFn: func(_ context.Context, id string) (*data.DistributionWallet, error) {
				require.Equal(t, "dw-123", id)
				return &data.DistributionWallet{ID: "dw-123", Name: "program-a"}, nil
			},
		}}

		req := httptest.NewRequest(http.MethodGet, "/distribution-wallets/dw-123", nil)
		rr := httptest.NewRecorder()
		newRouter(handler).ServeHTTP(rr, req)

		assert.Equal(t, http.StatusOK, rr.Code)
		assert.Contains(t, rr.Body.String(), `"id": "dw-123"`)
	})

	t.Run("missing wallet returns 404", func(t *testing.T) {
		handler := DistributionWalletsHandler{Service: &mockDistributionWalletService{
			getFn: func(_ context.Context, _ string) (*data.DistributionWallet, error) {
				return nil, fmt.Errorf("getting: %w", data.ErrRecordNotFound)
			},
		}}

		req := httptest.NewRequest(http.MethodGet, "/distribution-wallets/nope", nil)
		rr := httptest.NewRecorder()
		newRouter(handler).ServeHTTP(rr, req)

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
			handler := DistributionWalletsHandler{Service: &mockDistributionWalletService{
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
			handler := DistributionWalletsHandler{
				AuthManager: grantorMock,
				Service: &mockDistributionWalletService{
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
		handler := DistributionWalletsHandler{Service: &mockDistributionWalletService{
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
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)
		assert.Equal(t, http.StatusOK, rr.Code)
		assert.Contains(t, rr.Body.String(), `"m-1"`)

		req = httptest.NewRequest(http.MethodGet, "/distribution-wallets/missing/memberships", nil)
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
			handler := DistributionWalletsHandler{Service: &mockDistributionWalletService{
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
