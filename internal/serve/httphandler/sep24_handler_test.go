package httphandler

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
)

func Test_SEP24InfoHandler_ServeHTTP(t *testing.T) {
	dbConnectionPool := data.SetupDBCP(t)

	models, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)

	ctx := context.Background()

	handler := SEP24InfoHandler{
		Models: models,
	}

	t.Run("returns correct response with multiple assets", func(t *testing.T) {
		data.DeleteAllAssetFixtures(t, ctx, dbConnectionPool)

		data.CreateAssetFixture(t, ctx, dbConnectionPool,
			"USDC",
			"GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5")

		data.CreateAssetFixture(t, ctx, dbConnectionPool, "XLM", "")

		rr := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/sep24/info", nil)
		handler.ServeHTTP(rr, req)

		resp := rr.Result()
		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var response SEP24InfoResponse
		err = json.Unmarshal(respBody, &response)
		require.NoError(t, err)

		assert.Len(t, response.Deposit, 2)
		assert.Contains(t, response.Deposit, "USDC")
		assert.Contains(t, response.Deposit, "native")

		assert.Equal(t, SEP24OperationResponse{
			Enabled:   true,
			MinAmount: 1,
			MaxAmount: 10000,
		}, response.Deposit["USDC"])

		assert.Equal(t, SEP24OperationResponse{
			Enabled:   true,
			MinAmount: 1,
			MaxAmount: 10000,
		}, response.Deposit["native"])

		assert.Empty(t, response.Withdraw)
		assert.False(t, response.Fee.Enabled)
		assert.False(t, response.Features.AccountCreation)
		assert.False(t, response.Features.ClaimableBalances)
	})

	t.Run("returns empty deposit map when no assets exist", func(t *testing.T) {
		data.DeleteAllAssetFixtures(t, ctx, dbConnectionPool)

		rr := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/sep24/info", nil)
		handler.ServeHTTP(rr, req)

		resp := rr.Result()
		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var response SEP24InfoResponse
		err = json.Unmarshal(respBody, &response)
		require.NoError(t, err)

		assert.Empty(t, response.Deposit)
		assert.Empty(t, response.Withdraw)
		assert.False(t, response.Fee.Enabled)
		assert.False(t, response.Features.AccountCreation)
		assert.False(t, response.Features.ClaimableBalances)
	})
}
