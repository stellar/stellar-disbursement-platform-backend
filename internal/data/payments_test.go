package data

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stellar/go-stellar-sdk/support/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/db/dbtest"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/testutils"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
)

func Test_PaymentsModelGet(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()

	asset := CreateAssetFixture(t, ctx, dbConnectionPool, "USDC", "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVV")
	wallet1 := CreateWalletFixture(t, ctx, dbConnectionPool, "wallet1", "https://www.wallet.com", "www.wallet.com", "wallet1://")

	receiver := CreateReceiverFixture(t, ctx, dbConnectionPool, &Receiver{})
	receiverWallet1 := CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver.ID, wallet1.ID, DraftReceiversWalletStatus)

	disbursementModel := DisbursementModel{dbConnectionPool: dbConnectionPool}
	disbursement1 := CreateDisbursementFixture(t, ctx, dbConnectionPool, &disbursementModel, &Disbursement{
		Name:      "disbursement 1",
		Status:    DraftDisbursementStatus,
		Asset:     asset,
		Wallet:    wallet1,
		CreatedAt: testutils.TimePtr(time.Date(2022, 3, 21, 23, 40, 20, 1431, time.UTC)),
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
			Name:   "disbursement 2",
			Status: DraftDisbursementStatus,
			Asset:  asset,
			Wallet: wallet2,
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
	wallet := CreateWalletFixture(t, ctx, dbConnectionPool, "wallet1", "https://www.wallet.com", "www.wallet.com", "wallet1://")

	receiver := CreateReceiverFixture(t, ctx, dbConnectionPool, &Receiver{})
	receiverWallet := CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver.ID, wallet.ID, DraftReceiversWalletStatus)

	disbursementModel := DisbursementModel{dbConnectionPool: dbConnectionPool}
	disbursement1 := CreateDisbursementFixture(t, ctx, dbConnectionPool, &disbursementModel, &Disbursement{
		Name:   "disbursement 1",
		Status: DraftDisbursementStatus,
		Asset:  asset,
		Wallet: wallet,
	})

	disbursement2 := CreateDisbursementFixture(t, ctx, dbConnectionPool, &disbursementModel, &Disbursement{
		Name:   "disbursement 2",
		Status: DraftDisbursementStatus,
		Asset:  asset,
		Wallet: wallet,
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
	wallet := CreateWalletFixture(t, ctx, dbConnectionPool, "wallet1", "https://www.wallet.com", "www.wallet.com", "wallet1://")

	receiver := CreateReceiverFixture(t, ctx, dbConnectionPool, &Receiver{})
	receiverWallet := CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver.ID, wallet.ID, DraftReceiversWalletStatus)

	disbursementModel := DisbursementModel{dbConnectionPool: dbConnectionPool}
	disbursement1 := CreateDisbursementFixture(t, ctx, dbConnectionPool, &disbursementModel, &Disbursement{
		Name:   "disbursement 1",
		Status: DraftDisbursementStatus,
		Asset:  asset,
		Wallet: wallet,
	})

	disbursement2 := CreateDisbursementFixture(t, ctx, dbConnectionPool, &disbursementModel, &Disbursement{
		Name:   "disbursement 2",
		Status: DraftDisbursementStatus,
		Asset:  asset,
		Wallet: wallet,
	})

	paymentModel := PaymentModel{dbConnectionPool: dbConnectionPool}

	t.Run("returns empty list when no payments exist", func(t *testing.T) {
		payments, errPayment := paymentModel.GetAll(ctx, &QueryParams{}, dbConnectionPool, QueryTypeSelectPaginated)
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
		UpdatedAt:      time.Date(2022, 3, 21, 23, 40, 20, 1431, time.UTC),
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
		UpdatedAt:      time.Date(2023, 3, 21, 23, 40, 20, 1431, time.UTC),
	})

	t.Run("returns payments successfully", func(t *testing.T) {
		params := QueryParams{SortBy: DefaultPaymentSortField, SortOrder: DefaultPaymentSortOrder}
		actualPayments, err := paymentModel.GetAll(ctx, &params, dbConnectionPool, QueryTypeSelectPaginated)
		require.NoError(t, err)
		assert.Equal(t, 2, len(actualPayments))
		assert.Equal(t, []Payment{*expectedPayment2, *expectedPayment1}, actualPayments)
	})

	t.Run("returns payments successfully with limit", func(t *testing.T) {
		params := QueryParams{
			SortBy:    DefaultPaymentSortField,
			SortOrder: DefaultPaymentSortOrder,
			Page:      1,
			PageLimit: 1,
		}
		actualPayments, err := paymentModel.GetAll(ctx, &params, dbConnectionPool, QueryTypeSelectPaginated)
		require.NoError(t, err)
		assert.Equal(t, 1, len(actualPayments))
		assert.Equal(t, []Payment{*expectedPayment2}, actualPayments)
	})

	t.Run("returns payments successfully with offset", func(t *testing.T) {
		params := QueryParams{
			Page:      2,
			PageLimit: 1,
			SortBy:    DefaultPaymentSortField,
			SortOrder: DefaultPaymentSortOrder,
		}
		actualPayments, err := paymentModel.GetAll(ctx, &params, dbConnectionPool, QueryTypeSelectPaginated)
		require.NoError(t, err)
		assert.Equal(t, 1, len(actualPayments))
		assert.Equal(t, []Payment{*expectedPayment1}, actualPayments)
	})

	t.Run("returns payments successfully with created at order", func(t *testing.T) {
		params := &QueryParams{SortBy: SortFieldCreatedAt, SortOrder: SortOrderASC}
		actualPayments, err := paymentModel.GetAll(ctx, params, dbConnectionPool, QueryTypeSelectPaginated)

		require.NoError(t, err)
		assert.Equal(t, 2, len(actualPayments))
		assert.Equal(t, []Payment{*expectedPayment1, *expectedPayment2}, actualPayments)
	})

	t.Run("returns payments successfully with updated at order", func(t *testing.T) {
		params := &QueryParams{SortBy: SortFieldUpdatedAt, SortOrder: SortOrderASC}
		actualPayments, err := paymentModel.GetAll(ctx, params, dbConnectionPool, QueryTypeSelectPaginated)

		require.NoError(t, err)
		assert.Equal(t, 2, len(actualPayments))
		assert.Equal(t, []Payment{*expectedPayment1, *expectedPayment2}, actualPayments)
	})

	t.Run("returns payments successfully with one status filter", func(t *testing.T) {
		filters := map[FilterKey]interface{}{
			FilterKeyStatus: PendingPaymentStatus,
		}
		actualPayments, err := paymentModel.GetAll(ctx, &QueryParams{Filters: filters}, dbConnectionPool, QueryTypeSelectPaginated)
		require.NoError(t, err)
		assert.Equal(t, 1, len(actualPayments))
		assert.Equal(t, []Payment{*expectedPayment2}, actualPayments)
	})

	t.Run("returns payments successfully with list of status filters", func(t *testing.T) {
		filters := map[FilterKey]interface{}{
			FilterKeyStatus: []PaymentStatus{
				DraftPaymentStatus,
				PendingPaymentStatus,
			},
		}
		queryParams := QueryParams{
			Filters:   filters,
			SortBy:    DefaultPaymentSortField,
			SortOrder: DefaultPaymentSortOrder,
		}
		actualPayments, err := paymentModel.GetAll(ctx, &queryParams, dbConnectionPool, QueryTypeSelectPaginated)
		require.NoError(t, err)
		assert.Equal(t, 2, len(actualPayments))
		assert.Equal(t, []Payment{*expectedPayment2, *expectedPayment1}, actualPayments)
	})

	t.Run("should not return duplicated entries when receiver are in more than one disbursements with different wallets", func(t *testing.T) {
		models, err := NewModels(dbConnectionPool)
		require.NoError(t, err)

		DeleteAllPaymentsFixtures(t, ctx, dbConnectionPool)
		DeleteAllDisbursementFixtures(t, ctx, dbConnectionPool)
		DeleteAllAssetFixtures(t, ctx, dbConnectionPool)
		DeleteAllReceiverWalletsFixtures(t, ctx, dbConnectionPool)
		DeleteAllReceiversFixtures(t, ctx, dbConnectionPool)
		DeleteAllWalletFixtures(t, ctx, dbConnectionPool)

		usdc := CreateAssetFixture(t, ctx, dbConnectionPool, "USDC", "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVV")
		demoWallet := CreateWalletFixture(t, ctx, dbConnectionPool, "Demo Wallet", "https://demo-wallet.stellar.org", "https://demo-wallet.stellar.org", "demo-wallet-server.stellar.org")
		vibrantWallet := CreateWalletFixture(t, ctx, dbConnectionPool, "Vibrant Assist", "https://vibrantapp.com", "api-dev.vibrantapp.com", "https://vibrantapp.com/sdp-dev")

		receiver := CreateReceiverFixture(t, ctx, dbConnectionPool, &Receiver{})
		receiverDemoWallet := CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver.ID, demoWallet.ID, ReadyReceiversWalletStatus)
		receiverVibrantWallet := CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver.ID, vibrantWallet.ID, ReadyReceiversWalletStatus)

		disbursement1 := CreateDisbursementFixture(t, ctx, dbConnectionPool, models.Disbursements, &Disbursement{
			Name:   "disbursement 1",
			Status: ReadyDisbursementStatus,
			Asset:  usdc,
			Wallet: demoWallet,
		})

		disbursement2 := CreateDisbursementFixture(t, ctx, dbConnectionPool, models.Disbursements, &Disbursement{
			Name:   "disbursement 2",
			Status: ReadyDisbursementStatus,
			Asset:  usdc,
			Wallet: vibrantWallet,
		})

		demoWalletPayment := CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &Payment{
			Amount:         "100",
			Status:         ReadyPaymentStatus,
			Disbursement:   disbursement1,
			Asset:          *usdc,
			ReceiverWallet: receiverDemoWallet,
			UpdatedAt:      time.Date(2023, 3, 21, 23, 40, 20, 1431, time.UTC),
		})

		vibrantWalletPayment := CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &Payment{
			Amount:         "100",
			Status:         ReadyPaymentStatus,
			Disbursement:   disbursement2,
			Asset:          *usdc,
			ReceiverWallet: receiverVibrantWallet,
			UpdatedAt:      time.Date(2022, 3, 21, 23, 40, 20, 1431, time.UTC),
		})

		payments, err := models.Payment.GetAll(ctx, &QueryParams{
			Filters: map[FilterKey]interface{}{
				FilterKeyReceiverID: receiver.ID,
			},
			SortBy:    DefaultPaymentSortField,
			SortOrder: DefaultPaymentSortOrder,
		}, dbConnectionPool, QueryTypeSelectPaginated)
		require.NoError(t, err)

		assert.Len(t, payments, 2)
		assert.Equal(t, []Payment{
			*demoWalletPayment,
			*vibrantWalletPayment,
		}, payments)
	})
}

