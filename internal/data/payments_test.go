package data

import (
	"context"
	"testing"
	"time"

	"github.com/lib/pq"
	"github.com/stellar/go/support/log"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/db"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/db/dbtest"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_PaymentsModelGet(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()

	asset := CreateAssetFixture(t, ctx, dbConnectionPool, "USDC", "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVV")
	country := CreateCountryFixture(t, ctx, dbConnectionPool, "FRA", "France")
	wallet1 := CreateWalletFixture(t, ctx, dbConnectionPool, "wallet1", "https://www.wallet.com", "www.wallet.com", "wallet1://")

	receiver := CreateReceiverFixture(t, ctx, dbConnectionPool, &Receiver{})
	receiverWallet1 := CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver.ID, wallet1.ID, DraftReceiversWalletStatus)

	disbursementModel := DisbursementModel{dbConnectionPool: dbConnectionPool}
	disbursement1 := CreateDisbursementFixture(t, ctx, dbConnectionPool, &disbursementModel, &Disbursement{
		Name:      "disbursement 1",
		Status:    DraftDisbursementStatus,
		Asset:     asset,
		Wallet:    wallet1,
		Country:   country,
		CreatedAt: time.Date(2022, 3, 21, 23, 40, 20, 1431, time.UTC),
	})

	paymentModel := PaymentModel{dbConnectionPool: dbConnectionPool}

	t.Run("returns error when payment does not exist", func(t *testing.T) {
		_, err := paymentModel.Get(ctx, "invalid_id", dbConnectionPool)
		require.Error(t, err)
		require.Equal(t, ErrRecordNotFound, err)
	})

	t.Run("returns payment successfully", func(t *testing.T) {
		stellarTransactionID, err := utils.RandomString(64)
		require.NoError(t, err)
		stellarOperationID, err := utils.RandomString(32)
		require.NoError(t, err)

		expected := CreatePaymentFixture(t, ctx, dbConnectionPool, &paymentModel, &Payment{
			Amount:               "50",
			StellarTransactionID: stellarTransactionID,
			StellarOperationID:   stellarOperationID,
			Status:               DraftPaymentStatus,
			StatusHistory: []PaymentStatusHistoryEntry{
				{
					Status:        DraftPaymentStatus,
					StatusMessage: "",
					Timestamp:     time.Now(),
				},
			},
			Disbursement:   disbursement1,
			Asset:          *asset,
			ReceiverWallet: receiverWallet1,
		})
		actual, err := paymentModel.Get(ctx, expected.ID, dbConnectionPool)
		require.NoError(t, err)

		assert.Equal(t, *expected, *actual)
	})

	t.Run("returns payment successfully receiver with multiple wallets", func(t *testing.T) {
		wallet2 := CreateWalletFixture(t, ctx, dbConnectionPool, "wallet2", "https://www.wallet2.com", "www.wallet2.com", "wallet2://")

		receiverWallet2 := CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver.ID, wallet2.ID, DraftReceiversWalletStatus)

		disbursement2 := CreateDisbursementFixture(t, ctx, dbConnectionPool, &disbursementModel, &Disbursement{
			Name:    "disbursement 2",
			Status:  DraftDisbursementStatus,
			Asset:   asset,
			Wallet:  wallet2,
			Country: country,
		})

		stellarTransactionID, err := utils.RandomString(64)
		require.NoError(t, err)
		stellarOperationID, err := utils.RandomString(32)
		require.NoError(t, err)

		expected := CreatePaymentFixture(t, ctx, dbConnectionPool, &paymentModel, &Payment{
			Amount:               "50",
			StellarTransactionID: stellarTransactionID,
			StellarOperationID:   stellarOperationID,
			Status:               DraftPaymentStatus,
			StatusHistory: []PaymentStatusHistoryEntry{
				{
					Status:        DraftPaymentStatus,
					StatusMessage: "",
					Timestamp:     time.Now(),
				},
			},
			Disbursement:   disbursement2,
			Asset:          *asset,
			ReceiverWallet: receiverWallet2,
		})
		actual, err := paymentModel.Get(ctx, expected.ID, dbConnectionPool)
		require.NoError(t, err)

		assert.Equal(t, *expected, *actual)
	})
}

func Test_PaymentModelCount(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()

	asset := CreateAssetFixture(t, ctx, dbConnectionPool, "USDC", "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVV")
	country := CreateCountryFixture(t, ctx, dbConnectionPool, "FRA", "France")
	wallet := CreateWalletFixture(t, ctx, dbConnectionPool, "wallet1", "https://www.wallet.com", "www.wallet.com", "wallet1://")

	receiver := CreateReceiverFixture(t, ctx, dbConnectionPool, &Receiver{})
	receiverWallet := CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver.ID, wallet.ID, DraftReceiversWalletStatus)

	disbursementModel := DisbursementModel{dbConnectionPool: dbConnectionPool}
	disbursement1 := CreateDisbursementFixture(t, ctx, dbConnectionPool, &disbursementModel, &Disbursement{
		Name:    "disbursement 1",
		Status:  DraftDisbursementStatus,
		Asset:   asset,
		Wallet:  wallet,
		Country: country,
	})

	disbursement2 := CreateDisbursementFixture(t, ctx, dbConnectionPool, &disbursementModel, &Disbursement{
		Name:    "disbursement 2",
		Status:  DraftDisbursementStatus,
		Asset:   asset,
		Wallet:  wallet,
		Country: country,
	})

	paymentModel := PaymentModel{dbConnectionPool: dbConnectionPool}

	t.Run("returns 0 when no payments exist", func(t *testing.T) {
		count, errPayment := paymentModel.Count(ctx, &QueryParams{}, dbConnectionPool)
		require.NoError(t, errPayment)
		assert.Equal(t, 0, count)
	})

	stellarTransactionID, err := utils.RandomString(64)
	require.NoError(t, err)
	stellarOperationID, err := utils.RandomString(32)
	require.NoError(t, err)

	CreatePaymentFixture(t, ctx, dbConnectionPool, &paymentModel, &Payment{
		Amount:               "50",
		StellarTransactionID: stellarTransactionID,
		StellarOperationID:   stellarOperationID,
		Status:               DraftPaymentStatus,
		StatusHistory: []PaymentStatusHistoryEntry{
			{
				Status:        DraftPaymentStatus,
				StatusMessage: "",
				Timestamp:     time.Now(),
			},
		},
		Disbursement:   disbursement1,
		Asset:          *asset,
		ReceiverWallet: receiverWallet,
	})

	stellarTransactionID, err = utils.RandomString(64)
	require.NoError(t, err)
	stellarOperationID, err = utils.RandomString(32)
	require.NoError(t, err)

	CreatePaymentFixture(t, ctx, dbConnectionPool, &paymentModel, &Payment{
		Amount:               "150",
		StellarTransactionID: stellarTransactionID,
		StellarOperationID:   stellarOperationID,
		Status:               PendingPaymentStatus,
		StatusHistory: []PaymentStatusHistoryEntry{
			{
				Status:        PendingPaymentStatus,
				StatusMessage: "",
				Timestamp:     time.Now(),
			},
		},
		Disbursement:   disbursement2,
		Asset:          *asset,
		ReceiverWallet: receiverWallet,
	})

	t.Run("returns count of payments", func(t *testing.T) {
		count, err := paymentModel.Count(ctx, &QueryParams{}, dbConnectionPool)
		require.NoError(t, err)
		assert.Equal(t, 2, count)
	})

	t.Run("returns count of payments with filter", func(t *testing.T) {
		filters := map[FilterKey]interface{}{
			FilterKeyStatus: DraftPaymentStatus,
		}
		count, err := paymentModel.Count(ctx, &QueryParams{Filters: filters}, dbConnectionPool)
		require.NoError(t, err)
		assert.Equal(t, 1, count)
	})
}

