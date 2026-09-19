package httphandler

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/db/dbtest"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/sdpcontext"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/services"
	"github.com/stellar/stellar-disbursement-platform-backend/pkg/schema"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-auth/pkg/auth"
)

// Test_WalletTransitionGates proves each state transition is gated on the source wallet: users need
// a qualifying role on it, API keys need it in scope, Owners pass everywhere.
func Test_WalletTransitionGates(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := sdpcontext.SetTenantInContext(context.Background(), &schema.Tenant{ID: "gate-tenant", Name: "gate-tenant"})
	models, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)

	walletA := data.EnsureDefaultDistributionWalletFixture(t, ctx, dbConnectionPool)
	var walletBID string
	require.NoError(t, dbConnectionPool.GetContext(ctx, &walletBID, `
		INSERT INTO distribution_wallets (name, distribution_account_type)
		VALUES ('gate-wallet-b', 'DISTRIBUTION_ACCOUNT.STELLAR.DB_VAULT') RETURNING id`))

	// memberA holds EVERY wallet-scopable role on A — only the wallet dimension can fail.
	memberA := &auth.User{ID: "gate-member-a", Email: "a@gate.test", Roles: []string{
		string(data.FinancialControllerUserRole), string(data.InitiatorUserRole), string(data.BusinessUserRole),
	}}
	for _, role := range []data.UserRole{data.FinancialControllerUserRole, data.InitiatorUserRole, data.BusinessUserRole} {
		_, mErr := models.WalletMemberships.Insert(ctx, dbConnectionPool, memberA.ID, walletA.ID, role, nil)
		require.NoError(t, mErr)
	}
	owner := &auth.User{ID: "gate-owner", Email: "o@gate.test", IsOwner: true}

	// Single-role actors for the disbursement status matrix; each is granted its role on one wallet.
	newActor := func(id string, role data.UserRole, walletID string) *auth.User {
		if walletID != "" {
			_, mErr := models.WalletMemberships.Insert(ctx, dbConnectionPool, id, walletID, role, nil)
			require.NoError(t, mErr)
		}
		return &auth.User{ID: id, Email: id + "@gate.test", Roles: []string{string(role)}}
	}
	approverOnA := newActor("gate-approver-a", data.ApproverUserRole, walletA.ID)
	initiatorOnA := newActor("gate-initiator-a", data.InitiatorUserRole, walletA.ID)
	approverOnB := newActor("gate-approver-b", data.ApproverUserRole, walletBID)
	approverNoWallet := newActor("gate-approver-none", data.ApproverUserRole, "")
	developer := newActor("gate-developer", data.DeveloperUserRole, "")

	// An API key answers from its own scope: its creator's role and memberships are irrelevant.
	ownerKeyOnA := &data.APIKey{ID: "gate-owner-key-a", CreatedBy: owner.ID, DistributionWalletIDs: []string{walletA.ID}}
	ownerKeyUnscoped := &data.APIKey{ID: "gate-owner-key-unscoped", CreatedBy: owner.ID}
	developerKeyOnB := &data.APIKey{ID: "gate-developer-key-b", CreatedBy: developer.ID, DistributionWalletIDs: []string{walletBID}}

	authManagerMock := &auth.AuthManagerMock{}
	for _, actor := range []*auth.User{memberA, owner, approverOnA, initiatorOnA, approverOnB, approverNoWallet, developer} {
		authManagerMock.On("GetUserByID", mock.Anything, actor.ID).Return(actor, nil)
	}
	authManagerMock.On("GetUser", mock.Anything, mock.Anything).Return(memberA, nil).Maybe()

	disbursementHandler := DisbursementHandler{
		Models:                        models,
		AuthManager:                   authManagerMock,
		DisbursementManagementService: &services.DisbursementManagementService{Models: models, AuthManager: authManagerMock},
	}
	paymentsHandler := PaymentsHandler{Models: models, DBConnectionPool: dbConnectionPool, AuthManager: authManagerMock}

	r := chi.NewRouter()
	r.Delete("/disbursements/{id}", disbursementHandler.DeleteDisbursement)
	r.Patch("/disbursements/{id}/status", disbursementHandler.PatchDisbursementStatus)
	r.Patch("/payments/{id}/status", paymentsHandler.PatchPaymentStatus)
	r.Patch("/payments/retry", paymentsHandler.RetryPayments)

	// A nil apiKey is the JWT path; otherwise the request carries the key and no token.
	doAsPrincipal := func(userID string, apiKey *data.APIKey, method, path, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		reqCtx := sdpcontext.SetUserIDInContext(ctx, userID)
		if apiKey != nil {
			reqCtx = sdpcontext.SetAPIKeyInContext(reqCtx, apiKey)
		} else {
			reqCtx = sdpcontext.SetTokenInContext(reqCtx, "test-token")
		}
		req = req.WithContext(reqCtx)
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)
		return rr
	}
	doAs := func(userID, method, path, body string) *httptest.ResponseRecorder {
		return doAsPrincipal(userID, nil, method, path, body)
	}

	newDisbursement := func(name, walletID string, status data.DisbursementStatus) *data.Disbursement {
		return data.CreateDisbursementFixture(t, ctx, dbConnectionPool, models.Disbursements, &data.Disbursement{
			Name: name, SourceWalletID: walletID, Status: status,
		})
	}
	newPayment := func(d *data.Disbursement, status data.PaymentStatus) *data.Payment {
		receiver := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{})
		rw := data.CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver.ID, d.Wallet.ID, data.ReadyReceiversWalletStatus)
		return data.CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &data.Payment{
			ReceiverWallet: rw, Disbursement: d, Asset: *d.Asset, Amount: "7", Status: status,
		})
	}

	t.Run("cancel payment: cross-wallet 403, same-wallet passes the gate, owner passes", func(t *testing.T) {
		paymentB := newPayment(newDisbursement("gate-cancel-b", walletBID, data.StartedDisbursementStatus), data.ReadyPaymentStatus)
		rr := doAs(memberA.ID, http.MethodPatch, "/payments/"+paymentB.ID+"/status", `{"status": "CANCELED"}`)
		assert.Equal(t, http.StatusForbidden, rr.Code)
		assert.NotContains(t, rr.Body.String(), walletBID)

		paymentA := newPayment(newDisbursement("gate-cancel-a", walletA.ID, data.StartedDisbursementStatus), data.ReadyPaymentStatus)
		rr = doAs(memberA.ID, http.MethodPatch, "/payments/"+paymentA.ID+"/status", `{"status": "CANCELED"}`)
		assert.Equal(t, http.StatusOK, rr.Code)

		rr = doAs(owner.ID, http.MethodPatch, "/payments/"+paymentB.ID+"/status", `{"status": "CANCELED"}`)
		assert.Equal(t, http.StatusOK, rr.Code)
	})

	t.Run("retry payments: any cross-wallet payment in the batch rejects the whole request", func(t *testing.T) {
		failedA := newPayment(newDisbursement("gate-retry-a", walletA.ID, data.StartedDisbursementStatus), data.FailedPaymentStatus)
		failedB := newPayment(newDisbursement("gate-retry-b", walletBID, data.StartedDisbursementStatus), data.FailedPaymentStatus)

		body := fmt.Sprintf(`{"payment_ids": [%q, %q]}`, failedA.ID, failedB.ID)
		rr := doAs(memberA.ID, http.MethodPatch, "/payments/retry", body)
		assert.Equal(t, http.StatusForbidden, rr.Code)

		// Same-wallet-only batch succeeds.
		body = fmt.Sprintf(`{"payment_ids": [%q]}`, failedA.ID)
		rr = doAs(memberA.ID, http.MethodPatch, "/payments/retry", body)
		assert.Equal(t, http.StatusOK, rr.Code)
	})

	t.Run("delete draft disbursement: cross-wallet 403, same-wallet OK", func(t *testing.T) {
		draftB := newDisbursement("gate-delete-b", walletBID, data.DraftDisbursementStatus)
		rr := doAs(memberA.ID, http.MethodDelete, "/disbursements/"+draftB.ID, "")
		assert.Equal(t, http.StatusForbidden, rr.Code)

		draftA := newDisbursement("gate-delete-a", walletA.ID, data.DraftDisbursementStatus)
		rr = doAs(memberA.ID, http.MethodDelete, "/disbursements/"+draftA.ID, "")
		assert.Equal(t, http.StatusOK, rr.Code)
	})

	const (
		start  = "STARTED"
		pause  = "PAUSED"
		cancel = "CANCELED"
	)
	fromStatus := map[string]data.DisbursementStatus{
		start:  data.ReadyDisbursementStatus,
		pause:  data.StartedDisbursementStatus,
		cancel: data.ReadyDisbursementStatus,
	}
	patchStatus := func(actor *auth.User, apiKey *data.APIKey, disbursementID, status string) *httptest.ResponseRecorder {
		return doAsPrincipal(actor.ID, apiKey, http.MethodPatch, "/disbursements/"+disbursementID+"/status", fmt.Sprintf(`{"status": %q}`, status))
	}

	t.Run("disbursement status: one case per (action × principal × wallet)", func(t *testing.T) {
		matrix := []struct {
			action       string
			actor        *auth.User
			apiKey       *data.APIKey
			targetWallet string
			wantGate403  bool
		}{
			// Pause a disbursement sourced from wallet A:
			{pause, approverOnA, nil, walletA.ID, false},
			{pause, memberA, nil, walletA.ID, false},     // financial controller on A
			{pause, initiatorOnA, nil, walletA.ID, true}, // initiator does not qualify for transitions
			{pause, approverOnB, nil, walletA.ID, true},  // cross-wallet
			{pause, approverNoWallet, nil, walletA.ID, true},
			{pause, owner, nil, walletA.ID, false},
			// Pause a disbursement sourced from wallet B:
			{pause, approverOnA, nil, walletBID, true}, // cross-wallet (the canonical AC)
			{pause, approverOnB, nil, walletBID, false},
			{pause, owner, nil, walletBID, false},
			// Start (the gate fires before any transition/balance logic):
			{start, approverOnA, nil, walletBID, true},
			{start, approverNoWallet, nil, walletA.ID, true},
			{start, approverOnB, nil, walletA.ID, true},
			// Cancel:
			{cancel, approverOnA, nil, walletBID, true},
			{cancel, approverOnB, nil, walletBID, false},
			// API keys: an owner-minted key does not inherit its creator's tenant-wide reach…
			{start, owner, ownerKeyOnA, walletBID, true},
			{pause, owner, ownerKeyOnA, walletBID, true},
			{cancel, owner, ownerKeyOnA, walletBID, true},
			{pause, owner, ownerKeyUnscoped, walletA.ID, true},
			{pause, owner, ownerKeyOnA, walletA.ID, false},
			// …and a key in scope needs no role or membership from its creator.
			{pause, developer, developerKeyOnB, walletBID, false},
			{cancel, developer, developerKeyOnB, walletBID, false},
			{pause, developer, developerKeyOnB, walletA.ID, true},
		}

		for i, tc := range matrix {
			principal := tc.actor.ID
			if tc.apiKey != nil {
				principal = tc.apiKey.ID
			}
			name := fmt.Sprintf("%s as %s on wallet=%s gate403=%v", tc.action, principal, short(tc.targetWallet, walletA.ID, walletBID), tc.wantGate403)
			t.Run(name, func(t *testing.T) {
				disbursement := newDisbursement(fmt.Sprintf("gate-status-%d-%s", i, principal), tc.targetWallet, fromStatus[tc.action])

				rr := patchStatus(tc.actor, tc.apiKey, disbursement.ID, tc.action)

				got, gErr := models.Disbursements.Get(ctx, dbConnectionPool, disbursement.ID)
				require.NoError(t, gErr)
				if tc.wantGate403 {
					require.Equal(t, http.StatusForbidden, rr.Code, rr.Body.String())
					require.Contains(t, rr.Body.String(), services.ErrWalletActionForbidden.Error())
					require.Equal(t, fromStatus[tc.action], got.Status, "a rejected transition must leave the status untouched")
				} else {
					require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
					require.NotEqual(t, fromStatus[tc.action], got.Status)
				}
			})
		}
	})

	t.Run("disbursement status: an explicit grant on the wallet unlocks the action", func(t *testing.T) {
		disbursement := newDisbursement("gate-status-grant-unlocks", walletBID, data.StartedDisbursementStatus)

		rr := patchStatus(approverNoWallet, nil, disbursement.ID, pause)
		require.Equal(t, http.StatusForbidden, rr.Code, rr.Body.String())

		_, mErr := models.WalletMemberships.Insert(ctx, dbConnectionPool, approverNoWallet.ID, walletBID, data.ApproverUserRole, nil)
		require.NoError(t, mErr)

		rr = patchStatus(approverNoWallet, nil, disbursement.ID, pause)
		require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
		got, gErr := models.Disbursements.Get(ctx, dbConnectionPool, disbursement.ID)
		require.NoError(t, gErr)
		require.Equal(t, data.PausedDisbursementStatus, got.Status)
	})
}

func short(id string, a, b string) string {
	switch id {
	case a:
		return "A"
	case b:
		return "B"
	default:
		return id
	}
}
