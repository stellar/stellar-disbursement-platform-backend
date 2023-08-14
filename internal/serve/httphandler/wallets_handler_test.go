package httphandler

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/db"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/db/dbtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_WalletsHandlerGetWallets(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	models, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)

	ctx := context.Background()

	handler := &WalletsHandler{
		Models: models,
	}

	t.Run("successfully returns a list of countries", func(t *testing.T) {
		expected := data.ClearAndCreateWalletFixtures(t, ctx, dbConnectionPool)
		expectedJSON, err := json.Marshal(expected)
		require.NoError(t, err)

		rr := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/wallets", nil)
		http.HandlerFunc(handler.GetWallets).ServeHTTP(rr, req)

		resp := rr.Result()

		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		assert.JSONEq(t, string(expectedJSON), string(respBody))
	})
}