func Test_PaymentModelGetAll(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()

	asset := CreateAssetFixture(t, ctx, dbConnectionPool, "USDC", "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVV")
	country := CreateCountryFixture(t, ctx, dbConnectionPool, "FRA", "France")
	wallet := CreateWalletFixture(t, ctx, dbConnectionPool, "wallet1", "https://www.wallet.com", "www.wallet.com", "wallet1://")

	receiver := CreateReceiverFixture(t, ctx, dbConnectionPool, &Receiver{})
	receiverWallet := CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver.ID, wallet.ID, DraftReceiversWalletStatus)

	disbursementModel := DisbursementModel{dbConnectionPool: dbConnectionPool}
	disbursement1 := CreateDisbursementFixture(t, ctx, dbConnectionPool, &disbursementModel, &Disbursement{
		Name:    "disbursement 1",
		Status:  DraftDisbursementStatus,
		Asset:   asset,
		Wallet:  wallet,
		Country: country,
	})

	disbursement2 := CreateDisbursementFixture(t, ctx, dbConnectionPool, &disbursementModel, &Disbursement{
		Name:    "disbursement 2",
		Status:  DraftDisbursementStatus,
		Asset:   asset,
		Wallet:  wallet,
		Country: country,
	})

	paymentModel := PaymentModel{dbConnectionPool: dbConnectionPool}

	t.Run("returns empty list when no payments exist", func(t *testing.T) {
		payments, errPayment := paymentModel.GetAll(ctx, &QueryParams{}, dbConnectionPool)
		require.NoError(t, errPayment)
		assert.Equal(t, 0, len(payments))
	})

	stellarTransactionID, err := utils.RandomString(64)
	require.NoError(t, err)
	stellarOperationID, err := utils.RandomString(32)
	require.NoError(t, err)

	expectedPayment1 := CreatePaymentFixture(t, ctx, dbConnectionPool, &paymentModel, &Payment{
		Amount:               "50",
		StellarTransactionID: stellarTransactionID,
		StellarOperationID:   stellarOperationID,
		Status:               DraftPaymentStatus,
		StatusHistory: []PaymentStatusHistoryEntry{
			{
				Status:        DraftPaymentStatus,
				StatusMessage: "",
				Timestamp:     time.Now(),
			},
		},
		Disbursement:   disbursement1,
		Asset:          *asset,
		ReceiverWallet: receiverWallet,
	})

	stellarTransactionID, err = utils.RandomString(64)
	require.NoError(t, err)
	stellarOperationID, err = utils.RandomString(32)
	require.NoError(t, err)

	expectedPayment2 := CreatePaymentFixture(t, ctx, dbConnectionPool, &paymentModel, &Payment{
		Amount:               "150",
		StellarTransactionID: stellarTransactionID,
		StellarOperationID:   stellarOperationID,
		Status:               PendingPaymentStatus,
		StatusHistory: []PaymentStatusHistoryEntry{
			{
				Status:        DraftPaymentStatus,
				StatusMessage: "",
				Timestamp:     time.Now(),
			},
		},
		Disbursement:   disbursement2,
		Asset:          *asset,
		ReceiverWallet: receiverWallet,
	})

	t.Run("returns payments successfully", func(t *testing.T) {
		actualPayments, err := paymentModel.GetAll(ctx, &QueryParams{}, dbConnectionPool)
		require.NoError(t, err)
		assert.Equal(t, 2, len(actualPayments))
		assert.Equal(t, []Payment{*expectedPayment2, *expectedPayment1}, actualPayments)
	})

	t.Run("returns payments successfully with limit", func(t *testing.T) {
		actualPayments, err := paymentModel.GetAll(ctx, &QueryParams{Page: 1, PageLimit: 1}, dbConnectionPool)
		require.NoError(t, err)
		assert.Equal(t, 1, len(actualPayments))
		assert.Equal(t, []Payment{*expectedPayment1}, actualPayments)
	})

	t.Run("returns payments successfully with offset", func(t *testing.T) {
		actualPayments, err := paymentModel.GetAll(ctx, &QueryParams{Page: 2, PageLimit: 1}, dbConnectionPool)
		require.NoError(t, err)
		assert.Equal(t, 1, len(actualPayments))
		assert.Equal(t, []Payment{*expectedPayment2}, actualPayments)
	})

	t.Run("returns payments successfully with created at order", func(t *testing.T) {
		actualPayments, err := paymentModel.GetAll(ctx, &QueryParams{SortBy: SortFieldCreatedAt, SortOrder: SortOrderASC}, dbConnectionPool)

		require.NoError(t, err)
		assert.Equal(t, 2, len(actualPayments))
		assert.Equal(t, []Payment{*expectedPayment1, *expectedPayment2}, actualPayments)
	})

	t.Run("returns payments successfully with updated at order", func(t *testing.T) {
		actualPayments, err := paymentModel.GetAll(ctx, &QueryParams{SortBy: SortFieldUpdatedAt, SortOrder: SortOrderASC}, dbConnectionPool)

		require.NoError(t, err)
		assert.Equal(t, 2, len(actualPayments))
		assert.Equal(t, []Payment{*expectedPayment1, *expectedPayment2}, actualPayments)
	})

	t.Run("returns payments successfully with filter", func(t *testing.T) {
		filters := map[FilterKey]interface{}{
			FilterKeyStatus: PendingPaymentStatus,
		}
		actualPayments, err := paymentModel.GetAll(ctx, &QueryParams{Filters: filters}, dbConnectionPool)
		require.NoError(t, err)
		assert.Equal(t, 1, len(actualPayments))
		assert.Equal(t, []Payment{*expectedPayment2}, actualPayments)
	})

	t.Run("should not return duplicated entries when receiver are in more than one disbursements with different wallets", func(t *testing.T) {
		models, err := NewModels(dbConnectionPool)
		require.NoError(t, err)

		DeleteAllPaymentsFixtures(t, ctx, dbConnectionPool)
		DeleteAllDisbursementFixtures(t, ctx, dbConnectionPool)
		DeleteAllCountryFixtures(t, ctx, dbConnectionPool)
		DeleteAllAssetFixtures(t, ctx, dbConnectionPool)
		DeleteAllReceiverWalletsFixtures(t, ctx, dbConnectionPool)
		DeleteAllReceiversFixtures(t, ctx, dbConnectionPool)
		DeleteAllWalletFixtures(t, ctx, dbConnectionPool)

		usdc := CreateAssetFixture(t, ctx, dbConnectionPool, "USDC", "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVV")
		country := CreateCountryFixture(t, ctx, dbConnectionPool, "BRA", "Brazil")
		demoWallet := CreateWalletFixture(t, ctx, dbConnectionPool, "Demo Wallet", "https://demo-wallet.stellar.org", "https://demo-wallet.stellar.org", "demo-wallet-server.stellar.org")
		vibrantWallet := CreateWalletFixture(t, ctx, dbConnectionPool, "Vibrant Assist", "https://vibrantapp.com", "api-dev.vibrantapp.com", "https://vibrantapp.com/sdp-dev")

		receiver := CreateReceiverFixture(t, ctx, dbConnectionPool, &Receiver{})
		receiverDemoWallet := CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver.ID, demoWallet.ID, ReadyReceiversWalletStatus)
		receiverVibrantWallet := CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver.ID, vibrantWallet.ID, ReadyReceiversWalletStatus)

		disbursement1 := CreateDisbursementFixture(t, ctx, dbConnectionPool, models.Disbursements, &Disbursement{
			Name:    "disbursement 1",
			Status:  ReadyDisbursementStatus,
			Asset:   usdc,
			Wallet:  demoWallet,
			Country: country,
		})

		disbursement2 := CreateDisbursementFixture(t, ctx, dbConnectionPool, models.Disbursements, &Disbursement{
			Name:    "disbursement 2",
			Status:  ReadyDisbursementStatus,
			Asset:   usdc,
			Wallet:  vibrantWallet,
			Country: country,
		})

		demoWalletPayment := CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &Payment{
			Amount:         "100",
			Status:         ReadyPaymentStatus,
			Disbursement:   disbursement1,
			Asset:          *usdc,
			ReceiverWallet: receiverDemoWallet,
		})

		vibrantWalletPayment := CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &Payment{
			Amount:         "100",
			Status:         ReadyPaymentStatus,
			Disbursement:   disbursement2,
			Asset:          *usdc,
			ReceiverWallet: receiverVibrantWallet,
		})

		payments, err := models.Payment.GetAll(ctx, &QueryParams{
			Filters: map[FilterKey]interface{}{
				FilterKeyReceiverID: receiver.ID,
			},
		}, dbConnectionPool)
		require.NoError(t, err)

		assert.Len(t, payments, 2)
		assert.Equal(t, []Payment{
			*demoWalletPayment,
			*vibrantWalletPayment,
		}, payments)
	})
}

