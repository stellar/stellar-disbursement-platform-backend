package data

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/db/dbtest"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/testutils"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
)

func Test_CircleTransferRequestModel_Insert(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, outerErr := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, outerErr)
	defer dbConnectionPool.Close()

	ctx := context.Background()

	m := CircleTransferRequestModel{dbConnectionPool: dbConnectionPool}

	t.Run("returns error if paymentID is empty", func(t *testing.T) {
		circleEntry, err := m.Insert(ctx, dbConnectionPool, "")
		require.EqualError(t, err, "paymentID is required")
		require.Nil(t, circleEntry)
	})

	t.Run("🎉 successfully inserts a circle transfer request", func(t *testing.T) {
		paymentID := "payment-id"
		circleEntry, err := m.Insert(ctx, dbConnectionPool, paymentID)
		require.NoError(t, err)
		require.NotNil(t, circleEntry)

		assert.Equal(t, paymentID, circleEntry.PaymentID)
		assert.NotEmpty(t, circleEntry.UpdatedAt)
		assert.NotEmpty(t, circleEntry.CreatedAt)
		assert.Nil(t, circleEntry.CompletedAt)
		assert.NoError(t, uuid.Validate(circleEntry.IdempotencyKey), "idempotency key should be a valid UUID")
	})

	t.Run("database constraint that prevents repeated rows with the same paymentID and status!=failed", func(t *testing.T) {
		paymentID := "payment-id-2"
		circleEntry, err := m.Insert(ctx, dbConnectionPool, paymentID)
		require.NoError(t, err)

		_, err = m.Insert(ctx, dbConnectionPool, paymentID)
		require.Error(t, err)
		require.ErrorContains(t, err, "duplicate key value violates unique constraint")

		// it works again when we update the status of the existing entry to failed
		_, err = m.Update(ctx, dbConnectionPool, circleEntry.IdempotencyKey, CircleTransferRequestUpdate{
			Status:           CircleTransferStatusFailed,
			CircleTransferID: "circle-transfer-id",
			ResponseBody:     []byte(`{"foo":"bar"}`),
			SourceWalletID:   "source-wallet-id",
		})
		require.NoError(t, err)

		_, err = m.Insert(ctx, dbConnectionPool, paymentID)
		require.NoError(t, err)
	})
}

func Test_CircleTransferRequestModel_Update(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, outerErr := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, outerErr)
	defer dbConnectionPool.Close()

	ctx := context.Background()
	m := CircleTransferRequestModel{dbConnectionPool: dbConnectionPool}

	updateRequest := CircleTransferRequestUpdate{
		CircleTransferID: "circle-transfer-id",
		Status:           CircleTransferStatusPending,
		ResponseBody:     []byte(`{"foo":"bar"}`),
		SourceWalletID:   "source-wallet-id",
	}

	t.Run("return an error if the idempotencyKey is empty", func(t *testing.T) {
		circleEntry, err := m.Update(ctx, dbConnectionPool, "", CircleTransferRequestUpdate{})
		require.Error(t, err)
		require.ErrorContains(t, err, "idempotencyKey is required")
		require.Nil(t, circleEntry)
	})

	t.Run("return an error if the circle transfer request does not exist", func(t *testing.T) {
		circleEntry, err := m.Update(ctx, dbConnectionPool, "test-key", updateRequest)
		require.Error(t, err)
		require.ErrorContains(t, err, "circle transfer request with idempotency key test-key not found")
		require.ErrorIs(t, err, ErrRecordNotFound)
		require.Nil(t, circleEntry)
	})

	t.Run("🎉 successfully updates a circle transfer request (completedAt==nil)", func(t *testing.T) {
		paymentID := "payment-id"
		circleEntry, err := m.Insert(ctx, dbConnectionPool, paymentID)
		require.NoError(t, err)

		updatedCircleEntry, err := m.Update(ctx, dbConnectionPool, circleEntry.IdempotencyKey, updateRequest)
		require.NoError(t, err)
		require.NotNil(t, updatedCircleEntry)

		assert.Equal(t, circleEntry.IdempotencyKey, updatedCircleEntry.IdempotencyKey)
		assert.Equal(t, updateRequest.CircleTransferID, *updatedCircleEntry.CircleTransferID)
		assert.Equal(t, updateRequest.Status, *updatedCircleEntry.Status)
		assert.JSONEq(t, string(updateRequest.ResponseBody), string(updatedCircleEntry.ResponseBody))
		assert.Equal(t, updateRequest.SourceWalletID, *updatedCircleEntry.SourceWalletID)
		assert.NotEmpty(t, updatedCircleEntry.UpdatedAt)
		assert.Nil(t, updatedCircleEntry.CompletedAt)
	})

	t.Run("🎉 successfully updates a circle transfer request(completedAt!=nil)", func(t *testing.T) {
		paymentID := "payment-id2"
		circleEntry, err := m.Insert(ctx, dbConnectionPool, paymentID)
		require.NoError(t, err)

		updateRequest2 := updateRequest
		updateRequest2.CompletedAt = utils.TimePtr(time.Now())
		updatedCircleEntry, err := m.Update(ctx, dbConnectionPool, circleEntry.IdempotencyKey, updateRequest2)
		require.NoError(t, err)
		require.NotNil(t, updatedCircleEntry)

		assert.Equal(t, circleEntry.IdempotencyKey, updatedCircleEntry.IdempotencyKey)
		assert.Equal(t, updateRequest2.CircleTransferID, *updatedCircleEntry.CircleTransferID)
		assert.Equal(t, updateRequest2.Status, *updatedCircleEntry.Status)
		assert.JSONEq(t, string(updateRequest2.ResponseBody), string(updatedCircleEntry.ResponseBody))
		assert.Equal(t, updateRequest2.SourceWalletID, *updatedCircleEntry.SourceWalletID)
		assert.NotEmpty(t, updatedCircleEntry.UpdatedAt)
		assert.NotNil(t, updatedCircleEntry.CompletedAt)
	})
}

