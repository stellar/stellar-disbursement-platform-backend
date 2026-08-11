package httphandler

import (
	"context"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/db/dbtest"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/sdpcontext"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/middleware"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-auth/pkg/auth"
)

func Test_walletCapabilitiesFor(t *testing.T) {
	testCases := []struct {
		name            string
		user            *auth.User
		membershipRoles []data.UserRole
		want            map[string]bool
	}{
		{
			name: "owners are tenant-wide: every capability, no membership",
			user: &auth.User{ID: "o", IsOwner: true},
			want: map[string]bool{
				"can_create_disbursement": true, "can_start_disbursement": true,
				"can_pause_disbursement": true, "can_cancel_disbursement": true,
				"can_create_payment": true, "can_retry_payment": true, "can_cancel_payment": true,
			},
		},
		{
			name:            "financial controller narrowed to approver: approve, but do not create",
			user:            &auth.User{ID: "u", Roles: []string{string(data.FinancialControllerUserRole)}},
			membershipRoles: []data.UserRole{data.ApproverUserRole},
			want: map[string]bool{
				"can_create_disbursement": false, "can_start_disbursement": true,
				"can_pause_disbursement": true, "can_cancel_disbursement": true,
				"can_create_payment": false, "can_retry_payment": false, "can_cancel_payment": false,
			},
		},
		{
			name:            "initiator granted financial controller: the global role still caps them at create",
			user:            &auth.User{ID: "u", Roles: []string{string(data.InitiatorUserRole)}},
			membershipRoles: []data.UserRole{data.FinancialControllerUserRole},
			want: map[string]bool{
				"can_create_disbursement": true, "can_start_disbursement": false,
				"can_pause_disbursement": false, "can_cancel_disbursement": false,
				"can_create_payment": false, "can_retry_payment": false, "can_cancel_payment": false,
			},
		},
		{
			name: "financial controller with no membership on this account gets nothing",
			user: &auth.User{ID: "u", Roles: []string{string(data.FinancialControllerUserRole)}},
			want: map[string]bool{
				"can_create_disbursement": false, "can_start_disbursement": false,
				"can_pause_disbursement": false, "can_cancel_disbursement": false,
				"can_create_payment": false, "can_retry_payment": false, "can_cancel_payment": false,
			},
		},
		{
			name:            "developer granted approver: the narrowing leaves nothing",
			user:            &auth.User{ID: "u", Roles: []string{string(data.DeveloperUserRole)}},
			membershipRoles: []data.UserRole{data.ApproverUserRole},
			want: map[string]bool{
				"can_create_disbursement": false, "can_start_disbursement": false,
				"can_pause_disbursement": false, "can_cancel_disbursement": false,
				"can_create_payment": false, "can_retry_payment": false, "can_cancel_payment": false,
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, walletCapabilitiesFor(tc.user, tc.membershipRoles))
		})
	}
}