// func Test_PaymentsModelGetByIDs(t *testing.T) {
// 	dbt := dbtest.Open(t)
// 	defer dbt.Close()

// 	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
// 	require.NoError(t, err)
// 	defer dbConnectionPool.Close()

// 	ctx := context.Background()

// 	asset := CreateAssetFixture(t, ctx, dbConnectionPool, "USDC", "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVV")
// 	country := CreateCountryFixture(t, ctx, dbConnectionPool, "FRA", "France")
// 	wallet1 := CreateWalletFixture(t, ctx, dbConnectionPool, "wallet1", "https://www.wallet.com", "www.wallet.com", "wallet1://")

// 	receiver1 := CreateReceiverFixture(t, ctx, dbConnectionPool, &Receiver{})
// 	receiverWallet1 := CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver1.ID, wallet1.ID, DraftReceiversWalletStatus)

// 	receiver2 := CreateReceiverFixture(t, ctx, dbConnectionPool, &Receiver{})
// 	receiverWallet2 := CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver2.ID, wallet1.ID, DraftReceiversWalletStatus)

// 	disbursementModel := DisbursementModel{dbConnectionPool: dbConnectionPool}
// 	disbursement1 := CreateDisbursementFixture(t, ctx, dbConnectionPool, &disbursementModel, &Disbursement{
// 		Name:      "disbursement 1",
// 		Status:    Draft,
// 		Asset:     asset,
// 		Wallet:    wallet1,
// 		Country:   country,
// 		CreatedAt: time.Date(2022, 3, 21, 23, 40, 20, 1431, time.UTC),
// 	})

// 	paymentModel := PaymentModel{dbConnectionPool: dbConnectionPool}

// 	t.Run("returns empty list when payments ids are not found", func(t *testing.T) {
// 		payments, err := paymentModel.GetByIDs(ctx, dbConnectionPool, []string{"invalid_id"})
// 		require.NoError(t, err)
// 		require.Empty(t, payments)
// 	})

// 	t.Run("returns payments successfully", func(t *testing.T) {
// 		stellarTransactionID, err := utils.RandomString(64)
// 		require.NoError(t, err)
// 		stellarOperationID, err := utils.RandomString(32)
// 		require.NoError(t, err)

// 		payment1 := CreatePaymentFixture(t, ctx, dbConnectionPool, &paymentModel, &Payment{
// 			Amount:               "50",
// 			StellarTransactionID: stellarTransactionID,
// 			StellarOperationID:   stellarOperationID,
// 			Status:               DraftPaymentStatus,
// 			StatusHistory: []PaymentStatusHistoryEntry{
// 				{
// 					Status:        DraftPaymentStatus,
// 					StatusMessage: "",
// 					Timestamp:     time.Now(),
// 				},
// 			},
// 			Disbursement:   disbursement1,
// 			Asset:          *asset,
// 			ReceiverWallet: receiverWallet1,
// 		})

// 		stellarTransactionID, err = utils.RandomString(64)
// 		require.NoError(t, err)
// 		stellarOperationID, err = utils.RandomString(32)
// 		require.NoError(t, err)

// 		payment2 := CreatePaymentFixture(t, ctx, dbConnectionPool, &paymentModel, &Payment{
// 			Amount:               "150",
// 			StellarTransactionID: stellarTransactionID,
// 			StellarOperationID:   stellarOperationID,
// 			Status:               DraftPaymentStatus,
// 			StatusHistory: []PaymentStatusHistoryEntry{
// 				{
// 					Status:        DraftPaymentStatus,
// 					StatusMessage: "",
// 					Timestamp:     time.Now(),
// 				},
// 			},
// 			Disbursement:   disbursement1,
// 			Asset:          *asset,
// 			ReceiverWallet: receiverWallet2,
// 		})
// 		actual, err := paymentModel.GetByIDs(ctx, dbConnectionPool, []string{payment1.ID, payment2.ID})
// 		require.NoError(t, err)

// 		p1 := Payment{
// 			ID:                   payment1.ID,
// 			Amount:               payment1.Amount,
// 			StellarTransactionID: payment1.StellarTransactionID,
// 			StellarOperationID:   payment1.StellarOperationID,
// 			Status:               payment1.Status,
// 			CreatedAt:            payment1.CreatedAt,
// 			UpdatedAt:            payment1.UpdatedAt,
// 			Disbursement: &Disbursement{
// 				ID:     payment1.Disbursement.ID,
// 				Status: payment1.Disbursement.Status,
// 			},
// 			Asset: Asset{
// 				ID:     payment1.Asset.ID,
// 				Code:   payment1.Asset.Code,
// 				Issuer: payment1.Asset.Issuer,
// 			},
// 			ReceiverWallet: &ReceiverWallet{
// 				ID:              payment1.ReceiverWallet.ID,
// 				StellarAddress:  payment1.ReceiverWallet.StellarAddress,
// 				StellarMemo:     payment1.ReceiverWallet.StellarMemo,
// 				StellarMemoType: payment1.ReceiverWallet.StellarMemoType,
// 				Status:          payment1.ReceiverWallet.Status,
// 				Receiver: Receiver{
// 					ID: payment1.ReceiverWallet.Receiver.ID,
// 				},
// 			},
// 		}

// 		p2 := Payment{
// 			ID:                   payment2.ID,
// 			Amount:               payment2.Amount,
// 			StellarTransactionID: payment2.StellarTransactionID,
// 			StellarOperationID:   payment2.StellarOperationID,
// 			Status:               payment2.Status,
// 			CreatedAt:            payment2.CreatedAt,
// 			UpdatedAt:            payment2.UpdatedAt,
// 			Disbursement: &Disbursement{
// 				ID:     payment2.Disbursement.ID,
// 				Status: payment2.Disbursement.Status,
// 			},
// 			Asset: Asset{
// 				ID:     payment2.Asset.ID,
// 				Code:   payment2.Asset.Code,
// 				Issuer: payment2.Asset.Issuer,
// 			},
// 			ReceiverWallet: &ReceiverWallet{
// 				ID:              payment2.ReceiverWallet.ID,
// 				StellarAddress:  payment2.ReceiverWallet.StellarAddress,
// 				StellarMemo:     payment2.ReceiverWallet.StellarMemo,
// 				StellarMemoType: payment2.ReceiverWallet.StellarMemoType,
// 				Status:          payment2.ReceiverWallet.Status,
// 				Receiver: Receiver{
// 					ID: payment2.ReceiverWallet.Receiver.ID,
// 				},
// 			},
// 		}

// 		payments := []*Payment{&p1, &p2}
// 		assert.Equal(t, payments, actual)
// 	})
// }

