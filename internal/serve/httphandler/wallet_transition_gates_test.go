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
	"github.com/stellar/stellar-disbursement-platform-backend/pkg/schema"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-auth/pkg/auth"
)

// Test_W3_TransitionGates_MismatchedWallet proves "Approval or initiation against a mismatched
// wallet is rejected at every state transition" for the payment-level and lifecycle actions:
// cancel payment, retry payments, delete draft disbursement, upload instructions. A fully
// role-qualified member of wallet A gets 403 on entities sourced from wallet B; the same
// member succeeds on wallet A; Owners pass everywhere.
func Test_W3_TransitionGates_MismatchedWallet(t *testing.T) {
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

	authManagerMock := &auth.AuthManagerMock{}
	authManagerMock.On("GetUserByID", mock.Anything, memberA.ID).Return(memberA, nil)
	authManagerMock.On("GetUserByID", mock.Anything, owner.ID).Return(owner, nil)
	authManagerMock.On("GetUser", mock.Anything, mock.Anything).Return(memberA, nil).Maybe()

	disbursementHandler := DisbursementHandler{Models: models, AuthManager: authManagerMock}
	paymentsHandler := PaymentsHandler{Models: models, DBConnectionPool: dbConnectionPool, AuthManager: authManagerMock}

	r := chi.NewRouter()
	r.Delete("/disbursements/{id}", disbursementHandler.DeleteDisbursement)
	r.Patch("/payments/{id}/status", paymentsHandler.PatchPaymentStatus)
	r.Patch("/payments/retry", paymentsHandler.RetryPayments)

	doAs := func(userID, method, path, body string) *httptest.ResponseRecorder {
		var reader *strings.Reader
		if body == "" {
			reader = strings.NewReader("")
		} else {
			reader = strings.NewReader(body)
		}
		req := httptest.NewRequest(method, path, reader)
		reqCtx := sdpcontext.SetUserIDInContext(ctx, userID)
		reqCtx = sdpcontext.SetTokenInContext(reqCtx, "test-token")
		req = req.WithContext(reqCtx)
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)
		return rr
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
}
