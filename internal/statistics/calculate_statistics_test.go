package statistics

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/db"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/db/dbtest"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCalculateStatistics_emptyDatabase(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()

	t.Run("getPaymentsStats", func(t *testing.T) {
		paymentsCounter, paymentsAmountByAsset, errPayments := getPaymentsStats(ctx, dbConnectionPool, "")
		require.NoError(t, errPayments)

		// paymentsCounter assertions
		assert.IsType(t, &PaymentCounters{}, paymentsCounter)
		gotJsonCounter, errJson := json.Marshal(paymentsCounter)
		require.NoError(t, errJson)
		wantJsonCounter := `{
			"canceled":0,
			"draft": 0,
			"ready": 0,
			"pending": 0,
			"paused": 0,
			"success": 0,
			"failed": 0,
			"total": 0
		}`
		assert.JSONEq(t, wantJsonCounter, string(gotJsonCounter))

		// paymentsAmountByAsset assertions
		assert.IsType(t, []PaymentAmountsByAsset{}, paymentsAmountByAsset)
		gotJsonAmountByAsset, errJson := json.Marshal(paymentsAmountByAsset)
		require.NoError(t, errJson)
		wantJsonAmountByAsset := `[]`
		assert.JSONEq(t, wantJsonAmountByAsset, string(gotJsonAmountByAsset))
	})

	t.Run("getReceiverWalletsStats", func(t *testing.T) {
		receiverWalletStats, errReceiver := getReceiverWalletsStats(ctx, dbConnectionPool, "")
		require.NoError(t, errReceiver)

		// receiverWalletStats assertions
		assert.IsType(t, &ReceiverWalletsCounters{}, receiverWalletStats)
		gotJson, errJson := json.Marshal(receiverWalletStats)
		require.NoError(t, errJson)
		wantJson := `{
			"draft": 0,
			"flagged": 0,
			"ready": 0,
			"registered": 0,
			"total": 0
		}`
		assert.JSONEq(t, wantJson, string(gotJson))
	})

	t.Run("getTotalReceivers", func(t *testing.T) {
		totalReceivers, err := getTotalReceivers(ctx, dbConnectionPool, "")
		require.NoError(t, err)
		assert.Equal(t, int64(0), totalReceivers)
	})

	t.Run("getTotalDisbursements", func(t *testing.T) {
		totalDisbursements, err := getTotalDisbursements(ctx, dbConnectionPool)
		require.NoError(t, err)
		assert.Equal(t, int64(0), totalDisbursements)
	})
}