func Test_PaymentNewPaymentQuery(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	testCases := []struct {
		name           string
		baseQuery      string
		queryParams    QueryParams
		paginated      bool
		expectedQuery  string
		expectedParams []interface{}
	}{
		{
			name:           "build payment query without params and pagination",
			baseQuery:      "SELECT * FROM payments p",
			queryParams:    QueryParams{},
			paginated:      false,
			expectedQuery:  "SELECT * FROM payments p",
			expectedParams: []interface{}{},
		},
		{
			name:      "build payment query with status filter",
			baseQuery: "SELECT * FROM payments p",
			queryParams: QueryParams{
				Filters: map[FilterKey]interface{}{
					FilterKeyStatus: "draft",
				},
			},
			paginated:      false,
			expectedQuery:  "SELECT * FROM payments p WHERE 1=1 AND p.status = $1",
			expectedParams: []interface{}{"draft"},
		},
		{
			name:      "build payment query with receiver_id filter",
			baseQuery: "SELECT * FROM payments p",
			queryParams: QueryParams{
				Filters: map[FilterKey]interface{}{
					FilterKeyReceiverID: "receiver_id",
				},
			},
			paginated:      false,
			expectedQuery:  "SELECT * FROM payments p WHERE 1=1 AND p.receiver_id = $1",
			expectedParams: []interface{}{"receiver_id"},
		},
		{
			name:      "build payment query with created_at filters",
			baseQuery: "SELECT * FROM payments p",
			queryParams: QueryParams{
				Filters: map[FilterKey]interface{}{
					FilterKeyCreatedAtAfter:  "00-01-01",
					FilterKeyCreatedAtBefore: "00-01-31",
				},
			},
			paginated:      false,
			expectedQuery:  "SELECT * FROM payments p WHERE 1=1 AND p.created_at >= $1 AND p.created_at <= $2",
			expectedParams: []interface{}{"00-01-01", "00-01-31"},
		},
		{
			name:      "build payment query with pagination",
			baseQuery: "SELECT * FROM payments p",
			queryParams: QueryParams{
				Page:      1,
				PageLimit: 20,
				SortBy:    "created_at",
				SortOrder: "ASC",
			},
			paginated:      true,
			expectedQuery:  "SELECT * FROM payments p ORDER BY p.created_at ASC LIMIT $1 OFFSET $2",
			expectedParams: []interface{}{20, 0},
		},
		{
			name:      "build payment query with all filters and pagination",
			baseQuery: "SELECT * FROM payments p",
			queryParams: QueryParams{
				Page:      1,
				PageLimit: 20,
				SortBy:    "created_at",
				SortOrder: "ASC",
				Filters: map[FilterKey]interface{}{
					FilterKeyStatus:          "draft",
					FilterKeyReceiverID:      "receiver_id",
					FilterKeyCreatedAtAfter:  "00-01-01",
					FilterKeyCreatedAtBefore: "00-01-31",
				},
			},
			paginated:      true,
			expectedQuery:  "SELECT * FROM payments p WHERE 1=1 AND p.status = $1 AND p.receiver_id = $2 AND p.created_at >= $3 AND p.created_at <= $4 ORDER BY p.created_at ASC LIMIT $5 OFFSET $6",
			expectedParams: []interface{}{"draft", "receiver_id", "00-01-01", "00-01-31", 20, 0},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			query, params := newPaymentQuery(tc.baseQuery, &tc.queryParams, tc.paginated, dbConnectionPool)

			assert.Equal(t, tc.expectedQuery, query)
			assert.Equal(t, tc.expectedParams, params)
		})
	}
}