func Test_PaymentModel_GetByIDs(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()

	asset := CreateAssetFixture(t, ctx, dbConnectionPool, "USDC", "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVV")
	wallet1 := CreateWalletFixture(t, ctx, dbConnectionPool, "wallet1", "https://www.wallet.com", "www.wallet.com", "wallet1://")

	receiver1 := CreateReceiverFixture(t, ctx, dbConnectionPool, &Receiver{})
	receiverWallet1 := CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver1.ID, wallet1.ID, DraftReceiversWalletStatus)

	receiver2 := CreateReceiverFixture(t, ctx, dbConnectionPool, &Receiver{})
	receiverWallet2 := CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver2.ID, wallet1.ID, DraftReceiversWalletStatus)

	disbursementModel := DisbursementModel{dbConnectionPool: dbConnectionPool}
	d := time.Date(2022, 3, 21, 23, 40, 20, 1431, time.UTC)
	disbursement1 := CreateDisbursementFixture(t, ctx, dbConnectionPool, &disbursementModel, &Disbursement{
		Name:      "disbursement 1",
		Status:    DraftDisbursementStatus,
		Asset:     asset,
		Wallet:    wallet1,
		CreatedAt: &d,
	})

	paymentModel := PaymentModel{dbConnectionPool: dbConnectionPool}

	stellarTransactionID, err := utils.RandomString(64)
	require.NoError(t, err)
	stellarOperationID, err := utils.RandomString(32)
	require.NoError(t, err)
	payment1 := CreatePaymentFixture(t, ctx, dbConnectionPool, &paymentModel, &Payment{
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

	stellarTransactionID, err = utils.RandomString(64)
	require.NoError(t, err)
	stellarOperationID, err = utils.RandomString(32)
	require.NoError(t, err)
	payment2 := CreatePaymentFixture(t, ctx, dbConnectionPool, &paymentModel, &Payment{
		Amount:               "150",
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
		ReceiverWallet: receiverWallet2,
	})

	t.Run("returns empty list when payments ids are not found", func(t *testing.T) {
		payments, err := paymentModel.GetByIDs(ctx, dbConnectionPool, []string{"invalid_id"})
		require.NoError(t, err)
		require.Empty(t, payments)
	})

	t.Run("returns subset of payments successfully", func(t *testing.T) {
		payments, err := paymentModel.GetByIDs(ctx, dbConnectionPool, []string{payment1.ID})
		require.NoError(t, err)
		require.Len(t, payments, 1)
		assert.Equal(t, *payment1, payments[0])
	})

	t.Run("returns all payments successfully", func(t *testing.T) {
		payments, err := paymentModel.GetByIDs(ctx, dbConnectionPool, []string{payment1.ID, payment2.ID})
		require.NoError(t, err)
		require.Len(t, payments, 2)
		assert.Equal(t, *payment1, payments[0])
		assert.Equal(t, *payment2, payments[1])
	})
}

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
		queryType      QueryType
		expectedQuery  string
		expectedParams []any
	}{
		{
			name:           "build payment query without params and pagination",
			baseQuery:      "SELECT * FROM payments p",
			queryParams:    QueryParams{},
			queryType:      QueryTypeSelectAll,
			expectedQuery:  "SELECT * FROM payments p",
			expectedParams: []any{},
		},
		{
			name:      "build payment query with a query search",
			baseQuery: "SELECT * FROM payments p",
			queryParams: QueryParams{
				Query: "foo-bar",
			},
			queryType:      QueryTypeSelectAll,
			expectedQuery:  "SELECT * FROM payments p WHERE 1=1 AND (p.id ILIKE $1 OR p.external_payment_id ILIKE $2 OR rw.stellar_address ILIKE $3 OR COALESCE(d.name, '') ILIKE $4)",
			expectedParams: []any{"%foo-bar%", "%foo-bar%", "%foo-bar%", "%foo-bar%"},
		},
		{
			name:      "build payment query with status filter",
			baseQuery: "SELECT * FROM payments p",
			queryParams: QueryParams{
				Filters: map[FilterKey]any{
					FilterKeyStatus: "draft",
				},
			},
			queryType:      QueryTypeSelectAll,
			expectedQuery:  "SELECT * FROM payments p WHERE 1=1 AND p.status = $1",
			expectedParams: []any{"draft"},
		},
		{
			name:      "build payment query with receiver_id filter",
			baseQuery: "SELECT * FROM payments p",
			queryParams: QueryParams{
				Filters: map[FilterKey]any{
					FilterKeyReceiverID: "receiver_id",
				},
			},
			queryType:      QueryTypeSelectAll,
			expectedQuery:  "SELECT * FROM payments p WHERE 1=1 AND p.receiver_id = $1",
			expectedParams: []any{"receiver_id"},
		},
		{
			name:      "build payment query with created_at filters",
			baseQuery: "SELECT * FROM payments p",
			queryParams: QueryParams{
				Filters: map[FilterKey]any{
					FilterKeyCreatedAtAfter:  "00-01-01",
					FilterKeyCreatedAtBefore: "00-01-31",
				},
			},
			queryType:      QueryTypeSelectAll,
			expectedQuery:  "SELECT * FROM payments p WHERE 1=1 AND p.created_at >= $1 AND p.created_at <= $2",
			expectedParams: []any{"00-01-01", "00-01-31"},
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
			queryType:      QueryTypeSelectPaginated,
			expectedQuery:  "SELECT * FROM payments p ORDER BY p.created_at ASC LIMIT $1 OFFSET $2",
			expectedParams: []any{20, 0},
		},
		{
			name:      "build payment query with all filters and pagination",
			baseQuery: "SELECT * FROM payments p",
			queryParams: QueryParams{
				Page:      1,
				PageLimit: 20,
				SortBy:    "created_at",
				SortOrder: "ASC",
				Filters: map[FilterKey]any{
					FilterKeyStatus:          "draft",
					FilterKeyReceiverID:      "receiver_id",
					FilterKeyCreatedAtAfter:  "00-01-01",
					FilterKeyCreatedAtBefore: "00-01-31",
					FilterKeyPaymentType:     "DIRECT",
				},
			},
			queryType:      QueryTypeSelectPaginated,
			expectedQuery:  "SELECT * FROM payments p WHERE 1=1 AND p.status = $1 AND p.receiver_id = $2 AND p.created_at >= $3 AND p.created_at <= $4 AND p.type = $5 ORDER BY p.created_at ASC LIMIT $6 OFFSET $7",
			expectedParams: []any{"draft", "receiver_id", "00-01-01", "00-01-31", "DIRECT", 20, 0},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			query, params := newPaymentQuery(tc.baseQuery, &tc.queryParams, dbConnectionPool, tc.queryType)

			assert.Equal(t, tc.expectedQuery, query)
			assert.Equal(t, tc.expectedParams, params)
		})
	}
}