func Test_CircleTransferRequestModel_Get_and_GetAll(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, outerErr := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, outerErr)
	defer dbConnectionPool.Close()

	ctx := context.Background()
	now := time.Now()
	m := CircleTransferRequestModel{dbConnectionPool: dbConnectionPool}

	circleEntry1, outerErr := m.Insert(ctx, dbConnectionPool, "payment-id-1")
	require.NoError(t, outerErr)
	circleEntry1, outerErr = m.Update(ctx, dbConnectionPool, circleEntry1.IdempotencyKey, CircleTransferRequestUpdate{
		CircleTransferID: "circle-transfer-id-1",
		Status:           CircleTransferStatusSuccess,
		SyncAttempts:     10,
	})
	require.NoError(t, outerErr)
	circleEntry2, outerErr := m.Insert(ctx, dbConnectionPool, "payment-id-2")
	require.NoError(t, outerErr)
	circleEntry2, outerErr = m.Update(ctx, dbConnectionPool, circleEntry2.IdempotencyKey, CircleTransferRequestUpdate{
		CircleTransferID: "circle-transfer-id-2",
		Status:           CircleTransferStatusFailed,
		SyncAttempts:     1,
		CompletedAt:      &now,
	})
	require.NoError(t, outerErr)

	t.Run("Get", func(t *testing.T) {
		testCases := []struct {
			name                    string
			queryParams             QueryParams
			expectedCircleRequestID string
			expectedErrContains     string
		}{
			{
				name:                    "get by paymentID",
				queryParams:             QueryParams{Filters: map[FilterKey]interface{}{FilterKeyPaymentID: "payment-id-1"}},
				expectedCircleRequestID: circleEntry1.IdempotencyKey,
			},
			{
				name:                    "get by status",
				queryParams:             QueryParams{Filters: map[FilterKey]interface{}{FilterKeyStatus: CircleTransferStatusFailed}},
				expectedCircleRequestID: circleEntry2.IdempotencyKey,
			},
			{
				name:                    "get by completed_at IS NULL",
				queryParams:             QueryParams{Filters: map[FilterKey]interface{}{IsNull(FilterKeyCompletedAt): true}},
				expectedCircleRequestID: circleEntry1.IdempotencyKey,
			},
			{
				name:                    "get by sync_attempts < 10",
				queryParams:             QueryParams{Filters: map[FilterKey]interface{}{LowerThan(FilterKeySyncAttempts): 10}},
				expectedCircleRequestID: circleEntry2.IdempotencyKey,
			},
			{
				name:                "return an error if the record is not found",
				queryParams:         QueryParams{Filters: map[FilterKey]interface{}{FilterKeyPaymentID: "payment-id-3"}},
				expectedErrContains: ErrRecordNotFound.Error(),
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				circleEntry, err := m.Get(ctx, dbConnectionPool, tc.queryParams)
				if tc.expectedErrContains != "" {
					require.Error(t, err)
					require.ErrorContains(t, err, tc.expectedErrContains)
					require.Nil(t, circleEntry)
				} else {
					require.NoError(t, err)
					require.NotNil(t, circleEntry)
					require.Equal(t, tc.expectedCircleRequestID, circleEntry.IdempotencyKey)
				}
			})
		}
	})

	t.Run("GetAll", func(t *testing.T) {
		testCases := []struct {
			name                     string
			queryParams              QueryParams
			expectedCircleRequestIDs []string
			expectedErrContains      string
		}{
			{
				name:                     "get by paymentID",
				queryParams:              QueryParams{Filters: map[FilterKey]interface{}{FilterKeyPaymentID: "payment-id-1"}},
				expectedCircleRequestIDs: []string{circleEntry1.IdempotencyKey},
			},
			{
				name:                     "get by status",
				queryParams:              QueryParams{Filters: map[FilterKey]interface{}{FilterKeyStatus: CircleTransferStatusFailed}},
				expectedCircleRequestIDs: []string{circleEntry2.IdempotencyKey},
			},
			{
				name:                     "get by completed_at IS NULL",
				queryParams:              QueryParams{Filters: map[FilterKey]interface{}{IsNull(FilterKeyCompletedAt): true}},
				expectedCircleRequestIDs: []string{circleEntry1.IdempotencyKey},
			},
			{
				name:                     "get by sync_attempts < 10",
				queryParams:              QueryParams{Filters: map[FilterKey]interface{}{LowerThan(FilterKeySyncAttempts): 10}},
				expectedCircleRequestIDs: []string{circleEntry2.IdempotencyKey},
			},
			{
				name:                     "return empty if the record is not found",
				queryParams:              QueryParams{Filters: map[FilterKey]interface{}{FilterKeyPaymentID: "payment-id-3"}},
				expectedCircleRequestIDs: []string{},
			},
			{
				name: "return an error if more than one record is not found",
				queryParams: QueryParams{Filters: map[FilterKey]interface{}{FilterKeyStatus: []CircleTransferStatus{
					CircleTransferStatusSuccess,
					CircleTransferStatusFailed,
				}}},
				expectedCircleRequestIDs: []string{
					circleEntry1.IdempotencyKey,
					circleEntry2.IdempotencyKey,
				},
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				circleEntries, err := m.GetAll(ctx, dbConnectionPool, tc.queryParams)
				if tc.expectedErrContains != "" {
					require.Error(t, err)
					require.ErrorContains(t, err, tc.expectedErrContains)
					require.Nil(t, circleEntries)
				} else {
					require.NoError(t, err)
					require.Len(t, circleEntries, len(tc.expectedCircleRequestIDs))
					gotIDs := make([]string, len(circleEntries))
					for i, circleEntry := range circleEntries {
						gotIDs[i] = circleEntry.IdempotencyKey
					}
					require.ElementsMatch(t, tc.expectedCircleRequestIDs, gotIDs)
				}
			})
		}
	})
}

