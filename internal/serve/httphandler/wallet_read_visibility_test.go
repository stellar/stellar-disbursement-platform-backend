package httphandler

import (
	"context"
	"fmt"
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
	"github.com/stellar/stellar-disbursement-platform-backend/internal/services"
	sigMocks "github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine/signing/mocks"
	"github.com/stellar/stellar-disbursement-platform-backend/pkg/schema"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-auth/pkg/auth"
)

// Test_WalletReadVisibility proves the read-endpoint taxonomy for the membership-filtered
// bucket: listings filter to the caller's wallets (post-filter totals), individual reads
// outside the scope return 404 (existence never disclosed), and Owners see everything.
func Test_WalletReadVisibility(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()
	models, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)

	// Wallets A (default) + B; member of A only; owner.
	walletA := data.EnsureDefaultDistributionWalletFixture(t, ctx, dbConnectionPool)
	var walletBID string
	require.NoError(t, dbConnectionPool.GetContext(ctx, &walletBID, `
		INSERT INTO distribution_wallets (name, distribution_account_type)
		VALUES ('visibility-wallet-b', 'DISTRIBUTION_ACCOUNT.STELLAR.DB_VAULT') RETURNING id`))

	memberA := &auth.User{ID: "member-a", Email: "a@example.com", Roles: []string{string(data.BusinessUserRole)}}
	owner := &auth.User{ID: "owner-user", Email: "o@example.com", IsOwner: true}
	_, err = models.WalletMemberships.Insert(ctx, dbConnectionPool, memberA.ID, walletA.ID, data.BusinessUserRole, nil)
	require.NoError(t, err)

	authManagerMock := &auth.AuthManagerMock{}
	authManagerMock.On("GetUserByID", mock.Anything, memberA.ID).Return(memberA, nil)
	authManagerMock.On("GetUserByID", mock.Anything, owner.ID).Return(owner, nil)
	// Listing decoration (status-history user names) — not under test here.
	authManagerMock.On("GetUsersByID", mock.Anything, mock.Anything, mock.Anything).Return([]*auth.User{}, nil).Maybe()

	// Disbursements from each wallet + a payment under each.
	disbursementA := data.CreateDisbursementFixture(t, ctx, dbConnectionPool, models.Disbursements, &data.Disbursement{
		Name: "visible-from-a", SourceWalletID: walletA.ID,
	})
	disbursementB := data.CreateDisbursementFixture(t, ctx, dbConnectionPool, models.Disbursements, &data.Disbursement{
		Name: "hidden-from-a", SourceWalletID: walletBID,
	})

	receiver := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{})
	receiverWallet := data.CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver.ID, disbursementA.Wallet.ID, data.ReadyReceiversWalletStatus)
	paymentA := data.CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &data.Payment{
		ReceiverWallet: receiverWallet, Disbursement: disbursementA, Asset: *disbursementA.Asset, Amount: "10", Status: data.DraftPaymentStatus,
	})
	paymentB := data.CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &data.Payment{
		ReceiverWallet: receiverWallet, Disbursement: disbursementB, Asset: *disbursementB.Asset, Amount: "10", Status: data.DraftPaymentStatus,
	})
	// Direct payment (no disbursement) routed explicitly to wallet B — the case the
	// COALESCE-to-default-wallet regression missed entirely, since it has no Disbursement
	// to fall through to.
	paymentC := data.CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &data.Payment{
		ReceiverWallet: receiverWallet, Type: data.PaymentTypeDirect, SourceWalletID: walletBID,
		Asset: *disbursementB.Asset, Amount: "10", Status: data.DraftPaymentStatus,
	})

	disbursementHandler := DisbursementHandler{
		Models:      models,
		AuthManager: authManagerMock,
		DisbursementManagementService: &services.DisbursementManagementService{
			Models: models, AuthManager: authManagerMock,
		},
	}
	mDistAccResolver := sigMocks.NewMockDistributionAccountResolver(t)
	mDistAccResolver.
		On("DistributionAccountFromContext", mock.Anything).
		Return(schema.NewDefaultStellarTransactionAccount("GDIVVKL6QYF6C6K3C5PZZBQ2NQDLN2OSLMVIEQRHS6DZE7WRL33ZDNXL"), nil).
		Maybe()
	paymentsHandler := PaymentsHandler{Models: models, DBConnectionPool: dbConnectionPool, AuthManager: authManagerMock, DistributionAccountResolver: mDistAccResolver}

	r := chi.NewRouter()
	r.Get("/disbursements", disbursementHandler.GetDisbursements)
	r.Get("/disbursements/{id}", disbursementHandler.GetDisbursement)
	r.Get("/disbursements/{id}/instructions", disbursementHandler.GetDisbursementInstructions)
	r.Get("/payments", paymentsHandler.GetPayments)
	r.Get("/payments/{id}", paymentsHandler.GetPayment)

	doAs := func(userID, path string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req = req.WithContext(sdpcontext.SetUserIDInContext(req.Context(), userID))
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)
		return rr
	}

	t.Run("listings filter to the caller's wallets with post-filter totals", func(t *testing.T) {
		rr := doAs(memberA.ID, "/disbursements")
		require.Equal(t, http.StatusOK, rr.Code)
		body := rr.Body.String()
		assert.Contains(t, body, disbursementA.ID)
		assert.NotContains(t, body, disbursementB.ID)
		assert.Contains(t, body, `"total": 1`, "pagination totals must be post-filter")

		rr = doAs(owner.ID, "/disbursements")
		require.Equal(t, http.StatusOK, rr.Code)
		body = rr.Body.String()
		assert.Contains(t, body, disbursementA.ID)
		assert.Contains(t, body, disbursementB.ID)
	})

	t.Run("individual reads outside the scope return 404, never 403", func(t *testing.T) {
		rr := doAs(memberA.ID, fmt.Sprintf("/disbursements/%s", disbursementB.ID))
		assert.Equal(t, http.StatusNotFound, rr.Code)
		assert.NotContains(t, rr.Body.String(), walletBID, "no wallet details may leak")

		rr = doAs(memberA.ID, fmt.Sprintf("/disbursements/%s/instructions", disbursementB.ID))
		assert.Equal(t, http.StatusNotFound, rr.Code)

		rr = doAs(memberA.ID, fmt.Sprintf("/disbursements/%s", disbursementA.ID))
		assert.Equal(t, http.StatusOK, rr.Code)

		rr = doAs(owner.ID, fmt.Sprintf("/disbursements/%s", disbursementB.ID))
		assert.Equal(t, http.StatusOK, rr.Code)
	})

	t.Run("payments follow their disbursement's wallet", func(t *testing.T) {
		rr := doAs(memberA.ID, "/payments")
		require.Equal(t, http.StatusOK, rr.Code)
		body := rr.Body.String()
		assert.Contains(t, body, paymentA.ID)
		assert.NotContains(t, body, paymentB.ID)

		rr = doAs(memberA.ID, fmt.Sprintf("/payments/%s", paymentB.ID))
		assert.Equal(t, http.StatusNotFound, rr.Code)

		rr = doAs(memberA.ID, fmt.Sprintf("/payments/%s", paymentA.ID))
		assert.Equal(t, http.StatusOK, rr.Code)

		rr = doAs(owner.ID, "/payments")
		require.Equal(t, http.StatusOK, rr.Code)
		assert.Contains(t, rr.Body.String(), paymentB.ID)
	})

	t.Run("direct payments follow their own explicit source wallet, not the default", func(t *testing.T) {
		rr := doAs(memberA.ID, "/payments")
		require.Equal(t, http.StatusOK, rr.Code)
		assert.NotContains(t, rr.Body.String(), paymentC.ID, "member A has no membership on wallet B")

		rr = doAs(memberA.ID, fmt.Sprintf("/payments/%s", paymentC.ID))
		assert.Equal(t, http.StatusNotFound, rr.Code)

		rr = doAs(owner.ID, fmt.Sprintf("/payments/%s", paymentC.ID))
		assert.Equal(t, http.StatusOK, rr.Code)
	})

	t.Run("member with zero memberships sees nothing", func(t *testing.T) {
		nobody := &auth.User{ID: "member-none", Email: "n@example.com", Roles: []string{string(data.BusinessUserRole)}}
		authManagerMock.On("GetUserByID", mock.Anything, nobody.ID).Return(nobody, nil)

		rr := doAs(nobody.ID, "/disbursements")
		require.Equal(t, http.StatusOK, rr.Code)
		assert.NotContains(t, rr.Body.String(), disbursementA.ID)
		assert.NotContains(t, rr.Body.String(), disbursementB.ID)
	})
}
