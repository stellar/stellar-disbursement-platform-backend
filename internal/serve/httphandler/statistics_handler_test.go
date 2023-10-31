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
	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStatisticsHandler(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	// setup
	statisticsHandler := StatisticsHandler{DBConnectionPool: dbConnectionPool}
	r := chi.NewRouter()
	r.Get("/statistics", statisticsHandler.GetStatistics)
	r.Get("/statistics/{id}", statisticsHandler.GetStatisticsByDisbursement)

	t.Run("get statistics with no data", func(t *testing.T) {
		// test
		var req *http.Request
		req, err = http.NewRequest("GET", "/statistics", nil)
		require.NoError(t, err)
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		// assert response
		assert.Equal(t, http.StatusOK, rr.Code)

		wantJson := `{
			"payment_counters": {
				"canceled": 0,
				"draft": 0,
				"ready": 0,
				"pending": 0,
				"paused": 0,
				"success": 0,
				"failed": 0,
				"total": 0
			},
			"payment_amounts_by_asset": [],
			"receiver_wallets_counters": {
				"draft": 0,
				"ready": 0,
				"registered": 0,
				"flagged": 0,
				"total": 0
			},
			"total_receivers": 0,
			"total_disbursements": 0
		}`
		assert.JSONEq(t, wantJson, rr.Body.String())
	})

	t.Run("get statistics for invalid disbursement id", func(t *testing.T) {
		// test
		var req *http.Request
		req, err = http.NewRequest("GET", "/statistics/invalid-id", nil)
		require.NoError(t, err)
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		// assert response
		assert.Equal(t, http.StatusNotFound, rr.Code)

		wantJson := `{
			"error": "a disbursement with the id invalid-id does not exist"
		}`
		assert.JSONEq(t, wantJson, rr.Body.String())
	})

	ctx := context.Background()
	models, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)

	asset1 := data.CreateAssetFixture(t, ctx, dbConnectionPool, "USDC", "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVV")
	country := data.CreateCountryFixture(t, ctx, dbConnectionPool, "FRA", "France")
	wallet := data.CreateWalletFixture(t, ctx, dbConnectionPool, "wallet1", "https://www.wallet.com", "www.wallet.com", "wallet1://")

	disbursement := data.CreateDisbursementFixture(t, ctx, dbConnectionPool, models.Disbursements, &data.Disbursement{
		Name:    "disbursement 1",
		Status:  data.CompletedDisbursementStatus,
		Asset:   asset1,
		Wallet:  wallet,
		Country: country,
	})

	t.Run("get statistics for existing disbursement with no data", func(t *testing.T) {
		// test
		var req *http.Request
		req, err = http.NewRequest("GET", "/statistics/"+disbursement.ID, nil)
		require.NoError(t, err)
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		// assert response
		assert.Equal(t, http.StatusOK, rr.Code)

		wantJson := `{
			"payment_counters": {
				"canceled": 0,
				"draft": 0,
				"ready": 0,
				"pending": 0,
				"paused": 0,
				"success": 0,
				"failed": 0,
				"total": 0
			},
			"payment_amounts_by_asset": [],
			"receiver_wallets_counters": {
				"draft": 0,
				"ready": 0,
				"registered": 0,
				"flagged": 0,
				"total": 0
			},
			"total_receivers": 0
		}`
		assert.JSONEq(t, wantJson, rr.Body.String())
	})

	receiver := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{})
	receiverWallet := data.CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver.ID, wallet.ID, data.DraftReceiversWalletStatus)

	stellarTransactionID, err := utils.RandomString(64)
	require.NoError(t, err)
	stellarOperationID, err := utils.RandomString(32)
	require.NoError(t, err)

	data.CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &data.Payment{
		Amount:               "10",
		StellarTransactionID: stellarTransactionID,
		StellarOperationID:   stellarOperationID,
		Status:               data.DraftPaymentStatus,
		Disbursement:         disbursement,
		Asset:                *asset1,
		ReceiverWallet:       receiverWallet,
	})

	t.Run("get statistics", func(t *testing.T) {
		// test
		req, err := http.NewRequest("GET", "/statistics", nil)
		require.NoError(t, err)
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		// assert response
		assert.Equal(t, http.StatusOK, rr.Code)
		wantJson := `{
			"payment_counters": {
				"canceled": 0,
				"draft": 1,
				"ready": 0,
				"pending": 0,
				"paused": 0,
				"success": 0,
				"failed": 0,
				"total": 1
			},
			"payment_amounts_by_asset": [
				{
					"asset_code": "USDC",
					"payment_amounts": {
							"canceled": "",
						  "draft": "10.0000000",
						  "ready": "",
						  "pending": "",
						  "paused": "",
						  "success": "",
						  "failed": "",
						  "average": "10.0000000",
						  "total": "10.0000000"
					}
				}
			],
			"receiver_wallets_counters": {
				"draft": 1,
				"ready": 0,
				"registered": 0,
				"flagged": 0,
				"total": 1
			},
			"total_receivers": 1,
			"total_disbursements": 1
		}`
		assert.JSONEq(t, wantJson, rr.Body.String())
	})

	t.Run("get statistics for specific disbursement", func(t *testing.T) {
		route := fmt.Sprintf("/statistics/%s", disbursement.ID)
		req, err := http.NewRequest("GET", route, nil)
		require.NoError(t, err)
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		// assert response
		assert.Equal(t, http.StatusOK, rr.Code)

		wantJson := `{
			"payment_counters": {
				"canceled": 0,
				"draft": 1,
				"ready": 0,
				"pending": 0,
				"paused": 0,
				"success": 0,
				"failed": 0,
				"total": 1
			},
			"payment_amounts_by_asset": [
				{
					"asset_code": "USDC",
					"payment_amounts": {
							"canceled": "",
						  "draft": "10.0000000",
						  "ready": "",
						  "pending": "",
						  "paused": "",
						  "success": "",
						  "failed": "",
						  "average": "10.0000000",
						  "total": "10.0000000"
					}
				}
			],
			"receiver_wallets_counters": {
				"draft": 1,
				"ready": 0,
				"registered": 0,
				"flagged": 0,
				"total": 1
			},
			"total_receivers": 1
		}`
		assert.JSONEq(t, wantJson, rr.Body.String())
	})
}