func Test_CircleTransferRequestModel_GetIncompleteByPaymentID(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, outerErr := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, outerErr)
	defer dbConnectionPool.Close()

	ctx := context.Background()
	m := CircleTransferRequestModel{dbConnectionPool: dbConnectionPool}

	t.Run("return nil if no circle transfer request is found", func(t *testing.T) {
		circleEntry, err := m.GetIncompleteByPaymentID(ctx, dbConnectionPool, "payment-id")
		require.ErrorIs(t, err, ErrRecordNotFound)
		require.Nil(t, circleEntry)
	})

	t.Run("return nil if the existing circle transfer is in completed_at state", func(t *testing.T) {
		paymentID := "payment-id"
		circleEntry, err := m.Insert(ctx, dbConnectionPool, paymentID)
		require.NoError(t, err)

		_, err = m.Update(ctx, dbConnectionPool, circleEntry.IdempotencyKey, CircleTransferRequestUpdate{
			CircleTransferID: "circle-transfer-id",
			Status:           CircleTransferStatusFailed,
			ResponseBody:     []byte(`{"foo":"bar"}`),
			SourceWalletID:   "source-wallet-id",
			CompletedAt:      utils.TimePtr(time.Now()),
		})
		require.NoError(t, err)

		circleEntry, err = m.GetIncompleteByPaymentID(ctx, dbConnectionPool, paymentID)
		require.ErrorIs(t, err, ErrRecordNotFound)
		require.Nil(t, circleEntry)
	})

	t.Run("🎉 successfully finds an incomplete circle transfer request", func(t *testing.T) {
		paymentID := "payment-id"
		_, err := m.Insert(ctx, dbConnectionPool, paymentID)
		require.NoError(t, err)

		circleEntry, err := m.GetIncompleteByPaymentID(ctx, dbConnectionPool, paymentID)
		require.NoError(t, err)
		require.NotNil(t, circleEntry)
		assert.Equal(t, paymentID, circleEntry.PaymentID)
	})
}

