package httphandler

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/db"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/db/dbtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_RetryInvitation(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	models, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)

	handler := &ReceiverWalletsHandler{
		Models:           models,
		DBConnectionPool: dbConnectionPool,
	}

	ctx := context.Background()

	r := chi.NewRouter()
	r.Patch("/receivers/wallets/{receiver_wallet_id}", handler.RetryInvitation)

	t.Run("returns error when receiver wallet does not exist", func(t *testing.T) {
		route := "/receivers/wallets/invalid_id"
		req, err := http.NewRequest("PATCH", route, nil)
		require.NoError(t, err)

		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		resp := rr.Result()
		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	})

	t.Run("successfuly retry invitation", func(t *testing.T) {
		receiver := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{})
		wallet := data.CreateWalletFixture(t, ctx, dbConnectionPool, "wallet", "https://www.wallet.com", "www.wallet.com", "wallet1://")
		rw := data.CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver.ID, wallet.ID, data.ReadyReceiversWalletStatus)

		route := fmt.Sprintf("/receivers/wallets/%s", rw.ID)
		req, err := http.NewRequest("PATCH", route, nil)
		require.NoError(t, err)

		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		resp := rr.Result()
		assert.Equal(t, http.StatusOK, resp.StatusCode)
	})
}