func Test_PaymentModelRetryFailedPayments(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()

	models, err := NewModels(dbConnectionPool)
	require.NoError(t, err)

	DeleteAllPaymentsFixtures(t, ctx, dbConnectionPool)
	DeleteAllDisbursementFixtures(t, ctx, dbConnectionPool)
	DeleteAllCountryFixtures(t, ctx, dbConnectionPool)
	DeleteAllAssetFixtures(t, ctx, dbConnectionPool)
	DeleteAllReceiverWalletsFixtures(t, ctx, dbConnectionPool)
	DeleteAllReceiversFixtures(t, ctx, dbConnectionPool)
	DeleteAllWalletFixtures(t, ctx, dbConnectionPool)

	country := CreateCountryFixture(t, ctx, dbConnectionPool, "BRA", "Brazil")
	wallet := CreateWalletFixture(t, ctx, dbConnectionPool, "Wallet", "https://www.wallet.com", "www.wallet.com", "wallet://")
	asset := CreateAssetFixture(t, ctx, dbConnectionPool, "USDC", "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVV")

	receiver := CreateReceiverFixture(t, ctx, dbConnectionPool, &Receiver{})
	receiverWallet := CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver.ID, wallet.ID, ReadyReceiversWalletStatus)

	disbursement := CreateDisbursementFixture(t, ctx, dbConnectionPool, models.Disbursements, &Disbursement{
		Country:           country,
		Wallet:            wallet,
		Asset:             asset,
		Status:            ReadyDisbursementStatus,
		VerificationField: VerificationFieldDateOfBirth,
	})

	t.Run("does not update payments when no payments IDs is given", func(t *testing.T) {
		err := models.Payment.RetryFailedPayments(ctx, "user@test.com")
		assert.ErrorIs(t, err, ErrMissingInput)
	})

	t.Run("does not update payments when email is empty", func(t *testing.T) {
		err := models.Payment.RetryFailedPayments(ctx, "", "payment-id")
		assert.ErrorIs(t, err, ErrMissingInput)
	})

	t.Run("returns error when no rows is affected", func(t *testing.T) {
		DeleteAllPaymentsFixtures(t, ctx, dbConnectionPool)

		payment1 := CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &Payment{
			Amount:               "1",
			StellarTransactionID: "stellar-transaction-id-1",
			StellarOperationID:   "operation-id-1",
			Status:               PendingPaymentStatus,
			Disbursement:         disbursement,
			ReceiverWallet:       receiverWallet,
			Asset:                *asset,
		})

		payment2 := CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &Payment{
			Amount:               "1",
			StellarTransactionID: "stellar-transaction-id-2",
			StellarOperationID:   "operation-id-2",
			Status:               ReadyPaymentStatus,
			Disbursement:         disbursement,
			ReceiverWallet:       receiverWallet,
			Asset:                *asset,
		})

		err := models.Payment.RetryFailedPayments(ctx, "user@test.com", payment1.ID, payment2.ID)
		assert.ErrorIs(t, err, ErrMismatchNumRowsAffected)

		payment1DB, err := models.Payment.Get(ctx, payment1.ID, dbConnectionPool)
		require.NoError(t, err)

		payment2DB, err := models.Payment.Get(ctx, payment2.ID, dbConnectionPool)
		require.NoError(t, err)

		// Payment 1
		assert.Equal(t, PendingPaymentStatus, payment1DB.Status)
		assert.Equal(t, payment1.StellarTransactionID, payment1DB.StellarTransactionID)
		assert.Equal(t, payment1.StatusHistory, payment1DB.StatusHistory)

		// Payment 2
		assert.Equal(t, ReadyPaymentStatus, payment2DB.Status)
		assert.Equal(t, payment2.StellarTransactionID, payment2DB.StellarTransactionID)
		assert.Equal(t, payment2.StatusHistory, payment2DB.StatusHistory)
	})

	t.Run("returns error when the number of affected rows is different from the length of payment IDs", func(t *testing.T) {
		DeleteAllPaymentsFixtures(t, ctx, dbConnectionPool)

		payment1 := CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &Payment{
			Amount:               "1",
			StellarTransactionID: "stellar-transaction-id-1",
			StellarOperationID:   "operation-id-1",
			Status:               PendingPaymentStatus,
			Disbursement:         disbursement,
			ReceiverWallet:       receiverWallet,
			Asset:                *asset,
		})

		payment2 := CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &Payment{
			Amount:               "1",
			StellarTransactionID: "stellar-transaction-id-2",
			StellarOperationID:   "operation-id-2",
			Status:               ReadyPaymentStatus,
			Disbursement:         disbursement,
			ReceiverWallet:       receiverWallet,
			Asset:                *asset,
		})

		payment3 := CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &Payment{
			Amount:               "1",
			StellarTransactionID: "stellar-transaction-id-3",
			StellarOperationID:   "operation-id-3",
			Status:               FailedPaymentStatus,
			Disbursement:         disbursement,
			ReceiverWallet:       receiverWallet,
			Asset:                *asset,
		})

		err := models.Payment.RetryFailedPayments(ctx, "user@test.com", payment1.ID, payment2.ID, payment3.ID)
		assert.ErrorIs(t, err, ErrMismatchNumRowsAffected)

		payment1DB, err := models.Payment.Get(ctx, payment1.ID, dbConnectionPool)
		require.NoError(t, err)

		payment2DB, err := models.Payment.Get(ctx, payment2.ID, dbConnectionPool)
		require.NoError(t, err)

		payment3DB, err := models.Payment.Get(ctx, payment3.ID, dbConnectionPool)
		require.NoError(t, err)

		// Payment 1
		assert.Equal(t, PendingPaymentStatus, payment1DB.Status)
		assert.Equal(t, payment1.StellarTransactionID, payment1DB.StellarTransactionID)
		assert.Equal(t, payment1.StatusHistory, payment1DB.StatusHistory)

		// Payment 2
		assert.Equal(t, ReadyPaymentStatus, payment2DB.Status)
		assert.Equal(t, payment2.StellarTransactionID, payment2DB.StellarTransactionID)
		assert.Equal(t, payment2.StatusHistory, payment2DB.StatusHistory)

		// Payment 3
		assert.Equal(t, FailedPaymentStatus, payment3DB.Status)
		assert.Equal(t, payment3.StellarTransactionID, payment3DB.StellarTransactionID)
		assert.Equal(t, payment3.StatusHistory, payment3DB.StatusHistory)
	})

	t.Run("successfully updates failed payments", func(t *testing.T) {
		DeleteAllPaymentsFixtures(t, ctx, dbConnectionPool)

		payment1 := CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &Payment{
			Amount:               "1",
			StellarTransactionID: "stellar-transaction-id-1",
			StellarOperationID:   "operation-id-1",
			Status:               FailedPaymentStatus,
			Disbursement:         disbursement,
			ReceiverWallet:       receiverWallet,
			Asset:                *asset,
		})

		payment2 := CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &Payment{
			Amount:               "1",
			StellarTransactionID: "stellar-transaction-id-2",
			StellarOperationID:   "operation-id-2",
			Status:               FailedPaymentStatus,
			Disbursement:         disbursement,
			ReceiverWallet:       receiverWallet,
			Asset:                *asset,
		})

		err := models.Payment.RetryFailedPayments(ctx, "user@test.com", payment1.ID, payment2.ID)
		require.NoError(t, err)

		payment1DB, err := models.Payment.Get(ctx, payment1.ID, dbConnectionPool)
		require.NoError(t, err)

		payment2DB, err := models.Payment.Get(ctx, payment2.ID, dbConnectionPool)
		require.NoError(t, err)

		// Payment 1
		assert.Equal(t, ReadyPaymentStatus, payment1DB.Status)
		assert.Empty(t, payment1DB.StellarTransactionID)
		assert.NotEqual(t, payment1.StatusHistory, payment1DB.StatusHistory)
		assert.Len(t, payment1DB.StatusHistory, 2)
		assert.Equal(t, ReadyPaymentStatus, payment1DB.StatusHistory[1].Status)
		assert.Equal(t, "User user@test.com has requested to retry the payment - Previous Stellar Transaction ID: stellar-transaction-id-1", payment1DB.StatusHistory[1].StatusMessage)

		// Payment 2
		assert.Equal(t, ReadyPaymentStatus, payment2DB.Status)
		assert.Empty(t, payment2DB.StellarTransactionID)
		assert.NotEqual(t, payment2.StatusHistory, payment2DB.StatusHistory)
		assert.Len(t, payment2DB.StatusHistory, 2)
		assert.Equal(t, ReadyPaymentStatus, payment2DB.StatusHistory[1].Status)
		assert.Equal(t, "User user@test.com has requested to retry the payment - Previous Stellar Transaction ID: stellar-transaction-id-2", payment2DB.StatusHistory[1].StatusMessage)
	})

	t.Run("resets the anchor_platform_synced_at for the receiver wallets", func(t *testing.T) {
		DeleteAllPaymentsFixtures(t, ctx, dbConnectionPool)

		recv := CreateReceiverFixture(t, ctx, dbConnectionPool, &Receiver{})
		rw := CreateReceiverWalletFixture(t, ctx, dbConnectionPool, recv.ID, wallet.ID, ReadyReceiversWalletStatus)

		q := "UPDATE receiver_wallets SET anchor_platform_transaction_synced_at = NOW() WHERE id = $1"
		_, err := dbConnectionPool.ExecContext(ctx, q, rw.ID)
		require.NoError(t, err)

		q = "SELECT anchor_platform_transaction_synced_at FROM receiver_wallets WHERE id = $1"
		var syncedAt pq.NullTime
		err = dbConnectionPool.GetContext(ctx, &syncedAt, q, rw.ID)
		require.NoError(t, err)
		assert.True(t, syncedAt.Valid)
		assert.False(t, syncedAt.Time.IsZero())

		payment1 := CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &Payment{
			Amount:               "1",
			StellarTransactionID: "stellar-transaction-id-1",
			StellarOperationID:   "operation-id-1",
			Status:               FailedPaymentStatus,
			Disbursement:         disbursement,
			ReceiverWallet:       rw,
			Asset:                *asset,
		})

		payment2 := CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &Payment{
			Amount:               "1",
			StellarTransactionID: "stellar-transaction-id-2",
			StellarOperationID:   "operation-id-2",
			Status:               FailedPaymentStatus,
			Disbursement:         disbursement,
			ReceiverWallet:       rw,
			Asset:                *asset,
		})

		err = models.Payment.RetryFailedPayments(ctx, "user@test.com", payment1.ID, payment2.ID)
		require.NoError(t, err)

		payment1DB, err := models.Payment.Get(ctx, payment1.ID, dbConnectionPool)
		require.NoError(t, err)

		payment2DB, err := models.Payment.Get(ctx, payment2.ID, dbConnectionPool)
		require.NoError(t, err)

		// Payment 1
		assert.Equal(t, ReadyPaymentStatus, payment1DB.Status)
		assert.Empty(t, payment1DB.StellarTransactionID)
		assert.NotEqual(t, payment1.StatusHistory, payment1DB.StatusHistory)
		assert.Len(t, payment1DB.StatusHistory, 2)
		assert.Equal(t, ReadyPaymentStatus, payment1DB.StatusHistory[1].Status)
		assert.Equal(t, "User user@test.com has requested to retry the payment - Previous Stellar Transaction ID: stellar-transaction-id-1", payment1DB.StatusHistory[1].StatusMessage)

		// Payment 2
		assert.Equal(t, ReadyPaymentStatus, payment2DB.Status)
		assert.Empty(t, payment2DB.StellarTransactionID)
		assert.NotEqual(t, payment2.StatusHistory, payment2DB.StatusHistory)
		assert.Len(t, payment2DB.StatusHistory, 2)
		assert.Equal(t, ReadyPaymentStatus, payment2DB.StatusHistory[1].Status)
		assert.Equal(t, "User user@test.com has requested to retry the payment - Previous Stellar Transaction ID: stellar-transaction-id-2", payment2DB.StatusHistory[1].StatusMessage)

		err = dbConnectionPool.GetContext(ctx, &syncedAt, q, rw.ID)
		require.NoError(t, err)
		assert.False(t, syncedAt.Valid)
		assert.True(t, syncedAt.Time.IsZero())
	})
}