func Test_CircleTransferRequestModel_GetOrInsert(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, outerErr := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, outerErr)
	defer dbConnectionPool.Close()

	ctx := context.Background()
	m := CircleTransferRequestModel{dbConnectionPool: dbConnectionPool}

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
	payment1 := CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &Payment{
		ReceiverWallet: rwReady,
		Disbursement:   disbursement,
		Asset:          *asset,
		Amount:         "100",
		Status:         DraftPaymentStatus,
	})
	payment2 := CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &Payment{
		ReceiverWallet: rwReady,
		Disbursement:   disbursement,
		Asset:          *asset,
		Amount:         "100",
		Status:         DraftPaymentStatus,
	})

	t.Run("return an error if paymentID is empty", func(t *testing.T) {
		circleEntry, err := m.GetOrInsert(ctx, "")
		require.Error(t, err)
		require.ErrorContains(t, err, "paymentID is required")
		require.Nil(t, circleEntry)
	})

	t.Run("🎉 successfully finds an existing circle transfer request", func(t *testing.T) {
		insertedEntry, err := m.Insert(ctx, dbConnectionPool, payment1.ID)
		require.NoError(t, err)

		gotEntry, err := m.GetOrInsert(ctx, payment1.ID)
		require.NoError(t, err)
		require.NotNil(t, gotEntry)
		assert.Equal(t, insertedEntry, gotEntry)
	})

	t.Run("🎉 successfully insert a new circle transfer request", func(t *testing.T) {
		query := "SELECT COUNT(*) FROM circle_transfer_requests"
		var count int
		err := dbConnectionPool.GetContext(ctx, &count, query)
		require.NoError(t, err)
		require.Equal(t, 1, count)

		gotEntry, err := m.GetOrInsert(ctx, payment2.ID)
		require.NoError(t, err)
		require.NotNil(t, gotEntry)
		assert.Equal(t, payment2.ID, gotEntry.PaymentID)

		err = dbConnectionPool.GetContext(ctx, &count, query)
		require.NoError(t, err)
		require.Equal(t, 2, count) // <- new row inserted
	})
}