func Test_PaymentModelRetryFailedPayments(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, outerErr := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, outerErr)
	defer dbConnectionPool.Close()

	ctx := context.Background()

	models, outerErr := NewModels(dbConnectionPool)
	require.NoError(t, outerErr)

	wallet := CreateWalletFixture(t, ctx, dbConnectionPool, "Wallet", "https://www.wallet.com", "www.wallet.com", "wallet://")
	asset := CreateAssetFixture(t, ctx, dbConnectionPool, "USDC", "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVV")

	receiver := CreateReceiverFixture(t, ctx, dbConnectionPool, &Receiver{})
	receiverWallet := CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver.ID, wallet.ID, ReadyReceiversWalletStatus)

	disbursement := CreateDisbursementFixture(t, ctx, dbConnectionPool, models.Disbursements, &Disbursement{
		Wallet:            wallet,
		Asset:             asset,
		Status:            ReadyDisbursementStatus,
		VerificationField: VerificationTypeDateOfBirth,
	})

	t.Run("does not update payments when no payments IDs is given", func(t *testing.T) {
		err := models.Payment.RetryFailedPayments(ctx, dbConnectionPool, "user@test.com")
		assert.ErrorIs(t, err, ErrMissingInput)
	})

	t.Run("does not update payments when email is empty", func(t *testing.T) {
		err := models.Payment.RetryFailedPayments(ctx, dbConnectionPool, "", "payment-id")
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

		err := models.Payment.RetryFailedPayments(ctx, dbConnectionPool, "user@test.com", payment1.ID, payment2.ID)
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

		err := db.RunInTransaction(ctx, dbConnectionPool, nil, func(dbTx db.DBTransaction) error {
			innerErr := models.Payment.RetryFailedPayments(ctx, dbTx, "user@test.com", payment1.ID, payment2.ID, payment3.ID)
			assert.ErrorIs(t, innerErr, ErrMismatchNumRowsAffected)
			return innerErr
		})
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

		err := models.Payment.RetryFailedPayments(ctx, dbConnectionPool, "user@test.com", payment1.ID, payment2.ID)
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
	DeleteAllAssetFixtures(t, ctx, dbConnectionPool)
	DeleteAllReceiverWalletsFixtures(t, ctx, dbConnectionPool)
	DeleteAllReceiversFixtures(t, ctx, dbConnectionPool)
	DeleteAllWalletFixtures(t, ctx, dbConnectionPool)

	wallet := CreateWalletFixture(t, ctx, dbConnectionPool, "Wallet", "https://www.wallet.com", "www.wallet.com", "wallet://")
	asset := CreateAssetFixture(t, ctx, dbConnectionPool, "USDC", "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVV")

	receiver := CreateReceiverFixture(t, ctx, dbConnectionPool, &Receiver{})
	receiverWallet := CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver.ID, wallet.ID, ReadyReceiversWalletStatus)

	disbursement := CreateDisbursementFixture(t, ctx, dbConnectionPool, models.Disbursements, &Disbursement{
		Wallet:            wallet,
		Asset:             asset,
		Status:            ReadyDisbursementStatus,
		VerificationField: VerificationTypeDateOfBirth,
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

func Test_PaymentModel_GetReadyByDisbursementID(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()

	models, err := NewModels(dbConnectionPool)
	require.NoError(t, err)

	wallet := CreateWalletFixture(t, ctx, dbConnectionPool, "Wallet", "https://www.wallet.com", "www.wallet.com", "wallet://")
	asset := CreateAssetFixture(t, ctx, dbConnectionPool, "USDC", "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVV")

	receiver1 := CreateReceiverFixture(t, ctx, dbConnectionPool, &Receiver{})
	rw1 := CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver1.ID, wallet.ID, RegisteredReceiversWalletStatus)

	receiver2 := CreateReceiverFixture(t, ctx, dbConnectionPool, &Receiver{})
	rw2 := CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver2.ID, wallet.ID, ReadyReceiversWalletStatus)

	disbursement := CreateDisbursementFixture(t, ctx, dbConnectionPool, models.Disbursements, &Disbursement{
		Wallet:            wallet,
		Asset:             asset,
		Status:            StartedDisbursementStatus,
		VerificationField: VerificationTypeDateOfBirth,
	})

	t.Run("returns empty array when there's no payment ready", func(t *testing.T) {
		payments, err := models.Payment.GetReadyByDisbursementID(ctx, dbConnectionPool, disbursement.ID)
		require.NoError(t, err)
		assert.Empty(t, payments)
	})

	t.Run("return only payments ready to be paid from a REGISTERED wallet", func(t *testing.T) {
		payment1 := CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &Payment{
			Amount:         "2",
			Disbursement:   disbursement,
			Asset:          *asset,
			ReceiverWallet: rw1, // REGISTERED status, will be returned in the query
			Status:         ReadyPaymentStatus,
		})

		_ = CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &Payment{
			Amount:         "2",
			Disbursement:   disbursement,
			Asset:          *asset,
			ReceiverWallet: rw2, // READY status, will NOT be returned in the query
			Status:         ReadyPaymentStatus,
		})

		payments, err := models.Payment.GetReadyByDisbursementID(ctx, dbConnectionPool, disbursement.ID)
		require.NoError(t, err)
		require.Len(t, payments, 1)

		assert.Equal(t, payment1.ID, payments[0].ID)
		assert.Equal(t, payment1.Amount, payments[0].Amount)
		assert.Equal(t, payment1.Status, payments[0].Status)
		assert.Equal(t, payment1.Disbursement.ID, payments[0].Disbursement.ID)
		assert.Equal(t, payment1.ReceiverWallet.ID, payments[0].ReceiverWallet.ID)
	})
}