func TestCalculateStatistics(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()

	models, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)

	asset1 := data.CreateAssetFixture(t, ctx, dbConnectionPool, "USDC", "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVV")
	country := data.CreateCountryFixture(t, ctx, dbConnectionPool, "FRA", "France")
	wallet := data.CreateWalletFixture(t, ctx, dbConnectionPool, "wallet1", "https://www.wallet.com", "www.wallet.com", "wallet1://")

	receiver1 := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{})
	receiverWallet1 := data.CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver1.ID, wallet.ID, data.DraftReceiversWalletStatus)

	receiver2 := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{})
	receiverWallet2 := data.CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver2.ID, wallet.ID, data.DraftReceiversWalletStatus)

	disbursement1 := data.CreateDisbursementFixture(t, ctx, dbConnectionPool, models.Disbursements, &data.Disbursement{
		Name:    "disbursement 1",
		Status:  data.CompletedDisbursementStatus,
		Asset:   asset1,
		Wallet:  wallet,
		Country: country,
	})

	stellarTransactionID, err := utils.RandomString(64)
	require.NoError(t, err)
	stellarOperationID, err := utils.RandomString(32)
	require.NoError(t, err)

	data.CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &data.Payment{
		Amount:               "10",
		StellarTransactionID: stellarTransactionID,
		StellarOperationID:   stellarOperationID,
		Status:               data.DraftPaymentStatus,
		Disbursement:         disbursement1,
		Asset:                *asset1,
		ReceiverWallet:       receiverWallet1,
	})

	stellarTransactionID, err = utils.RandomString(64)
	require.NoError(t, err)
	stellarOperationID, err = utils.RandomString(32)
	require.NoError(t, err)

	data.CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &data.Payment{
		Amount:               "10",
		StellarTransactionID: stellarTransactionID,
		StellarOperationID:   stellarOperationID,
		Status:               data.DraftPaymentStatus,
		Disbursement:         disbursement1,
		Asset:                *asset1,
		ReceiverWallet:       receiverWallet2,
	})

	t.Run("get receiver wallet stats", func(t *testing.T) {
		receiverWalletStats, errReceiver := getReceiverWalletsStats(ctx, dbConnectionPool, "")
		require.NoError(t, errReceiver)

		assert.IsType(t, &ReceiverWalletsCounters{}, receiverWalletStats)

		gotJson, errJson := json.Marshal(receiverWalletStats)
		require.NoError(t, errJson)

		wantJson := `{
			"draft": 2,
			"flagged": 0,
			"ready": 0,
			"registered": 0,
			"total": 2
		}`

		assert.JSONEq(t, wantJson, string(gotJson))
	})

	t.Run("get total disbursement", func(t *testing.T) {
		totalDisbursement, errDisbursement := getTotalDisbursements(ctx, dbConnectionPool)
		require.NoError(t, errDisbursement)

		assert.Equal(t, int64(1), totalDisbursement)
	})

	t.Run("get payment stats", func(t *testing.T) {
		paymentsCounter, paymentsAmountByAsset, errPayments := getPaymentsStats(ctx, dbConnectionPool, "")
		require.NoError(t, errPayments)

		assert.IsType(t, &PaymentCounters{}, paymentsCounter)
		assert.IsType(t, []PaymentAmountsByAsset{}, paymentsAmountByAsset)

		gotJsonCounter, errJson := json.Marshal(paymentsCounter)
		require.NoError(t, errJson)

		wantJsonCounter := `{
			"canceled":0,
			"draft": 2,
			"ready": 0,
			"pending": 0,
			"paused": 0,
			"success": 0,
			"failed": 0,
			"total": 2
		}`

		assert.JSONEq(t, wantJsonCounter, string(gotJsonCounter))

		gotJsonAmountByAsset, errJson := json.Marshal(paymentsAmountByAsset)
		require.NoError(t, errJson)

		wantJsonAmountByAsset := `[
				{
					"asset_code": "USDC",
					"payment_amounts": {
							"canceled": "",
							"draft": "20.0000000",
							"ready": "",
							"pending": "",
							"paused": "",
							"success": "",
							"failed": "",
							"average": "10.0000000",
							"total": "20.0000000"
					}
				}
			]`

		assert.JSONEq(t, wantJsonAmountByAsset, string(gotJsonAmountByAsset))
	})

	asset2 := data.CreateAssetFixture(t, ctx, dbConnectionPool, "EURT", "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVV")

	disbursement2 := data.CreateDisbursementFixture(t, ctx, dbConnectionPool, models.Disbursements, &data.Disbursement{
		Name:    "disbursement 2",
		Status:  data.CompletedDisbursementStatus,
		Asset:   asset2,
		Wallet:  wallet,
		Country: country,
	})

	stellarTransactionID, err = utils.RandomString(64)
	require.NoError(t, err)
	stellarOperationID, err = utils.RandomString(32)
	require.NoError(t, err)

	data.CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &data.Payment{
		Amount:               "10",
		StellarTransactionID: stellarTransactionID,
		StellarOperationID:   stellarOperationID,
		Status:               data.SuccessPaymentStatus,
		Disbursement:         disbursement2,
		Asset:                *asset2,
		ReceiverWallet:       receiverWallet1,
	})

	t.Run("get payment stats with multiple assets codes", func(t *testing.T) {
		paymentsCounter, paymentsAmountByAsset, err := getPaymentsStats(ctx, dbConnectionPool, "")
		require.NoError(t, err)

		assert.IsType(t, &PaymentCounters{}, paymentsCounter)
		assert.IsType(t, []PaymentAmountsByAsset{}, paymentsAmountByAsset)

		gotJsonCounter, err := json.Marshal(paymentsCounter)
		require.NoError(t, err)

		wantJsonCounter := `{
			"canceled": 0,
			"draft": 2,
			"ready": 0,
			"pending": 0,
			"paused": 0,
			"success": 1,
			"failed": 0,
			"total": 3
		}`

		assert.JSONEq(t, wantJsonCounter, string(gotJsonCounter))

		gotJsonAmountByAsset, err := json.Marshal(paymentsAmountByAsset)
		require.NoError(t, err)

		wantJsonAmountByAsset := `[
				{
					"asset_code": "EURT",
					"payment_amounts": {
						  "canceled": "",
							"draft": "",
							"ready": "",
							"pending": "",
							"paused": "",
							"success": "10.0000000",
							"failed": "",
							"average": "10.0000000",
							"total": "10.0000000"
					}
				},
				{
					"asset_code": "USDC",
					"payment_amounts": {
							"canceled":"",
							"draft": "20.0000000",
							"ready": "",
							"pending": "",
							"paused": "",
							"success": "",
							"failed": "",
							"average": "10.0000000",
							"total": "20.0000000"
					}
				}
			]`

		assert.JSONEq(t, wantJsonAmountByAsset, string(gotJsonAmountByAsset))
	})

	t.Run("get payment stats for specific disbursement", func(t *testing.T) {
		paymentsCounter, paymentsAmountByAsset, err := getPaymentsStats(ctx, dbConnectionPool, disbursement2.ID)
		require.NoError(t, err)

		assert.IsType(t, &PaymentCounters{}, paymentsCounter)
		assert.IsType(t, []PaymentAmountsByAsset{}, paymentsAmountByAsset)

		gotJsonCounter, err := json.Marshal(paymentsCounter)
		require.NoError(t, err)

		wantJsonCounter := `{
			"canceled":0,
			"draft": 0,
			"ready": 0,
			"pending": 0,
			"paused": 0,
			"success": 1,
			"failed": 0,
			"total": 1
		}`

		assert.JSONEq(t, wantJsonCounter, string(gotJsonCounter))

		gotJsonAmountByAsset, err := json.Marshal(paymentsAmountByAsset)
		require.NoError(t, err)

		wantJsonAmountByAsset := `[
				{
					"asset_code": "EURT",
					"payment_amounts": {
							"canceled":"",
							"draft": "",
							"ready": "",
							"pending": "",
							"paused": "",
							"success": "10.0000000",
							"failed": "",
							"average": "10.0000000",
							"total": "10.0000000"
					}
				}
			]`

		assert.JSONEq(t, wantJsonAmountByAsset, string(gotJsonAmountByAsset))
	})

	t.Run("get receiver wallet stats for specific disbursement", func(t *testing.T) {
		receiverWalletStats, err := getReceiverWalletsStats(ctx, dbConnectionPool, disbursement2.ID)
		require.NoError(t, err)

		assert.IsType(t, &ReceiverWalletsCounters{}, receiverWalletStats)

		gotJson, err := json.Marshal(receiverWalletStats)
		require.NoError(t, err)

		wantJson := `{
			"draft": 1,
			"flagged": 0,
			"ready": 0,
			"registered": 0,
			"total": 1
		}`

		assert.JSONEq(t, wantJson, string(gotJson))
	})

	t.Run("get total receivers", func(t *testing.T) {
		totalReceivers, err := getTotalReceivers(ctx, dbConnectionPool, "")
		require.NoError(t, err)
		assert.Equal(t, int64(2), totalReceivers)
	})

	t.Run("get total receivers with disbursement ID", func(t *testing.T) {
		totalReceivers, err := getTotalReceivers(ctx, dbConnectionPool, disbursement2.ID)
		require.NoError(t, err)
		assert.Equal(t, int64(1), totalReceivers)
	})
}