func Test_buildCircleTransferRequestQuery(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	baseQuery := "SELECT * FROM circle_transfer_requests c"

	testCases := []struct {
		name           string
		queryParams    QueryParams
		expectedQuery  string
		expectedParams []interface{}
	}{
		{
			name:           "build query without params",
			queryParams:    QueryParams{},
			expectedQuery:  "SELECT * FROM circle_transfer_requests c",
			expectedParams: []interface{}{},
		},
		{
			name: "build query with status filter (value)",
			queryParams: QueryParams{
				Filters: map[FilterKey]interface{}{
					FilterKeyStatus: "pending",
				},
			},
			expectedQuery:  "SELECT * FROM circle_transfer_requests c WHERE 1=1 AND c.status = $1",
			expectedParams: []interface{}{"pending"},
		},
		{
			name: "build query with status filter (slice)",
			queryParams: QueryParams{
				Filters: map[FilterKey]interface{}{
					FilterKeyStatus: []CircleTransferStatus{CircleTransferStatusSuccess, CircleTransferStatusFailed},
				},
			},
			expectedQuery:  "SELECT * FROM circle_transfer_requests c WHERE 1=1 AND c.status = ANY($1)",
			expectedParams: []interface{}{pq.Array([]CircleTransferStatus{CircleTransferStatusSuccess, CircleTransferStatusFailed})},
		},
		{
			name: "build query with payment_id filter",
			queryParams: QueryParams{
				Filters: map[FilterKey]interface{}{
					FilterKeyPaymentID: "test-payment-id",
				},
			},
			expectedQuery:  "SELECT * FROM circle_transfer_requests c WHERE 1=1 AND c.payment_id = $1",
			expectedParams: []interface{}{"test-payment-id"},
		},
		{
			name: "build query with IsNull(completed_at) filter",
			queryParams: QueryParams{
				Filters: map[FilterKey]interface{}{
					IsNull(FilterKeyCompletedAt): true,
				},
			},
			expectedQuery:  "SELECT * FROM circle_transfer_requests c WHERE 1=1 AND c.completed_at IS NULL",
			expectedParams: []interface{}{},
		},
		{
			name: "build query with LowerThan(sync_attempts) filter",
			queryParams: QueryParams{
				Filters: map[FilterKey]interface{}{
					LowerThan(FilterKeySyncAttempts): 7,
				},
			},
			expectedQuery:  "SELECT * FROM circle_transfer_requests c WHERE 1=1 AND c.sync_attempts < $1",
			expectedParams: []interface{}{7},
		},
		{
			name: "build query with sort by",
			queryParams: QueryParams{
				SortBy:    "created_at",
				SortOrder: "ASC",
			},
			expectedQuery:  "SELECT * FROM circle_transfer_requests c ORDER BY c.created_at ASC",
			expectedParams: []interface{}{},
		},
		{
			name: "build query with pagination",
			queryParams: QueryParams{
				Page:      1,
				PageLimit: 20,
			},
			expectedQuery:  "SELECT * FROM circle_transfer_requests c LIMIT $1 OFFSET $2",
			expectedParams: []interface{}{20, 0},
		},
		{
			name: "build query with FOR UPDATE SKIP LOCKED",
			queryParams: QueryParams{
				ForUpdateSkipLocked: true,
			},
			expectedQuery:  "SELECT * FROM circle_transfer_requests c FOR UPDATE SKIP LOCKED",
			expectedParams: []interface{}{},
		},
		{
			name: "build query with all filters, and pagination, and FOR UPDATE SKIP LOCKED",
			queryParams: QueryParams{
				Page:      1,
				PageLimit: 20,
				SortBy:    "created_at",
				SortOrder: "ASC",
				Filters: map[FilterKey]interface{}{
					FilterKeyStatus:                  "pending",
					FilterKeyPaymentID:               "test-payment-id",
					IsNull(FilterKeyCompletedAt):     true,
					LowerThan(FilterKeySyncAttempts): 7,
				},
				ForUpdateSkipLocked: true,
			},
			expectedQuery:  "SELECT * FROM circle_transfer_requests c WHERE 1=1 AND c.status = $1 AND c.payment_id = $2 AND c.completed_at IS NULL AND c.sync_attempts < $3 ORDER BY c.created_at ASC LIMIT $4 OFFSET $5 FOR UPDATE SKIP LOCKED",
			expectedParams: []interface{}{"pending", "test-payment-id", 7, 20, 0},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			query, params := buildCircleTransferRequestQuery(baseQuery, tc.queryParams, dbConnectionPool)

			assert.Equal(t, tc.expectedQuery, query)
			assert.Equal(t, tc.expectedParams, params)
		})
	}
}