func Test_PaymentModel_GetReadyByPaymentsID(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()

	models, err := NewModels(dbConnectionPool)
	require.NoError(t, err)

	wallet := CreateWalletFixture(t, ctx, dbConnectionPool, "Wallet", "https://www.wallet.com", "www.wallet.com", "wallet://")
	asset := CreateAssetFixture(t, ctx, dbConnectionPool, "USDC", "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVV")

	receiver1 := CreateReceiverFixture(t, ctx, dbConnectionPool, &Receiver{})
	rw1 := CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver1.ID, wallet.ID, RegisteredReceiversWalletStatus)

	receiver2 := CreateReceiverFixture(t, ctx, dbConnectionPool, &Receiver{})
	rw2 := CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver2.ID, wallet.ID, ReadyReceiversWalletStatus)

	disbursement := CreateDisbursementFixture(t, ctx, dbConnectionPool, models.Disbursements, &Disbursement{
		Wallet:            wallet,
		Asset:             asset,
		Status:            StartedDisbursementStatus,
		VerificationField: VerificationTypeDateOfBirth,
	})

	t.Run("returns empty array when there's no payment ready", func(t *testing.T) {
		payment := CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &Payment{
			Amount:         "2",
			Disbursement:   disbursement,
			Asset:          *asset,
			ReceiverWallet: rw1,
			Status:         DraftPaymentStatus,
		})

		payments, err := models.Payment.GetReadyByID(ctx, dbConnectionPool, payment.ID)
		require.NoError(t, err)
		assert.Empty(t, payments)
	})

	t.Run("return only payments ready to be paid from a REGISTERED wallet", func(t *testing.T) {
		payment1 := CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &Payment{
			Amount:         "2",
			Disbursement:   disbursement,
			Asset:          *asset,
			ReceiverWallet: rw1, // REGISTERED status, will be returned in the query
			Status:         ReadyPaymentStatus,
		})

		payment2 := CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &Payment{
			Amount:         "2",
			Disbursement:   disbursement,
			Asset:          *asset,
			ReceiverWallet: rw2, // READY status, will NOT be returned in the query
			Status:         ReadyPaymentStatus,
		})

		payments, err := models.Payment.GetReadyByID(ctx, dbConnectionPool, payment1.ID, payment2.ID)
		require.NoError(t, err)
		require.Len(t, payments, 1)

		assert.Equal(t, payment1.ID, payments[0].ID)
		assert.Equal(t, payment1.Amount, payments[0].Amount)
		assert.Equal(t, payment1.Status, payments[0].Status)
		assert.Equal(t, payment1.Disbursement.ID, payments[0].Disbursement.ID)
		assert.Equal(t, payment1.ReceiverWallet.ID, payments[0].ReceiverWallet.ID)
	})
}