// Test_APIKeyWalletReadScope covers the hole the Owner-only route gate left open: on the API-key
// path RequirePermission never calls the role middleware it was handed, so a key with
// read:distribution_wallets reaches the admin reads directly. The handlers must therefore
// re-apply, themselves, what the route gate only intended. The creator here is deliberately not
// an owner — an owner short-circuits every membership lookup and would pass regardless of what
// the handlers do.
//
// The two gates differ by endpoint, because the endpoints differ in kind: the three admin reads
// are Owner-only and refuse a non-owner outright, while /{id}/capabilities reports the caller's
// own capabilities and so is merely membership-scoped.
func Test_APIKeyWalletReadScope(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()
	models, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)

	walletA := data.EnsureDefaultDistributionWalletFixture(t, ctx, dbConnectionPool)
	var walletBID string
	require.NoError(t, dbConnectionPool.GetContext(ctx, &walletBID, `
		INSERT INTO distribution_wallets (name, distribution_account_type)
		VALUES ('api-key-wallet-b', 'DISTRIBUTION_ACCOUNT.STELLAR.DB_VAULT') RETURNING id`))

	creator := &auth.User{ID: "api-key-creator", Email: "k@example.com", Roles: []string{string(data.BusinessUserRole)}}
	_, err = models.WalletMemberships.Insert(ctx, dbConnectionPool, creator.ID, walletA.ID, data.BusinessUserRole, nil)
	require.NoError(t, err)

	authManagerMock := &auth.AuthManagerMock{}
	authManagerMock.On("GetUserByID", mock.Anything, creator.ID).Return(creator, nil)

	handler := DistributionWalletsHandler{
		AuthManager: authManagerMock,
		Models:      models,
		Service: &mockDistributionWalletService{
			getFn: func(_ context.Context, id string) (*data.DistributionWallet, error) {
				return &data.DistributionWallet{ID: id, Name: "api-key-wallet-b"}, nil
			},
			listMembershipsFn: func(_ context.Context, walletID string) ([]data.WalletMembership, error) {
				return []data.WalletMembership{{ID: "leaked-membership", WalletID: walletID}}, nil
			},
			listAuditFn: func(_ context.Context, walletID string) ([]data.WalletMembershipAuditEntry, error) {
				return []data.WalletMembershipAuditEntry{{
					WalletMembership: data.WalletMembership{ID: "leaked-audit", WalletID: walletID},
				}}, nil
			},
		},
	}

	// The route gate exactly as the admin reads ship it in serve.go.
	r := chi.NewRouter()
	r.With(middleware.RequirePermission(
		data.ReadDistributionWallets,
		middleware.AnyRoleMiddleware(authManagerMock, data.OwnerUserRole),
	)).Group(func(r chi.Router) {
		r.Get("/distribution-wallets/{id}", handler.GetDistributionWallet)
		r.Get("/distribution-wallets/{id}/memberships", handler.GetDistributionWalletMemberships)
		r.Get("/distribution-wallets/{id}/audit", handler.GetDistributionWalletAudit)
		r.Get("/distribution-wallets/{id}/capabilities", handler.GetDistributionWalletCapabilities)
	})

	apiKey := &data.APIKey{
		ID:          "api-key-1",
		Name:        "reader",
		CreatedBy:   creator.ID,
		Permissions: data.APIKeyPermissions{data.ReadDistributionWallets},
	}
	withAPIKey := func(path string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		reqCtx := sdpcontext.SetAPIKeyInContext(req.Context(), apiKey)
		reqCtx = sdpcontext.SetUserIDInContext(reqCtx, apiKey.CreatedBy)
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req.WithContext(reqCtx))
		return rr
	}

	// The admin reads are Owner-only, so a key created by a non-owner is refused whether or not
	// the wallet is in the creator's scope: scope alone would have admitted them to walletA,
	// where they hold a membership, and handed them its roster and its grant/revoke history.
	adminReads := map[string]string{
		"wallet":      "",
		"memberships": "/memberships",
		"audit":       "/audit",
	}
	scopes := map[string]string{"in the creator's scope": walletA.ID, "outside it": walletBID}
	for name, suffix := range adminReads {
		for scopeName, walletID := range scopes {
			t.Run(fmt.Sprintf("%s: owner-only, %s returns 403", name, scopeName), func(t *testing.T) {
				rr := withAPIKey(fmt.Sprintf("/distribution-wallets/%s%s", walletID, suffix))
				assert.Equal(t, http.StatusForbidden, rr.Code)
				assert.NotContains(t, rr.Body.String(), "api-key-wallet-b")
				assert.NotContains(t, rr.Body.String(), "leaked-")
			})
		}
	}

	t.Run("capabilities: outside the creator's scope returns 404", func(t *testing.T) {
		rr := withAPIKey(fmt.Sprintf("/distribution-wallets/%s/capabilities", walletBID))
		assert.Equal(t, http.StatusNotFound, rr.Code)
		assert.NotContains(t, rr.Body.String(), "api-key-wallet-b")
	})

	t.Run("capabilities: inside the creator's scope it is served, and follows the creator's "+
		"membership role rather than the key's permissions", func(t *testing.T) {
		rr := withAPIKey(fmt.Sprintf("/distribution-wallets/%s/capabilities", walletA.ID))
		require.Equal(t, http.StatusOK, rr.Code)
		body := rr.Body.String()
		// Global business + business membership: direct payments, nothing on disbursements.
		assert.Contains(t, body, `"can_create_payment": true`)
		assert.Contains(t, body, `"can_create_disbursement": false`)
		assert.Contains(t, body, `"can_start_disbursement": false`)
	})
}