func Test_CircleTransferRequestModel_GetCurrentTransfersForPaymentIDs(t *testing.T) {
	dbConnectionPool := testutils.GetDBConnectionPool(t)

	ctx := context.Background()
	m := CircleTransferRequestModel{dbConnectionPool: dbConnectionPool}

	// Create fixtures
	models, outerErr := NewModels(dbConnectionPool)
	require.NoError(t, outerErr)
	asset := CreateAssetFixture(t, ctx, dbConnectionPool, "USDC", "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVV")
	wallet := CreateWalletFixture(t, ctx, dbConnectionPool, "wallet1", "https://www.wallet.com", "www.wallet.com", "wallet1://")
	disbursement := CreateDisbursementFixture(t, ctx, dbConnectionPool, models.Disbursements, &Disbursement{
		Wallet: wallet,
		Status: ReadyDisbursementStatus,
		Asset:  asset,
	})
	receiverReady := CreateReceiverFixture(t, ctx, dbConnectionPool, &Receiver{})
	rwReady := CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiverReady.ID, wallet.ID, ReadyReceiversWalletStatus)
	payment1 := CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &Payment{
		ReceiverWallet: rwReady,
		Disbursement:   disbursement,
		Asset:          *asset,
		Amount:         "100",
		Status:         DraftPaymentStatus,
	})
	payment2 := CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &Payment{
		ReceiverWallet: rwReady,
		Disbursement:   disbursement,
		Asset:          *asset,
		Amount:         "200",
		Status:         DraftPaymentStatus,
	})
	payment3 := CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &Payment{
		ReceiverWallet: rwReady,
		Disbursement:   disbursement,
		Asset:          *asset,
		Amount:         "300",
		Status:         DraftPaymentStatus,
	})

	testCases := []struct {
		name           string
		paymentIDs     []string
		initFn         func(t *testing.T, ctx context.Context, sqlExec db.SQLExecuter)
		expectedResult map[string]*CircleTransferRequest
		expectedErr    string
	}{
		{
			name:           "return an error if paymentIDs is empty",
			paymentIDs:     []string{},
			expectedResult: nil,
			expectedErr:    "paymentIDs is required",
		},
		{
			name:       "🎉 successfully finds circle current transfer request",
			paymentIDs: []string{payment3.ID},
			initFn: func(t *testing.T, ctx context.Context, sqlExec db.SQLExecuter) {
				// insert a transfer for payment 3
				tr, err := m.Insert(ctx, sqlExec, payment3.ID)
				require.NoError(t, err)

				_, err = m.Update(ctx, sqlExec, tr.IdempotencyKey, CircleTransferRequestUpdate{
					CircleTransferID: "circle-transfer-id-3",
					Status:           CircleTransferStatusFailed,
				})
				require.NoError(t, err)

				// insert another transfer for payment 3
				tr2, err := m.Insert(ctx, sqlExec, payment3.ID)
				require.NoError(t, err)

				// updated the created_at of the second transfer to be later than the first transfer, so that the second transfer is returned by GetCurrentTransfersForPaymentIDs
				_, err = sqlExec.ExecContext(ctx, `UPDATE circle_transfer_requests SET created_at = created_at + INTERVAL '1 second' WHERE idempotency_key = $1`, tr2.IdempotencyKey)
				require.NoError(t, err)

				_, err = m.Update(ctx, sqlExec, tr2.IdempotencyKey, CircleTransferRequestUpdate{
					CircleTransferID: "circle-transfer-id-3-NEW",
					Status:           CircleTransferStatusSuccess,
				})
				require.NoError(t, err)
			},
			expectedResult: map[string]*CircleTransferRequest{
				payment3.ID: {
					PaymentID:        payment3.ID,
					CircleTransferID: utils.StringPtr("circle-transfer-id-3-NEW"),
				},
			},
		},

		{
			name:       "🎉 successfully finds circle transfer requests for multiple payments",
			paymentIDs: []string{payment1.ID, payment2.ID},
			initFn: func(t *testing.T, ctx context.Context, sqlExec db.SQLExecuter) {
				transfer1, err := m.Insert(ctx, sqlExec, payment1.ID)
				require.NoError(t, err)

				_, err = m.Update(ctx, sqlExec, transfer1.IdempotencyKey, CircleTransferRequestUpdate{
					CircleTransferID: "circle-transfer-id-1",
					Status:           CircleTransferStatusFailed,
				})
				require.NoError(t, err)

				transfer2, err := m.Insert(ctx, sqlExec, payment2.ID)
				require.NoError(t, err)

				_, err = m.Update(ctx, sqlExec, transfer2.IdempotencyKey, CircleTransferRequestUpdate{
					CircleTransferID: "circle-transfer-id-2",
					Status:           CircleTransferStatusPending,
				})
				require.NoError(t, err)
			},
			expectedResult: map[string]*CircleTransferRequest{
				payment1.ID: {
					PaymentID:        payment1.ID,
					CircleTransferID: utils.StringPtr("circle-transfer-id-1"),
				},
				payment2.ID: {
					PaymentID:        payment2.ID,
					CircleTransferID: utils.StringPtr("circle-transfer-id-2"),
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tx := testutils.BeginTxWithRollback(t, ctx, dbConnectionPool)

			if tc.initFn != nil {
				tc.initFn(t, ctx, tx)
			}

			result, err := m.GetCurrentTransfersForPaymentIDs(ctx, tx, tc.paymentIDs)
			if tc.expectedErr != "" {
				require.ErrorContains(t, err, tc.expectedErr)
				require.Nil(t, result)
			} else {
				require.NoError(t, err)
				require.NotNil(t, result)
				assert.Equal(t, len(tc.expectedResult), len(result))
				for expectedPaymentID, expectedResult := range tc.expectedResult {
					assert.NotNil(t, result[expectedPaymentID])
					assert.Equal(t, *expectedResult.CircleTransferID, *result[expectedPaymentID].CircleTransferID)
					assert.Equal(t, expectedResult.PaymentID, result[expectedPaymentID].PaymentID)
				}
			}
		})
	}
}