func Test_PaymentModel_GetReadyByReceiverWalletID(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()

	models, err := NewModels(dbConnectionPool)
	require.NoError(t, err)

	wallet := CreateWalletFixture(t, ctx, dbConnectionPool, "Wallet", "https://www.wallet.com", "www.wallet.com", "wallet://")
	asset := CreateAssetFixture(t, ctx, dbConnectionPool, "USDC", "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVV")

	receiver1 := CreateReceiverFixture(t, ctx, dbConnectionPool, &Receiver{})
	rw1 := CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver1.ID, wallet.ID, RegisteredReceiversWalletStatus)

	receiver2 := CreateReceiverFixture(t, ctx, dbConnectionPool, &Receiver{})
	rw2 := CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver2.ID, wallet.ID, ReadyReceiversWalletStatus)

	disbursement := CreateDisbursementFixture(t, ctx, dbConnectionPool, models.Disbursements, &Disbursement{
		Wallet:            wallet,
		Asset:             asset,
		Status:            StartedDisbursementStatus,
		VerificationField: VerificationTypeDateOfBirth,
	})

	t.Run("returns empty array when there's no payment ready", func(t *testing.T) {
		_ = CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &Payment{
			Amount:         "2",
			Disbursement:   disbursement,
			Asset:          *asset,
			ReceiverWallet: rw1,
			Status:         DraftPaymentStatus,
		})

		payments, err := models.Payment.GetReadyByReceiverWalletID(ctx, dbConnectionPool, rw1.ID)
		require.NoError(t, err)
		assert.Empty(t, payments)
	})

	t.Run("return only payments ready to be paid from a REGISTERED wallet", func(t *testing.T) {
		payment1 := CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &Payment{
			Amount:         "2",
			Disbursement:   disbursement,
			Asset:          *asset,
			ReceiverWallet: rw1, // REGISTERED status, will be returned in the query
			Status:         ReadyPaymentStatus,
		})

		_ = CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &Payment{
			Amount:         "2",
			Disbursement:   disbursement,
			Asset:          *asset,
			ReceiverWallet: rw2, // READY status, will NOT be returned in the query
			Status:         ReadyPaymentStatus,
		})

		payments, err := models.Payment.GetReadyByReceiverWalletID(ctx, dbConnectionPool, rw1.ID)
		require.NoError(t, err)
		require.Len(t, payments, 1)

		assert.Equal(t, payment1.ID, payments[0].ID)
		assert.Equal(t, payment1.Amount, payments[0].Amount)
		assert.Equal(t, payment1.Status, payments[0].Status)
		assert.Equal(t, payment1.Disbursement.ID, payments[0].Disbursement.ID)
		assert.Equal(t, payment1.ReceiverWallet.ID, payments[0].ReceiverWallet.ID)

		payments, err = models.Payment.GetReadyByReceiverWalletID(ctx, dbConnectionPool, rw2.ID)
		require.NoError(t, err)
		assert.Empty(t, payments)
	})
}