// capabilityEnforcementSites points each matrix entry at the code that actually enforces it:
// the handler registered in serve.go (whose AnyRoleMiddleware declares the tenant-role gate)
// and the function holding the wallet-membership gate.
var capabilityEnforcementSites = map[string]struct {
	routeHandler string
	file         string
	function     string
}{
	"can_create_disbursement": {"PostDisbursement", "disbursement_handler.go", "createNewDisbursement"},
	"can_start_disbursement":  {"PatchDisbursementStatus", "../../services/disbursement_management_service.go", "StartDisbursement"},
	"can_pause_disbursement":  {"PatchDisbursementStatus", "../../services/disbursement_management_service.go", "PauseDisbursement"},
	"can_cancel_disbursement": {"PatchDisbursementStatus", "../../services/disbursement_management_service.go", "CancelDisbursement"},
	"can_create_payment":      {"PostDirectPayment", "payments_handler.go", "PostDirectPayment"},
	"can_retry_payment":       {"RetryPayments", "payments_handler.go", "RetryPayments"},
	"can_cancel_payment":      {"PatchPaymentStatus", "payments_handler.go", "PatchPaymentStatus"},
}

// Test_WalletCapabilityMatrix_Conformance keeps walletCapabilityMatrix honest by reading the
// role sets straight out of the enforcement sites: the route declarations in serve.go and the
// wallet-membership gates in the handlers and the disbursement service. If someone changes what
// a route or a gate requires, this fails and the matrix has to follow — the matrix is a
// projection of those rules for the client, never a second set of them.
func Test_WalletCapabilityMatrix_Conformance(t *testing.T) {
	routeRoles := parseRouteRoles(t, "../serve.go")
	gateRoles := map[string]map[string][]data.UserRole{}

	require.Len(t, walletCapabilityMatrix, len(capabilityEnforcementSites))
	for _, capability := range walletCapabilityMatrix {
		site, ok := capabilityEnforcementSites[capability.name]
		require.Truef(t, ok, "capability %q has no enforcement site: point it at one", capability.name)

		if _, parsed := gateRoles[site.file]; !parsed {
			gateRoles[site.file] = parseWalletGateRoles(t, site.file)
		}

		declaredRoles, routed := routeRoles[site.routeHandler]
		require.Truef(t, routed,
			"no route declaration found for %s in serve.go: the enforcement site moved, update capabilityEnforcementSites", site.routeHandler)
		gatedRoles, gated := gateRoles[site.file][site.function]
		require.Truef(t, gated,
			"no wallet-membership gate found in %s (%s): the enforcement site moved, update capabilityEnforcementSites", site.file, site.function)

		assert.ElementsMatchf(t, declaredRoles, capability.globalRoles,
			"%s.globalRoles drifted from the route declaration for %s in serve.go", capability.name, site.routeHandler)
		assert.ElementsMatchf(t, gatedRoles, capability.walletRoles,
			"%s.walletRoles drifted from the wallet-membership gate in %s (%s)", capability.name, site.file, site.function)
	}
}

var roleByIdent = map[string]data.UserRole{
	"OwnerUserRole":               data.OwnerUserRole,
	"FinancialControllerUserRole": data.FinancialControllerUserRole,
	"DeveloperUserRole":           data.DeveloperUserRole,
	"BusinessUserRole":            data.BusinessUserRole,
	"InitiatorUserRole":           data.InitiatorUserRole,
	"ApproverUserRole":            data.ApproverUserRole,
}

