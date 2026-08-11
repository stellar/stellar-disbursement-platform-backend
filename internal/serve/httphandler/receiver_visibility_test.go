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
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-auth/pkg/auth"
)

// Test_ReceiverVisibility_R1 proves the accepted flag R1: receivers are membership-filtered —
// visible to callers whose wallets have paid them; hidden (404, no disclosure) otherwise;
// Owners see all.
func Test_ReceiverVisibility_R1(t *testing.T) {
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
		VALUES ('recv-wallet-b', 'DISTRIBUTION_ACCOUNT.STELLAR.DB_VAULT') RETURNING id`))

	memberA := &auth.User{ID: "recv-member-a", Email: "a@recv.test", Roles: []string{string(data.BusinessUserRole)}}
	owner := &auth.User{ID: "recv-owner", Email: "o@recv.test", IsOwner: true}
	_, err = models.WalletMemberships.Insert(ctx, dbConnectionPool, memberA.ID, walletA.ID, data.BusinessUserRole, nil)
	require.NoError(t, err)

	authManagerMock := &auth.AuthManagerMock{}
	authManagerMock.On("GetUserByID", mock.Anything, memberA.ID).Return(memberA, nil)
	authManagerMock.On("GetUserByID", mock.Anything, owner.ID).Return(owner, nil)

	mkReceiverPaidFrom := func(name, walletID string) *data.Receiver {
		d := data.CreateDisbursementFixture(t, ctx, dbConnectionPool, models.Disbursements, &data.Disbursement{
			Name: name, SourceWalletID: walletID, Status: data.StartedDisbursementStatus,
		})
		receiver := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{})
		rw := data.CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver.ID, d.Wallet.ID, data.ReadyReceiversWalletStatus)
		data.CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &data.Payment{
			ReceiverWallet: rw, Disbursement: d, Asset: *d.Asset, Amount: "3", Status: data.DraftPaymentStatus,
		})
		return receiver
	}
	receiverFromA := mkReceiverPaidFrom("recv-disb-a", walletA.ID)
	receiverFromB := mkReceiverPaidFrom("recv-disb-b", walletBID)

	handler := ReceiverHandler{Models: models, DBConnectionPool: dbConnectionPool, AuthManager: authManagerMock}
	r := chi.NewRouter()
	r.Get("/receivers", handler.GetReceivers)
	r.Get("/receivers/{id}", handler.GetReceiver)

	doAs := func(userID, path string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req = req.WithContext(sdpcontext.SetUserIDInContext(ctx, userID))
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)
		return rr
	}

	t.Run("listing filters to receivers paid from the caller's wallets", func(t *testing.T) {
		rr := doAs(memberA.ID, "/receivers")
		require.Equal(t, http.StatusOK, rr.Code)
		assert.Contains(t, rr.Body.String(), receiverFromA.ID)
		assert.NotContains(t, rr.Body.String(), receiverFromB.ID)

		rr = doAs(owner.ID, "/receivers")
		require.Equal(t, http.StatusOK, rr.Code)
		assert.Contains(t, rr.Body.String(), receiverFromA.ID)
		assert.Contains(t, rr.Body.String(), receiverFromB.ID)
	})

	t.Run("individual receiver outside scope is 404 — no disclosure", func(t *testing.T) {
		rr := doAs(memberA.ID, fmt.Sprintf("/receivers/%s", receiverFromB.ID))
		assert.Equal(t, http.StatusNotFound, rr.Code)

		rr = doAs(memberA.ID, fmt.Sprintf("/receivers/%s", receiverFromA.ID))
		assert.Equal(t, http.StatusOK, rr.Code)

		rr = doAs(owner.ID, fmt.Sprintf("/receivers/%s", receiverFromB.ID))
		assert.Equal(t, http.StatusOK, rr.Code)
	})
}