func Test_PaymentModel_GetAllReadyToPatchCompletionAnchorTransactions(t *testing.T) {
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

		wallet := CreateWalletFixture(t, ctx, dbConnectionPool, "Wallet", "https://www.wallet.com", "www.wallet.com", "wallet://")
		asset := CreateAssetFixture(t, ctx, dbConnectionPool, "USDC", "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVV")

		receiver := CreateReceiverFixture(t, ctx, dbConnectionPool, &Receiver{})
		receiverWallet := CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver.ID, wallet.ID, ReadyReceiversWalletStatus)

		disbursement := CreateDisbursementFixture(t, ctx, dbConnectionPool, models.Disbursements, &Disbursement{
			Wallet:            wallet,
			Asset:             asset,
			Status:            StartedDisbursementStatus,
			VerificationField: VerificationTypeDateOfBirth,
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

		wallet := CreateWalletFixture(t, ctx, dbConnectionPool, "Wallet", "https://www.wallet.com", "www.wallet.com", "wallet://")
		asset := CreateAssetFixture(t, ctx, dbConnectionPool, "USDC", "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVV")

		receiver := CreateReceiverFixture(t, ctx, dbConnectionPool, &Receiver{})
		receiverWallet := CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver.ID, wallet.ID, RegisteredReceiversWalletStatus)

		disbursement := CreateDisbursementFixture(t, ctx, dbConnectionPool, models.Disbursements, &Disbursement{
			Wallet:            wallet,
			Asset:             asset,
			Status:            StartedDisbursementStatus,
			VerificationField: VerificationTypeDateOfBirth,
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

		wallet := CreateWalletFixture(t, ctx, dbConnectionPool, "Wallet", "https://www.wallet.com", "www.wallet.com", "wallet://")
		asset := CreateAssetFixture(t, ctx, dbConnectionPool, "USDC", "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVV")

		receiver1 := CreateReceiverFixture(t, ctx, dbConnectionPool, &Receiver{})
		receiverWallet1 := CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver1.ID, wallet.ID, RegisteredReceiversWalletStatus)
		receiver2 := CreateReceiverFixture(t, ctx, dbConnectionPool, &Receiver{})
		receiverWallet2 := CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver2.ID, wallet.ID, RegisteredReceiversWalletStatus)

		disbursement := CreateDisbursementFixture(t, ctx, dbConnectionPool, models.Disbursements, &Disbursement{
			Wallet:            wallet,
			Asset:             asset,
			Status:            StartedDisbursementStatus,
			VerificationField: VerificationTypeDateOfBirth,
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
		assert.Equal(t, receiverWallet1.SEP24TransactionID, payments[0].ReceiverWallet.SEP24TransactionID)

		assert.Equal(t, paymentReceiver2.ID, payments[1].ID)
		assert.Equal(t, paymentReceiver2.Status, payments[1].Status)
		assert.Equal(t, receiverWallet2.SEP24TransactionID, payments[1].ReceiverWallet.SEP24TransactionID)
	})

	t.Run("gets more than one payment when a receiver has payments in the Success or Failed statuses for the same wallet provider", func(t *testing.T) {
		DeleteAllFixtures(t, ctx, dbConnectionPool)

		wallet := CreateWalletFixture(t, ctx, dbConnectionPool, "Wallet", "https://www.wallet.com", "www.wallet.com", "wallet://")
		asset := CreateAssetFixture(t, ctx, dbConnectionPool, "USDC", "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVV")

		receiver := CreateReceiverFixture(t, ctx, dbConnectionPool, &Receiver{})
		receiverWallet := CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver.ID, wallet.ID, RegisteredReceiversWalletStatus)

		disbursement1 := CreateDisbursementFixture(t, ctx, dbConnectionPool, models.Disbursements, &Disbursement{
			Wallet:            wallet,
			Asset:             asset,
			Status:            StartedDisbursementStatus,
			VerificationField: VerificationTypeDateOfBirth,
		})

		disbursement2 := CreateDisbursementFixture(t, ctx, dbConnectionPool, models.Disbursements, &Disbursement{
			Wallet:            wallet,
			Asset:             asset,
			Status:            StartedDisbursementStatus,
			VerificationField: VerificationTypeDateOfBirth,
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
		assert.Equal(t, receiverWallet.SEP24TransactionID, payments[0].ReceiverWallet.SEP24TransactionID)

		assert.Equal(t, payment2.ID, payments[1].ID)
		assert.Equal(t, payment2.Status, payments[1].Status)
		assert.Equal(t, receiverWallet.SEP24TransactionID, payments[1].ReceiverWallet.SEP24TransactionID)
	})

	t.Run("gets more than one payment when a receiver has payments for more than one wallet provider", func(t *testing.T) {
		DeleteAllFixtures(t, ctx, dbConnectionPool)

		wallet1 := CreateWalletFixture(t, ctx, dbConnectionPool, "Wallet1", "https://www.wallet1.com", "www.wallet1.com", "wallet1://")
		wallet2 := CreateWalletFixture(t, ctx, dbConnectionPool, "Wallet2", "https://www.wallet2.com", "www.wallet2.com", "wallet2://")
		asset := CreateAssetFixture(t, ctx, dbConnectionPool, "USDC", "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVV")

		receiver := CreateReceiverFixture(t, ctx, dbConnectionPool, &Receiver{})
		receiverWallet1 := CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver.ID, wallet1.ID, RegisteredReceiversWalletStatus)
		receiverWallet2 := CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver.ID, wallet2.ID, RegisteredReceiversWalletStatus)

		disbursement1 := CreateDisbursementFixture(t, ctx, dbConnectionPool, models.Disbursements, &Disbursement{
			Wallet:            wallet1,
			Asset:             asset,
			Status:            StartedDisbursementStatus,
			VerificationField: VerificationTypeDateOfBirth,
		})

		disbursement2 := CreateDisbursementFixture(t, ctx, dbConnectionPool, models.Disbursements, &Disbursement{
			Wallet:            wallet2,
			Asset:             asset,
			Status:            StartedDisbursementStatus,
			VerificationField: VerificationTypeDateOfBirth,
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
		assert.Equal(t, receiverWallet1.SEP24TransactionID, payments[0].ReceiverWallet.SEP24TransactionID)

		assert.Equal(t, payment2.ID, payments[1].ID)
		assert.Equal(t, payment2.Status, payments[1].Status)
		assert.Equal(t, receiverWallet1.SEP24TransactionID, payments[1].ReceiverWallet.SEP24TransactionID)

		assert.Equal(t, payment3.ID, payments[2].ID)
		assert.Equal(t, payment3.Status, payments[2].Status)
		assert.Equal(t, receiverWallet2.SEP24TransactionID, payments[2].ReceiverWallet.SEP24TransactionID)
	})

	t.Run("doesn't return error when receiver wallet has the stellar_memo and stellar_memo_type null", func(t *testing.T) {
		DeleteAllFixtures(t, ctx, dbConnectionPool)

		wallet := CreateWalletFixture(t, ctx, dbConnectionPool, "Wallet1", "https://www.wallet1.com", "www.wallet1.com", "wallet1://")
		asset := CreateAssetFixture(t, ctx, dbConnectionPool, "USDC", "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVV")

		receiver := CreateReceiverFixture(t, ctx, dbConnectionPool, &Receiver{})
		receiverWallet := CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver.ID, wallet.ID, RegisteredReceiversWalletStatus)

		disbursement := CreateDisbursementFixture(t, ctx, dbConnectionPool, models.Disbursements, &Disbursement{
			Wallet:            wallet,
			Asset:             asset,
			Status:            StartedDisbursementStatus,
			VerificationField: VerificationTypeDateOfBirth,
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
		assert.Equal(t, receiverWallet.SEP24TransactionID, payments[0].ReceiverWallet.SEP24TransactionID)
	})
}

func Test_PaymentModel_GetBatchForUpdate(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, outerErr := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, outerErr)
	defer dbConnectionPool.Close()

	ctx := context.Background()

	models, outerErr := NewModels(dbConnectionPool)
	require.NoError(t, outerErr)

	// fixtures
	disbursement := CreateDisbursementFixture(t, ctx, dbConnectionPool, models.Disbursements, &Disbursement{Status: StartedDisbursementStatus})
	receiver1 := CreateReceiverFixture(t, ctx, dbConnectionPool, &Receiver{})
	rw1 := CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver1.ID, disbursement.Wallet.ID, RegisteredReceiversWalletStatus)
	paymentReady := CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &Payment{
		Amount:         "1",
		Disbursement:   disbursement,
		Asset:          *disbursement.Asset,
		ReceiverWallet: rw1,
		Status:         ReadyPaymentStatus,
	})
	CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &Payment{
		Amount:         "2",
		Disbursement:   disbursement,
		Asset:          *disbursement.Asset,
		ReceiverWallet: rw1,
		Status:         PendingPaymentStatus,
	})
	CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &Payment{
		Amount:         "3",
		Disbursement:   disbursement,
		Asset:          *disbursement.Asset,
		ReceiverWallet: rw1,
		Status:         FailedPaymentStatus,
	})

	t.Run("returns error for invalid batch size", func(t *testing.T) {
		payments, err := models.Payment.GetBatchForUpdate(ctx, dbConnectionPool, 0)
		assert.EqualError(t, err, "batch size must be greater than 0")
		assert.Nil(t, payments)
	})

	t.Run("returns correct batch of payments", func(t *testing.T) {
		dbTx1, err := dbConnectionPool.BeginTxx(ctx, nil)
		require.NoError(t, err)
		defer func() {
			err = dbTx1.Rollback()
			require.NoError(t, err)
		}()

		batchSize := 2

		payments, err := models.Payment.GetBatchForUpdate(ctx, dbTx1, batchSize)
		require.NoError(t, err)
		assert.Len(t, payments, 1) // Only 1 payment is ready.
		assert.Equal(t, paymentReady.ID, payments[0].ID)

		// check row is locked
		dbTx2, err := dbConnectionPool.BeginTxx(ctx, nil)
		require.NoError(t, err)
		defer func() {
			err = dbTx2.Rollback()
			require.NoError(t, err)
		}()

		_, err = dbTx2.ExecContext(ctx, "SET LOCAL lock_timeout = '1s'")
		require.NoError(t, err)
		_, err = dbTx2.ExecContext(ctx, "UPDATE payments SET status = 'FAILED' WHERE id = $1", paymentReady.ID)
		assert.ErrorContains(t, err, "pq: canceling statement due to lock timeout")
	})
}