// parseRouteRoles maps every routed handler name to the roles its AnyRoleMiddleware admits,
// reading the `r.With(RequirePermission(_, AnyRoleMiddleware(_, roles...))).Group/Method(...)`
// chains in serve.go.
func parseRouteRoles(t *testing.T, path string) map[string][]data.UserRole {
	t.Helper()

	file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	require.NoError(t, err)

	rolesByHandler := map[string][]data.UserRole{}
	ast.Inspect(file, func(n ast.Node) bool {
		chained, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		method, ok := chained.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		with, ok := method.X.(*ast.CallExpr)
		if !ok {
			return true
		}
		if withSelector, isSelector := with.Fun.(*ast.SelectorExpr); !isSelector || withSelector.Sel.Name != "With" {
			return true
		}

		roles := rolesFromAnyRoleMiddleware(with)
		if roles == nil {
			return true
		}
		for _, arg := range chained.Args {
			ast.Inspect(arg, func(inner ast.Node) bool {
				if selector, isSelector := inner.(*ast.SelectorExpr); isSelector {
					rolesByHandler[selector.Sel.Name] = roles
				}
				return true
			})
		}
		return true
	})
	return rolesByHandler
}

func rolesFromAnyRoleMiddleware(call *ast.CallExpr) []data.UserRole {
	var roles []data.UserRole
	ast.Inspect(call, func(n ast.Node) bool {
		inner, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		selector, ok := inner.Fun.(*ast.SelectorExpr)
		if !ok || selector.Sel.Name != "AnyRoleMiddleware" {
			return true
		}
		// arg 0 is the auth manager; the rest are the roles, either listed one by one or
		// spread from data.GetAllRoles()/data.GetBusinessOperationRoles().
		for _, arg := range inner.Args[1:] {
			switch role := arg.(type) {
			case *ast.SelectorExpr:
				if parsed, known := roleByIdent[role.Sel.Name]; known {
					roles = append(roles, parsed)
				}
			case *ast.CallExpr:
				if fn, isSelector := role.Fun.(*ast.SelectorExpr); isSelector {
					switch fn.Sel.Name {
					case "GetAllRoles":
						roles = append(roles, data.GetAllRoles()...)
					case "GetBusinessOperationRoles":
						roles = append(roles, data.GetBusinessOperationRoles()...)
					}
				}
			}
		}
		return false
	})
	return roles
}

// parseWalletGateRoles maps every function in a file to the roles it passes to the
// wallet-membership gate (ensureWalletActionAllowed / resolveSourceWalletForWrite /
// EnsureUserCanActOnWallet).
func parseWalletGateRoles(t *testing.T, path string) map[string][]data.UserRole {
	t.Helper()

	file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	require.NoError(t, err)

	gates := map[string]struct{}{
		"ensureWalletActionAllowed":   {},
		"resolveSourceWalletForWrite": {},
		"EnsureUserCanActOnWallet":    {},
	}

	rolesByFunction := map[string][]data.UserRole{}
	for _, decl := range file.Decls {
		funcDecl, ok := decl.(*ast.FuncDecl)
		if !ok {
			continue
		}
		ast.Inspect(funcDecl, func(n ast.Node) bool {
			call, isCall := n.(*ast.CallExpr)
			if !isCall {
				return true
			}
			ident, isIdent := call.Fun.(*ast.Ident)
			if !isIdent {
				return true
			}
			if _, isGate := gates[ident.Name]; !isGate {
				return true
			}
			for _, arg := range call.Args {
				if selector, isSelector := arg.(*ast.SelectorExpr); isSelector {
					if role, known := roleByIdent[selector.Sel.Name]; known {
						rolesByFunction[funcDecl.Name.Name] = append(rolesByFunction[funcDecl.Name.Name], role)
					}
				}
			}
			return true
		})
	}
	return rolesByFunction
}