func Test_PaymentModelGetAllReadyToPatchCompletionAnchorTransactions(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()

	models, err := NewModels(dbConnectionPool)
	require.NoError(t, err)

	t.Run("return empty", func(t *testing.T) {
		payments, err := models.Payment.GetAllReadyToPatchCompletionAnchorTransactions(ctx, dbConnectionPool)
		require.NoError(t, err)
		assert.Empty(t, payments)
	})

	t.Run("doesn't get payments when receiver wallet is not registered", func(t *testing.T) {
		DeleteAllFixtures(t, ctx, dbConnectionPool)

		country := CreateCountryFixture(t, ctx, dbConnectionPool, "BRA", "Brazil")
		wallet := CreateWalletFixture(t, ctx, dbConnectionPool, "Wallet", "https://www.wallet.com", "www.wallet.com", "wallet://")
		asset := CreateAssetFixture(t, ctx, dbConnectionPool, "USDC", "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVV")

		receiver := CreateReceiverFixture(t, ctx, dbConnectionPool, &Receiver{})
		receiverWallet := CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver.ID, wallet.ID, ReadyReceiversWalletStatus)

		disbursement := CreateDisbursementFixture(t, ctx, dbConnectionPool, models.Disbursements, &Disbursement{
			Country:           country,
			Wallet:            wallet,
			Asset:             asset,
			Status:            StartedDisbursementStatus,
			VerificationField: VerificationFieldDateOfBirth,
		})

		// It's not possible to have a payment in a end state when the receiver wallet is not registered yet
		// but this is for validation purposes.
		_ = CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &Payment{
			Amount:               "1",
			StellarTransactionID: "stellar-transaction-id-1",
			StellarOperationID:   "operation-id-1",
			Status:               SuccessPaymentStatus,
			Disbursement:         disbursement,
			ReceiverWallet:       receiverWallet,
			Asset:                *asset,
		})

		payments, err := models.Payment.GetAllReadyToPatchCompletionAnchorTransactions(ctx, dbConnectionPool)
		require.NoError(t, err)
		assert.Empty(t, payments)
	})

	t.Run("doesn't get payments not in the Success or Failed statuses", func(t *testing.T) {
		DeleteAllFixtures(t, ctx, dbConnectionPool)

		country := CreateCountryFixture(t, ctx, dbConnectionPool, "BRA", "Brazil")
		wallet := CreateWalletFixture(t, ctx, dbConnectionPool, "Wallet", "https://www.wallet.com", "www.wallet.com", "wallet://")
		asset := CreateAssetFixture(t, ctx, dbConnectionPool, "USDC", "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVV")

		receiver := CreateReceiverFixture(t, ctx, dbConnectionPool, &Receiver{})
		receiverWallet := CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver.ID, wallet.ID, RegisteredReceiversWalletStatus)

		disbursement := CreateDisbursementFixture(t, ctx, dbConnectionPool, models.Disbursements, &Disbursement{
			Country:           country,
			Wallet:            wallet,
			Asset:             asset,
			Status:            StartedDisbursementStatus,
			VerificationField: VerificationFieldDateOfBirth,
		})

		_ = CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &Payment{
			Amount:               "1",
			StellarTransactionID: "stellar-transaction-id-1",
			StellarOperationID:   "operation-id-1",
			Status:               PendingPaymentStatus,
			Disbursement:         disbursement,
			ReceiverWallet:       receiverWallet,
			Asset:                *asset,
		})

		payments, err := models.Payment.GetAllReadyToPatchCompletionAnchorTransactions(ctx, dbConnectionPool)
		require.NoError(t, err)
		assert.Empty(t, payments)
	})

	t.Run("gets only payments in the Success or Failed statuses", func(t *testing.T) {
		DeleteAllFixtures(t, ctx, dbConnectionPool)

		country := CreateCountryFixture(t, ctx, dbConnectionPool, "BRA", "Brazil")
		wallet := CreateWalletFixture(t, ctx, dbConnectionPool, "Wallet", "https://www.wallet.com", "www.wallet.com", "wallet://")
		asset := CreateAssetFixture(t, ctx, dbConnectionPool, "USDC", "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVV")

		receiver1 := CreateReceiverFixture(t, ctx, dbConnectionPool, &Receiver{})
		receiverWallet1 := CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver1.ID, wallet.ID, RegisteredReceiversWalletStatus)
		receiver2 := CreateReceiverFixture(t, ctx, dbConnectionPool, &Receiver{})
		receiverWallet2 := CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver2.ID, wallet.ID, RegisteredReceiversWalletStatus)

		disbursement := CreateDisbursementFixture(t, ctx, dbConnectionPool, models.Disbursements, &Disbursement{
			Country:           country,
			Wallet:            wallet,
			Asset:             asset,
			Status:            StartedDisbursementStatus,
			VerificationField: VerificationFieldDateOfBirth,
		})

		paymentReceiver1 := CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &Payment{
			Amount:               "1",
			StellarTransactionID: "stellar-transaction-id-1",
			StellarOperationID:   "operation-id-1",
			Status:               SuccessPaymentStatus,
			Disbursement:         disbursement,
			ReceiverWallet:       receiverWallet1,
			Asset:                *asset,
		})

		paymentReceiver2 := CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &Payment{
			Amount:               "1",
			StellarTransactionID: "stellar-transaction-id-2",
			StellarOperationID:   "operation-id-2",
			Status:               FailedPaymentStatus,
			Disbursement:         disbursement,
			ReceiverWallet:       receiverWallet2,
			Asset:                *asset,
		})

		payments, err := models.Payment.GetAllReadyToPatchCompletionAnchorTransactions(ctx, dbConnectionPool)
		require.NoError(t, err)
		require.Len(t, payments, 2)

		assert.Equal(t, paymentReceiver1.ID, payments[0].ID)
		assert.Equal(t, paymentReceiver1.Status, payments[0].Status)
		assert.Equal(t, receiverWallet1.AnchorPlatformTransactionID, payments[0].ReceiverWallet.AnchorPlatformTransactionID)

		assert.Equal(t, paymentReceiver2.ID, payments[1].ID)
		assert.Equal(t, paymentReceiver2.Status, payments[1].Status)
		assert.Equal(t, receiverWallet2.AnchorPlatformTransactionID, payments[1].ReceiverWallet.AnchorPlatformTransactionID)
	})

	t.Run("gets more than one payment when a receiver has payments in the Success or Failed statuses for the same wallet provider", func(t *testing.T) {
		DeleteAllFixtures(t, ctx, dbConnectionPool)

		country := CreateCountryFixture(t, ctx, dbConnectionPool, "BRA", "Brazil")
		wallet := CreateWalletFixture(t, ctx, dbConnectionPool, "Wallet", "https://www.wallet.com", "www.wallet.com", "wallet://")
		asset := CreateAssetFixture(t, ctx, dbConnectionPool, "USDC", "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVV")

		receiver := CreateReceiverFixture(t, ctx, dbConnectionPool, &Receiver{})
		receiverWallet := CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver.ID, wallet.ID, RegisteredReceiversWalletStatus)

		disbursement1 := CreateDisbursementFixture(t, ctx, dbConnectionPool, models.Disbursements, &Disbursement{
			Country:           country,
			Wallet:            wallet,
			Asset:             asset,
			Status:            StartedDisbursementStatus,
			VerificationField: VerificationFieldDateOfBirth,
		})

		disbursement2 := CreateDisbursementFixture(t, ctx, dbConnectionPool, models.Disbursements, &Disbursement{
			Country:           country,
			Wallet:            wallet,
			Asset:             asset,
			Status:            StartedDisbursementStatus,
			VerificationField: VerificationFieldDateOfBirth,
		})

		payment1 := CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &Payment{
			Amount:               "1",
			StellarTransactionID: "stellar-transaction-id-1",
			StellarOperationID:   "operation-id-1",
			Status:               SuccessPaymentStatus,
			Disbursement:         disbursement1,
			ReceiverWallet:       receiverWallet,
			Asset:                *asset,
		})

		payment2 := CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &Payment{
			Amount:               "1",
			StellarTransactionID: "stellar-transaction-id-2",
			StellarOperationID:   "operation-id-2",
			Status:               FailedPaymentStatus,
			Disbursement:         disbursement2,
			ReceiverWallet:       receiverWallet,
			Asset:                *asset,
		})

		payments, err := models.Payment.GetAllReadyToPatchCompletionAnchorTransactions(ctx, dbConnectionPool)
		require.NoError(t, err)
		require.Len(t, payments, 2)

		assert.Equal(t, payment1.ID, payments[0].ID)
		assert.Equal(t, payment1.Status, payments[0].Status)
		assert.Equal(t, receiverWallet.AnchorPlatformTransactionID, payments[0].ReceiverWallet.AnchorPlatformTransactionID)

		assert.Equal(t, payment2.ID, payments[1].ID)
		assert.Equal(t, payment2.Status, payments[1].Status)
		assert.Equal(t, receiverWallet.AnchorPlatformTransactionID, payments[1].ReceiverWallet.AnchorPlatformTransactionID)
	})

	t.Run("gets more than one payment when a receiver has payments for more than one wallet provider", func(t *testing.T) {
		DeleteAllFixtures(t, ctx, dbConnectionPool)

		country := CreateCountryFixture(t, ctx, dbConnectionPool, "BRA", "Brazil")
		wallet1 := CreateWalletFixture(t, ctx, dbConnectionPool, "Wallet1", "https://www.wallet1.com", "www.wallet1.com", "wallet1://")
		wallet2 := CreateWalletFixture(t, ctx, dbConnectionPool, "Wallet2", "https://www.wallet2.com", "www.wallet2.com", "wallet2://")
		asset := CreateAssetFixture(t, ctx, dbConnectionPool, "USDC", "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVV")

		receiver := CreateReceiverFixture(t, ctx, dbConnectionPool, &Receiver{})
		receiverWallet1 := CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver.ID, wallet1.ID, RegisteredReceiversWalletStatus)
		receiverWallet2 := CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver.ID, wallet2.ID, RegisteredReceiversWalletStatus)

		disbursement1 := CreateDisbursementFixture(t, ctx, dbConnectionPool, models.Disbursements, &Disbursement{
			Country:           country,
			Wallet:            wallet1,
			Asset:             asset,
			Status:            StartedDisbursementStatus,
			VerificationField: VerificationFieldDateOfBirth,
		})

		disbursement2 := CreateDisbursementFixture(t, ctx, dbConnectionPool, models.Disbursements, &Disbursement{
			Country:           country,
			Wallet:            wallet2,
			Asset:             asset,
			Status:            StartedDisbursementStatus,
			VerificationField: VerificationFieldDateOfBirth,
		})

		payment1 := CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &Payment{
			Amount:               "1",
			StellarTransactionID: "stellar-transaction-id-1",
			StellarOperationID:   "operation-id-1",
			Status:               SuccessPaymentStatus,
			Disbursement:         disbursement1,
			ReceiverWallet:       receiverWallet1,
			Asset:                *asset,
		})

		payment2 := CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &Payment{
			Amount:               "1",
			StellarTransactionID: "stellar-transaction-id-2",
			StellarOperationID:   "operation-id-2",
			Status:               FailedPaymentStatus,
			Disbursement:         disbursement1,
			ReceiverWallet:       receiverWallet1,
			Asset:                *asset,
		})

		payment3 := CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &Payment{
			Amount:               "1",
			StellarTransactionID: "stellar-transaction-id-3",
			StellarOperationID:   "operation-id-3",
			Status:               FailedPaymentStatus,
			Disbursement:         disbursement2,
			ReceiverWallet:       receiverWallet2,
			Asset:                *asset,
		})

		payments, err := models.Payment.GetAllReadyToPatchCompletionAnchorTransactions(ctx, dbConnectionPool)
		require.NoError(t, err)
		require.Len(t, payments, 3)

		assert.Equal(t, payment1.ID, payments[0].ID)
		assert.Equal(t, payment1.Status, payments[0].Status)
		assert.Equal(t, receiverWallet1.AnchorPlatformTransactionID, payments[0].ReceiverWallet.AnchorPlatformTransactionID)

		assert.Equal(t, payment2.ID, payments[1].ID)
		assert.Equal(t, payment2.Status, payments[1].Status)
		assert.Equal(t, receiverWallet1.AnchorPlatformTransactionID, payments[1].ReceiverWallet.AnchorPlatformTransactionID)

		assert.Equal(t, payment3.ID, payments[2].ID)
		assert.Equal(t, payment3.Status, payments[2].Status)
		assert.Equal(t, receiverWallet2.AnchorPlatformTransactionID, payments[2].ReceiverWallet.AnchorPlatformTransactionID)
	})

	t.Run("doesn't return error when receiver wallet has the stellar_memo and stellar_memo_type null", func(t *testing.T) {
		DeleteAllFixtures(t, ctx, dbConnectionPool)

		country := CreateCountryFixture(t, ctx, dbConnectionPool, "BRA", "Brazil")
		wallet := CreateWalletFixture(t, ctx, dbConnectionPool, "Wallet1", "https://www.wallet1.com", "www.wallet1.com", "wallet1://")
		asset := CreateAssetFixture(t, ctx, dbConnectionPool, "USDC", "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVV")

		receiver := CreateReceiverFixture(t, ctx, dbConnectionPool, &Receiver{})
		receiverWallet := CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver.ID, wallet.ID, RegisteredReceiversWalletStatus)

		disbursement := CreateDisbursementFixture(t, ctx, dbConnectionPool, models.Disbursements, &Disbursement{
			Country:           country,
			Wallet:            wallet,
			Asset:             asset,
			Status:            StartedDisbursementStatus,
			VerificationField: VerificationFieldDateOfBirth,
		})

		payment := CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &Payment{
			Amount:               "1",
			StellarTransactionID: "stellar-transaction-id-1",
			StellarOperationID:   "operation-id-1",
			Status:               SuccessPaymentStatus,
			Disbursement:         disbursement,
			ReceiverWallet:       receiverWallet,
			Asset:                *asset,
		})

		const q = "UPDATE receiver_wallets SET stellar_memo = NULL, stellar_memo_type = NULL WHERE id = $1"
		_, err := dbConnectionPool.ExecContext(ctx, q, receiverWallet.ID)
		require.NoError(t, err)

		payments, err := models.Payment.GetAllReadyToPatchCompletionAnchorTransactions(ctx, dbConnectionPool)
		require.NoError(t, err)
		require.Len(t, payments, 1)

		assert.Equal(t, payment.ID, payments[0].ID)
		assert.Equal(t, payment.Status, payments[0].Status)
		assert.Equal(t, receiverWallet.AnchorPlatformTransactionID, payments[0].ReceiverWallet.AnchorPlatformTransactionID)
	})
}