func Test_PaymentModel_UpdateStatus(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, outerErr := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, outerErr)
	defer dbConnectionPool.Close()

	ctx := context.Background()

	models, outerErr := NewModels(dbConnectionPool)
	require.NoError(t, outerErr)

	t.Run("return an error if paymentID is empty", func(t *testing.T) {
		err := models.Payment.UpdateStatus(ctx, dbConnectionPool, "", SuccessPaymentStatus, nil, "")
		assert.ErrorContains(t, err, "paymentID is required")
	})

	t.Run("return an error if status is invalid", func(t *testing.T) {
		err := models.Payment.UpdateStatus(ctx, dbConnectionPool, "payment-id", PaymentStatus("INVALID"), nil, "")
		assert.ErrorContains(t, err, "status is invalid")
	})

	t.Run("return an error if payment doesn't exist", func(t *testing.T) {
		err := models.Payment.UpdateStatus(ctx, dbConnectionPool, "payment-id", SuccessPaymentStatus, nil, "")
		assert.ErrorContains(t, err, "payment with ID payment-id was not found")
		assert.ErrorIs(t, err, ErrRecordNotFound)
	})

	t.Run("🎉 successfully updates status", func(t *testing.T) {
		// Create fixtures
		models, err := NewModels(dbConnectionPool)
		require.NoError(t, err)
		asset := CreateAssetFixture(t, ctx, dbConnectionPool, "USDC", "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVV")
		wallet := CreateWalletFixture(t, ctx, dbConnectionPool, "wallet1", "https://www.wallet.com", "www.wallet.com", "wallet1://")
		disbursement := CreateDisbursementFixture(t, ctx, dbConnectionPool, models.Disbursements, &Disbursement{
			Wallet: wallet,
			Status: ReadyDisbursementStatus,
			Asset:  asset,
		})
		receiverReady := CreateReceiverFixture(t, ctx, dbConnectionPool, &Receiver{})
		rwReady := CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiverReady.ID, wallet.ID, ReadyReceiversWalletStatus)
		payment := CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &Payment{
			ReceiverWallet: rwReady,
			Disbursement:   disbursement,
			Asset:          *asset,
			Amount:         "100",
			Status:         DraftPaymentStatus,
		})

		// 1. Update status WITHOUT Stellar trabnsaction ID
		statusMsg := "transfer is in CIRCLE"
		err = models.Payment.UpdateStatus(ctx, dbConnectionPool, payment.ID, PendingPaymentStatus, &statusMsg, "")
		require.NoError(t, err)

		paymentDB, err := models.Payment.Get(ctx, payment.ID, dbConnectionPool)
		require.NoError(t, err)
		assert.Equal(t, PendingPaymentStatus, paymentDB.Status)
		assert.Equal(t, len(payment.StatusHistory)+1, len(paymentDB.StatusHistory), "a new status history should have been created")
		assert.Empty(t, paymentDB.StellarTransactionID)

		// 2. Update status WITH Stellar transaction ID
		stellarTransactionID := "stellar-transaction-id"
		err = models.Payment.UpdateStatus(ctx, dbConnectionPool, payment.ID, SuccessPaymentStatus, &statusMsg, stellarTransactionID)
		require.NoError(t, err)

		paymentDB, err = models.Payment.Get(ctx, payment.ID, dbConnectionPool)
		require.NoError(t, err)
		assert.Equal(t, SuccessPaymentStatus, paymentDB.Status)
		assert.Equal(t, len(payment.StatusHistory)+2, len(paymentDB.StatusHistory), "a new status history should have been created")
		assert.Equal(t, stellarTransactionID, paymentDB.StellarTransactionID)
	})
}

