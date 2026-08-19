package statistics

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/db/dbtest"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
)

func TestCalculateStatistics_emptyDatabase(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()

	t.Run("getPaymentsStats", func(t *testing.T) {
		paymentsCounter, paymentsAmountByAsset, errPayments := getPaymentsStats(ctx, dbConnectionPool, "", nil)
		require.NoError(t, errPayments)

		// paymentsCounter assertions
		assert.IsType(t, &PaymentCounters{}, paymentsCounter)
		gotJSONCounter, errJSON := json.Marshal(paymentsCounter)
		require.NoError(t, errJSON)
		wantJSONCounter := `{
			"canceled":0,
			"draft": 0,
			"ready": 0,
			"pending": 0,
			"paused": 0,
			"success": 0,
			"failed": 0,
			"total": 0
		}`
		assert.JSONEq(t, wantJSONCounter, string(gotJSONCounter))

		// paymentsAmountByAsset assertions
		assert.IsType(t, []PaymentAmountsByAsset{}, paymentsAmountByAsset)
		gotJSONAmountByAsset, errJSON := json.Marshal(paymentsAmountByAsset)
		require.NoError(t, errJSON)
		wantJSONAmountByAsset := `[]`
		assert.JSONEq(t, wantJSONAmountByAsset, string(gotJSONAmountByAsset))
	})

	t.Run("getReceiverWalletsStats", func(t *testing.T) {
		receiverWalletStats, errReceiver := getReceiverWalletsStats(ctx, dbConnectionPool, "", nil)
		require.NoError(t, errReceiver)

		// receiverWalletStats assertions
		assert.IsType(t, &ReceiverWalletsCounters{}, receiverWalletStats)
		gotJSON, errJSON := json.Marshal(receiverWalletStats)
		require.NoError(t, errJSON)
		wantJSON := `{
			"draft": 0,
			"flagged": 0,
			"ready": 0,
			"registered": 0,
			"total": 0
		}`
		assert.JSONEq(t, wantJSON, string(gotJSON))
	})

	t.Run("getTotalReceivers", func(t *testing.T) {
		totalReceivers, err := getTotalReceivers(ctx, dbConnectionPool, "", nil)
		require.NoError(t, err)
		assert.Equal(t, int64(0), totalReceivers)
	})

	t.Run("getTotalDisbursements", func(t *testing.T) {
		totalDisbursements, err := getTotalDisbursements(ctx, dbConnectionPool, nil)
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
	wallet := data.CreateWalletFixture(t, ctx, dbConnectionPool, "wallet1", "https://www.wallet.com", "www.wallet.com", "wallet1://")

	receiver1 := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{})
	receiverWallet1 := data.CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver1.ID, wallet.ID, data.DraftReceiversWalletStatus)

	receiver2 := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{})
	receiverWallet2 := data.CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver2.ID, wallet.ID, data.DraftReceiversWalletStatus)

	disbursement1 := data.CreateDisbursementFixture(t, ctx, dbConnectionPool, models.Disbursements, &data.Disbursement{
		Name:   "disbursement 1",
		Status: data.CompletedDisbursementStatus,
		Asset:  asset1,
		Wallet: wallet,
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
		receiverWalletStats, errReceiver := getReceiverWalletsStats(ctx, dbConnectionPool, "", nil)
		require.NoError(t, errReceiver)

		assert.IsType(t, &ReceiverWalletsCounters{}, receiverWalletStats)

		gotJSON, errJSON := json.Marshal(receiverWalletStats)
		require.NoError(t, errJSON)

		wantJSON := `{
			"draft": 2,
			"flagged": 0,
			"ready": 0,
			"registered": 0,
			"total": 2
		}`

		assert.JSONEq(t, wantJSON, string(gotJSON))
	})

	t.Run("get total disbursement", func(t *testing.T) {
		totalDisbursement, errDisbursement := getTotalDisbursements(ctx, dbConnectionPool, nil)
		require.NoError(t, errDisbursement)

		assert.Equal(t, int64(1), totalDisbursement)
	})

	t.Run("get payment stats", func(t *testing.T) {
		paymentsCounter, paymentsAmountByAsset, errPayments := getPaymentsStats(ctx, dbConnectionPool, "", nil)
		require.NoError(t, errPayments)

		assert.IsType(t, &PaymentCounters{}, paymentsCounter)
		assert.IsType(t, []PaymentAmountsByAsset{}, paymentsAmountByAsset)

		gotJSONCounter, errJSON := json.Marshal(paymentsCounter)
		require.NoError(t, errJSON)

		wantJSONCounter := `{
			"canceled":0,
			"draft": 2,
			"ready": 0,
			"pending": 0,
			"paused": 0,
			"success": 0,
			"failed": 0,
			"total": 2
		}`

		assert.JSONEq(t, wantJSONCounter, string(gotJSONCounter))

		gotJSONAmountByAsset, errJSON := json.Marshal(paymentsAmountByAsset)
		require.NoError(t, errJSON)

		wantJSONAmountByAsset := `[
				{
					"asset_code": "USDC",
					"asset_issuer": "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVV",
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

		assert.JSONEq(t, wantJSONAmountByAsset, string(gotJSONAmountByAsset))
	})

	asset2 := data.CreateAssetFixture(t, ctx, dbConnectionPool, "EURT", "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVV")

	disbursement2 := data.CreateDisbursementFixture(t, ctx, dbConnectionPool, models.Disbursements, &data.Disbursement{
		Name:   "disbursement 2",
		Status: data.CompletedDisbursementStatus,
		Asset:  asset2,
		Wallet: wallet,
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
		paymentsCounter, paymentsAmountByAsset, err := getPaymentsStats(ctx, dbConnectionPool, "", nil)
		require.NoError(t, err)

		assert.IsType(t, &PaymentCounters{}, paymentsCounter)
		assert.IsType(t, []PaymentAmountsByAsset{}, paymentsAmountByAsset)

		gotJSONCounter, err := json.Marshal(paymentsCounter)
		require.NoError(t, err)

		wantJSONCounter := `{
			"canceled": 0,
			"draft": 2,
			"ready": 0,
			"pending": 0,
			"paused": 0,
			"success": 1,
			"failed": 0,
			"total": 3
		}`

		assert.JSONEq(t, wantJSONCounter, string(gotJSONCounter))

		gotJSONAmountByAsset, err := json.Marshal(paymentsAmountByAsset)
		require.NoError(t, err)

		wantJSONAmountByAsset := `[
				{
					"asset_code": "EURT",
					"asset_issuer": "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVV",
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
					"asset_issuer": "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVV",
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

		assert.JSONEq(t, wantJSONAmountByAsset, string(gotJSONAmountByAsset))
	})

	t.Run("get payment stats for specific disbursement", func(t *testing.T) {
		paymentsCounter, paymentsAmountByAsset, err := getPaymentsStats(ctx, dbConnectionPool, disbursement2.ID, nil)
		require.NoError(t, err)

		assert.IsType(t, &PaymentCounters{}, paymentsCounter)
		assert.IsType(t, []PaymentAmountsByAsset{}, paymentsAmountByAsset)

		gotJSONCounter, err := json.Marshal(paymentsCounter)
		require.NoError(t, err)

		wantJSONCounter := `{
			"canceled":0,
			"draft": 0,
			"ready": 0,
			"pending": 0,
			"paused": 0,
			"success": 1,
			"failed": 0,
			"total": 1
		}`

		assert.JSONEq(t, wantJSONCounter, string(gotJSONCounter))

		gotJSONAmountByAsset, err := json.Marshal(paymentsAmountByAsset)
		require.NoError(t, err)

		wantJSONAmountByAsset := `[
				{
					"asset_code": "EURT",
					"asset_issuer": "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVV",
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

		assert.JSONEq(t, wantJSONAmountByAsset, string(gotJSONAmountByAsset))
	})

	t.Run("get receiver wallet stats for specific disbursement", func(t *testing.T) {
		receiverWalletStats, err := getReceiverWalletsStats(ctx, dbConnectionPool, disbursement2.ID, nil)
		require.NoError(t, err)

		assert.IsType(t, &ReceiverWalletsCounters{}, receiverWalletStats)

		gotJSON, err := json.Marshal(receiverWalletStats)
		require.NoError(t, err)

		wantJSON := `{
			"draft": 1,
			"flagged": 0,
			"ready": 0,
			"registered": 0,
			"total": 1
		}`

		assert.JSONEq(t, wantJSON, string(gotJSON))
	})

	t.Run("get total receivers", func(t *testing.T) {
		totalReceivers, err := getTotalReceivers(ctx, dbConnectionPool, "", nil)
		require.NoError(t, err)
		assert.Equal(t, int64(2), totalReceivers)
	})

	t.Run("get total receivers with disbursement ID", func(t *testing.T) {
		totalReceivers, err := getTotalReceivers(ctx, dbConnectionPool, disbursement2.ID, nil)
		require.NoError(t, err)
		assert.Equal(t, int64(1), totalReceivers)
	})
}

// An issuer migration puts two issuers of the same asset code in play at once. They have to stay
// on separate rows: once merged nothing downstream can split them again, and the dashboard joins
// these figures against balances keyed by CODE:ISSUER.
func Test_getPaymentsStats_separatesAssetsByIssuer(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()

	models, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)

	const (
		issuerOld = "GA62MH5RDXFWAIWHQEFNMO2SVDDCQLWOO3GO36VQB5LHUXL22DQ6IQAU"
		issuerNew = "GABC65XJDMXTGPNZRCI6V3KOKKWVK55UEKGQLONRIVYPMEJNNQ45YOEE"
	)

	usdcOld := data.CreateAssetFixture(t, ctx, dbConnectionPool, "USDC", issuerOld)
	usdcNew := data.CreateAssetFixture(t, ctx, dbConnectionPool, "USDC", issuerNew)
	xlm := data.CreateAssetFixture(t, ctx, dbConnectionPool, "XLM", "")

	wallet := data.CreateWalletFixture(t, ctx, dbConnectionPool, "wallet1", "https://www.wallet.com", "www.wallet.com", "wallet1://")
	receiver := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{})
	receiverWallet := data.CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver.ID, wallet.ID, data.DraftReceiversWalletStatus)

	createPayment := func(asset *data.Asset, amount string, status data.PaymentStatus) {
		t.Helper()

		disbursement := data.CreateDisbursementFixture(t, ctx, dbConnectionPool, models.Disbursements, &data.Disbursement{
			Status: data.CompletedDisbursementStatus,
			Asset:  asset,
			Wallet: wallet,
		})

		stellarTransactionID, txErr := utils.RandomString(64)
		require.NoError(t, txErr)
		stellarOperationID, opErr := utils.RandomString(32)
		require.NoError(t, opErr)

		data.CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &data.Payment{
			Amount:               amount,
			StellarTransactionID: stellarTransactionID,
			StellarOperationID:   stellarOperationID,
			Status:               status,
			Disbursement:         disbursement,
			Asset:                *asset,
			ReceiverWallet:       receiverWallet,
		})
	}

	createPayment(usdcOld, "10", data.DraftPaymentStatus)
	createPayment(usdcNew, "25", data.SuccessPaymentStatus)
	createPayment(xlm, "7", data.SuccessPaymentStatus)

	paymentsCounter, paymentsAmountByAsset, err := getPaymentsStats(ctx, dbConnectionPool, "", nil)
	require.NoError(t, err)

	gotJSONCounter, err := json.Marshal(paymentsCounter)
	require.NoError(t, err)

	wantJSONCounter := `{
		"canceled": 0,
		"draft": 1,
		"ready": 0,
		"pending": 0,
		"paused": 0,
		"success": 2,
		"failed": 0,
		"total": 3
	}`

	assert.JSONEq(t, wantJSONCounter, string(gotJSONCounter))

	// Ordering between distinct assets depends on the database collation, so assert set
	// membership rather than a fixed sequence.
	assert.ElementsMatch(t, []PaymentAmountsByAsset{
		{
			AssetCode:   "USDC",
			AssetIssuer: issuerOld,
			PaymentAmounts: PaymentAmounts{
				Draft:   "10.0000000",
				Average: "10.0000000",
				Total:   "10.0000000",
			},
		},
		{
			AssetCode:   "USDC",
			AssetIssuer: issuerNew,
			PaymentAmounts: PaymentAmounts{
				Success: "25.0000000",
				Average: "25.0000000",
				Total:   "25.0000000",
			},
		},
		{
			// assets.issuer is NOT NULL and holds the empty string for native XLM.
			AssetCode:   "XLM",
			AssetIssuer: "",
			PaymentAmounts: PaymentAmounts{
				Success: "7.0000000",
				Average: "7.0000000",
				Total:   "7.0000000",
			},
		},
	}, paymentsAmountByAsset)
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
		wallet := data.CreateWalletFixture(t, ctx, dbConnectionPool, "wallet1", "https://www.wallet.com", "www.wallet.com", "wallet1://")

		disbursement := data.CreateDisbursementFixture(t, ctx, dbConnectionPool, model.Disbursements, &data.Disbursement{
			Status: data.DraftDisbursementStatus,
			StatusHistory: []data.DisbursementStatusHistoryEntry{
				{
					Status: data.DraftDisbursementStatus,
					UserID: "user1",
				},
			},
			Asset:  asset,
			Wallet: wallet,
		})
		exists, err := checkIfDisbursementExists(context.Background(), dbConnectionPool, disbursement.ID)
		require.NoError(t, err)
		assert.True(t, exists)
	})
}

// Test_getTotalReceivers_walletScope pins the count to the same rule the receivers list filters on.
// Multi-wallet scoped both by payment alone, which dropped receivers that had been created but never
// paid — they showed up in neither the list nor the count, not even for the account that made them.
func Test_getTotalReceivers_walletScope(t *testing.T) {
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
		VALUES ('stats-wallet-b', 'DISTRIBUTION_ACCOUNT.STELLAR.DB_VAULT') RETURNING id`))

	// Unpaid, created under A.
	data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{SourceWalletID: walletA.ID})
	// Unpaid, created under B.
	data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{SourceWalletID: walletBID})

	// Created under B, but paid from A: additive, so both accounts reach them.
	paidFromA := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{SourceWalletID: walletBID})
	d := data.CreateDisbursementFixture(t, ctx, dbConnectionPool, models.Disbursements, &data.Disbursement{
		Name: "stats-disb-a", SourceWalletID: walletA.ID, Status: data.StartedDisbursementStatus,
	})
	rw := data.CreateReceiverWalletFixture(t, ctx, dbConnectionPool, paidFromA.ID, d.Wallet.ID, data.ReadyReceiversWalletStatus)
	data.CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &data.Payment{
		ReceiverWallet: rw, Disbursement: d, Asset: *d.Asset, Amount: "3", Status: data.DraftPaymentStatus,
	})

	totalA, err := getTotalReceivers(ctx, dbConnectionPool, "", []string{walletA.ID})
	require.NoError(t, err)
	assert.Equal(t, int64(2), totalA, "the one A created plus the one A paid")

	totalB, err := getTotalReceivers(ctx, dbConnectionPool, "", []string{walletBID})
	require.NoError(t, err)
	assert.Equal(t, int64(2), totalB, "both of B's own, including the one A also reaches")

	totalTenant, err := getTotalReceivers(ctx, dbConnectionPool, "", nil)
	require.NoError(t, err)
	assert.Equal(t, int64(3), totalTenant, "a nil scope stays tenant-wide")
}