func Test_PaymentModelCancelPayment(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()

	models, err := NewModels(dbConnectionPool)
	require.NoError(t, err)

	DeleteAllPaymentsFixtures(t, ctx, dbConnectionPool)
	DeleteAllDisbursementFixtures(t, ctx, dbConnectionPool)
	DeleteAllCountryFixtures(t, ctx, dbConnectionPool)
	DeleteAllAssetFixtures(t, ctx, dbConnectionPool)
	DeleteAllReceiverWalletsFixtures(t, ctx, dbConnectionPool)
	DeleteAllReceiversFixtures(t, ctx, dbConnectionPool)
	DeleteAllWalletFixtures(t, ctx, dbConnectionPool)

	country := CreateCountryFixture(t, ctx, dbConnectionPool, "BRA", "Brazil")
	wallet := CreateWalletFixture(t, ctx, dbConnectionPool, "Wallet", "https://www.wallet.com", "www.wallet.com", "wallet://")
	asset := CreateAssetFixture(t, ctx, dbConnectionPool, "USDC", "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVV")

	receiver := CreateReceiverFixture(t, ctx, dbConnectionPool, &Receiver{})
	receiverWallet := CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver.ID, wallet.ID, ReadyReceiversWalletStatus)

	disbursement := CreateDisbursementFixture(t, ctx, dbConnectionPool, models.Disbursements, &Disbursement{
		Country:           country,
		Wallet:            wallet,
		Asset:             asset,
		Status:            ReadyDisbursementStatus,
		VerificationField: VerificationFieldDateOfBirth,
	})

	t.Run("no ready payment for more than 5 days won't cancel any", func(t *testing.T) {
		getEntries := log.DefaultLogger.StartTest(log.DebugLevel)
		payment1 := CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &Payment{
			Amount:               "1",
			StellarTransactionID: "stellar-transaction-id-1",
			StellarOperationID:   "operation-id-1",
			Status:               DraftPaymentStatus,
			Disbursement:         disbursement,
			ReceiverWallet:       receiverWallet,
			Asset:                *asset,
			StatusHistory: []PaymentStatusHistoryEntry{
				{
					Status:        DraftPaymentStatus,
					StatusMessage: "",
					Timestamp:     time.Now().AddDate(0, 0, -6),
				},
			},
		})

		payment2 := CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &Payment{
			Amount:               "1",
			StellarTransactionID: "stellar-transaction-id-1",
			StellarOperationID:   "operation-id-1",
			Status:               ReadyPaymentStatus,
			Disbursement:         disbursement,
			ReceiverWallet:       receiverWallet,
			Asset:                *asset,
			StatusHistory: []PaymentStatusHistoryEntry{
				{
					Status:        ReadyPaymentStatus,
					StatusMessage: "",
					Timestamp:     time.Now(),
				},
			},
		})

		err := models.Payment.CancelPaymentsWithinPeriodDays(ctx, dbConnectionPool, 5)
		require.NoError(t, err)

		entries := getEntries()
		require.Len(t, entries, 1)
		assert.Equal(
			t,
			"No payments were canceled",
			entries[0].Message,
		)

		payment1DB, err := models.Payment.Get(ctx, payment1.ID, dbConnectionPool)
		require.NoError(t, err)

		payment2DB, err := models.Payment.Get(ctx, payment2.ID, dbConnectionPool)
		require.NoError(t, err)

		assert.Equal(t, DraftPaymentStatus, payment1DB.Status)
		assert.Equal(t, ReadyPaymentStatus, payment2DB.Status)
	})

	t.Run("successfully cancel payments when it has multiple ready status history entries", func(t *testing.T) {
		payment1 := CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &Payment{
			Amount:               "1",
			StellarTransactionID: "stellar-transaction-id-1",
			StellarOperationID:   "operation-id-1",
			Status:               ReadyPaymentStatus,
			Disbursement:         disbursement,
			ReceiverWallet:       receiverWallet,
			Asset:                *asset,
			StatusHistory: []PaymentStatusHistoryEntry{
				{
					Status:        DraftPaymentStatus,
					StatusMessage: "",
					Timestamp:     time.Now().AddDate(0, 0, -7),
				},
				{
					Status:        ReadyPaymentStatus,
					StatusMessage: "",
					Timestamp:     time.Now().AddDate(0, 0, -7),
				},
				{
					Status:        PendingPaymentStatus,
					StatusMessage: "",
					Timestamp:     time.Now().AddDate(0, 0, -5),
				},
				{
					Status:        FailedPaymentStatus,
					StatusMessage: "",
					Timestamp:     time.Now().AddDate(0, 0, -5),
				},
				{
					Status:        ReadyPaymentStatus,
					StatusMessage: "",
					Timestamp:     time.Now(),
				},
			},
		})

		payment2 := CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &Payment{
			Amount:               "1",
			StellarTransactionID: "stellar-transaction-id-1",
			StellarOperationID:   "operation-id-1",
			Status:               ReadyPaymentStatus,
			Disbursement:         disbursement,
			ReceiverWallet:       receiverWallet,
			Asset:                *asset,
			StatusHistory: []PaymentStatusHistoryEntry{
				{
					Status:        DraftPaymentStatus,
					StatusMessage: "",
					Timestamp:     time.Now().AddDate(0, 0, -7),
				},
				{
					Status:        ReadyPaymentStatus,
					StatusMessage: "",
					Timestamp:     time.Now().AddDate(0, 0, -7),
				},
				{
					Status:        PendingPaymentStatus,
					StatusMessage: "",
					Timestamp:     time.Now().AddDate(0, 0, -5),
				},
				{
					Status:        FailedPaymentStatus,
					StatusMessage: "",
					Timestamp:     time.Now().AddDate(0, 0, -3),
				},
				{
					Status:        ReadyPaymentStatus,
					StatusMessage: "",
					Timestamp:     time.Now().AddDate(0, 0, -3),
				},
			},
		})

		payment3 := CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &Payment{
			Amount:               "1",
			StellarTransactionID: "stellar-transaction-id-1",
			StellarOperationID:   "operation-id-1",
			Status:               SuccessPaymentStatus,
			Disbursement:         disbursement,
			ReceiverWallet:       receiverWallet,
			Asset:                *asset,
			StatusHistory: []PaymentStatusHistoryEntry{
				{
					Status:        DraftPaymentStatus,
					StatusMessage: "",
					Timestamp:     time.Now().AddDate(0, 0, -7),
				},
				{
					Status:        ReadyPaymentStatus,
					StatusMessage: "",
					Timestamp:     time.Now().AddDate(0, 0, -7),
				},
				{
					Status:        PendingPaymentStatus,
					StatusMessage: "",
					Timestamp:     time.Now().AddDate(0, 0, -7),
				},
				{
					Status:        SuccessPaymentStatus,
					StatusMessage: "",
					Timestamp:     time.Now().AddDate(0, 0, -7),
				},
			},
		})

		err := models.Payment.CancelPaymentsWithinPeriodDays(ctx, dbConnectionPool, 5)
		require.NoError(t, err)

		payment1DB, err := models.Payment.Get(ctx, payment1.ID, dbConnectionPool)
		require.NoError(t, err)
		payment2DB, err := models.Payment.Get(ctx, payment2.ID, dbConnectionPool)
		require.NoError(t, err)
		payment3DB, err := models.Payment.Get(ctx, payment3.ID, dbConnectionPool)
		require.NoError(t, err)

		assert.Equal(t, ReadyPaymentStatus, payment1DB.Status)
		assert.Equal(t, ReadyPaymentStatus, payment2DB.Status)
		assert.Equal(t, SuccessPaymentStatus, payment3DB.Status)
	})

	t.Run("cancels ready payments for more than 5 days", func(t *testing.T) {
		payment1 := CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &Payment{
			Amount:               "1",
			StellarTransactionID: "stellar-transaction-id-1",
			StellarOperationID:   "operation-id-1",
			Status:               ReadyPaymentStatus,
			Disbursement:         disbursement,
			ReceiverWallet:       receiverWallet,
			Asset:                *asset,
			StatusHistory: []PaymentStatusHistoryEntry{
				{
					Status:        ReadyPaymentStatus,
					StatusMessage: "",
					Timestamp:     time.Now().AddDate(0, 0, -5),
				},
			},
		})

		payment2 := CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &Payment{
			Amount:               "1",
			StellarTransactionID: "stellar-transaction-id-1",
			StellarOperationID:   "operation-id-1",
			Status:               ReadyPaymentStatus,
			Disbursement:         disbursement,
			ReceiverWallet:       receiverWallet,
			Asset:                *asset,
			StatusHistory: []PaymentStatusHistoryEntry{
				{
					Status:        ReadyPaymentStatus,
					StatusMessage: "",
					Timestamp:     time.Now().AddDate(0, 0, -7),
				},
			},
		})

		err := models.Payment.CancelPaymentsWithinPeriodDays(ctx, dbConnectionPool, 5)
		require.NoError(t, err)

		payment1DB, err := models.Payment.Get(ctx, payment1.ID, dbConnectionPool)
		require.NoError(t, err)

		payment2DB, err := models.Payment.Get(ctx, payment2.ID, dbConnectionPool)
		require.NoError(t, err)

		assert.Equal(t, CanceledPaymentStatus, payment1DB.Status)
		assert.Equal(t, CanceledPaymentStatus, payment2DB.Status)
	})
}