func Test_PaymentColumnNames(t *testing.T) {
	testCases := []struct {
		tableReference string
		resultAlias    string
		expected       string
	}{
		{
			tableReference: "",
			resultAlias:    "",
			expected: strings.Join([]string{
				"id",
				"amount",
				"status",
				"status_history",
				"type",
				"created_at",
				"updated_at",
				`COALESCE(stellar_transaction_id, '') AS "stellar_transaction_id"`,
				`COALESCE(stellar_operation_id, '') AS "stellar_operation_id"`,
				`COALESCE(external_payment_id, '') AS "external_payment_id"`,
				`COALESCE(sender_address, '') AS "sender_address"`,
			}, ",\n"),
		},
		{
			tableReference: "p",
			resultAlias:    "",
			expected: strings.Join([]string{
				"p.id",
				"p.amount",
				"p.status",
				"p.status_history",
				"p.type",
				"p.created_at",
				"p.updated_at",
				`COALESCE(p.stellar_transaction_id, '') AS "stellar_transaction_id"`,
				`COALESCE(p.stellar_operation_id, '') AS "stellar_operation_id"`,
				`COALESCE(p.external_payment_id, '') AS "external_payment_id"`,
				`COALESCE(p.sender_address, '') AS "sender_address"`,
			}, ",\n"),
		},
		{
			tableReference: "p",
			resultAlias:    "payment",
			expected: strings.Join([]string{
				`p.id AS "payment.id"`,
				`p.amount AS "payment.amount"`,
				`p.status AS "payment.status"`,
				`p.status_history AS "payment.status_history"`,
				`p.type AS "payment.type"`,
				`p.created_at AS "payment.created_at"`,
				`p.updated_at AS "payment.updated_at"`,
				`COALESCE(p.stellar_transaction_id, '') AS "payment.stellar_transaction_id"`,
				`COALESCE(p.stellar_operation_id, '') AS "payment.stellar_operation_id"`,
				`COALESCE(p.external_payment_id, '') AS "payment.external_payment_id"`,
				`COALESCE(p.sender_address, '') AS "payment.sender_address"`,
			}, ",\n"),
		},
	}

	for _, tc := range testCases {
		t.Run(testCaseNameForScanText(t, tc.tableReference, tc.resultAlias), func(t *testing.T) {
			actual := PaymentColumnNames(tc.tableReference, tc.resultAlias)
			assert.Equal(t, tc.expected, actual)
		})
	}
}