func Test_CircleTransferRequestModel_GetPendingReconciliation(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()
	m := CircleTransferRequestModel{dbConnectionPool: dbConnectionPool}

	t.Run("returns empty slice when no pending transfers exist", func(t *testing.T) {
		transfers, err := m.GetPendingReconciliation(ctx, dbConnectionPool)
		require.NoError(t, err)
		assert.Empty(t, transfers)
	})

	t.Run("returns only pending transfers with sync_attempts < maxSyncAttempts", func(t *testing.T) {
		// Create test data with different statuses and sync attempts
		transferTypes := []struct {
			count        int
			status       CircleTransferStatus
			syncAttempts int
		}{
			{batchSize, CircleTransferStatusPending, 0},       // Group 1: pending, no attempts
			{5, CircleTransferStatusPending, 1},               // Group 2: pending, one attempt
			{5, CircleTransferStatusPending, maxSyncAttempts}, // Group 3: pending, max attempts
			{5, CircleTransferStatusSuccess, 0},               // Group 4: success
			{5, CircleTransferStatusFailed, 0},                // Group 5: failed
		}

		for groupIdx, tt := range transferTypes {
			for i := 0; i < tt.count; i++ {
				transfer, err := m.Insert(ctx, dbConnectionPool, fmt.Sprintf("payment-id-g%d-%d", groupIdx, i))
				require.NoError(t, err)

				_, err = m.Update(ctx, dbConnectionPool, transfer.IdempotencyKey, CircleTransferRequestUpdate{
					Status:           tt.status,
					CircleTransferID: fmt.Sprintf("transfer-id-%d-%d", groupIdx, i),
					ResponseBody:     []byte(`{"foo":"bar"}`),
					SourceWalletID:   "wallet-123",
					SyncAttempts:     tt.syncAttempts,
				})
				require.NoError(t, err)
			}
		}

		// Verify initial state
		transfers, err := m.GetPendingReconciliation(ctx, dbConnectionPool)
		require.NoError(t, err)
		assert.Len(t, transfers, batchSize)
		for _, transfer := range transfers {
			assert.Equal(t, CircleTransferStatusPending, *transfer.Status)
			assert.LessOrEqual(t, transfer.SyncAttempts, maxSyncAttempts)
		}

		// Update sync attempts and verify ordering
		now := time.Now()
		for i, transfer := range transfers {
			lastSyncAttemptAt := now.Add(-time.Duration(i) * time.Second)
			_, err = m.Update(ctx, dbConnectionPool, transfer.IdempotencyKey, CircleTransferRequestUpdate{
				SyncAttempts:      maxSyncAttempts,
				LastSyncAttemptAt: &lastSyncAttemptAt,
			})
			require.NoError(t, err)
		}

		transfers, err = m.GetPendingReconciliation(ctx, dbConnectionPool)
		require.NoError(t, err)
		assert.Len(t, transfers, 5) // Only one transfer should remain under maxSyncAttempts
	})
}