func Test_checkIfDisbursementExists(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()
	model, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)

	t.Run("disbursement does not exist", func(t *testing.T) {
		exists, err := checkIfDisbursementExists(context.Background(), dbConnectionPool, "non-existing-id")
		require.NoError(t, err)
		assert.False(t, exists)
	})

	t.Run("disbursement exists", func(t *testing.T) {
		asset := data.CreateAssetFixture(t, ctx, dbConnectionPool, "USDC", "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVV")
		country := data.CreateCountryFixture(t, ctx, dbConnectionPool, "FRA", "France")
		wallet := data.CreateWalletFixture(t, ctx, dbConnectionPool, "wallet1", "https://www.wallet.com", "www.wallet.com", "wallet1://")

		disbursement := data.CreateDisbursementFixture(t, ctx, dbConnectionPool, model.Disbursements, &data.Disbursement{
			Status: data.DraftDisbursementStatus,
			StatusHistory: []data.DisbursementStatusHistoryEntry{
				{
					Status: data.DraftDisbursementStatus,
					UserID: "user1",
				},
			},
			Asset:   asset,
			Country: country,
			Wallet:  wallet,
		})
		exists, err := checkIfDisbursementExists(context.Background(), dbConnectionPool, disbursement.ID)
		require.NoError(t, err)
		assert.True(t, exists)
	})
}
