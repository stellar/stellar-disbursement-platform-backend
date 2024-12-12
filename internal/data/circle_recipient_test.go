package data

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/db/dbtest"
)

func Test_CircleRecipientModel_Insert(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, outerErr := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, outerErr)
	defer dbConnectionPool.Close()

	ctx := context.Background()

	wallet := CreateWalletFixture(t, ctx, dbConnectionPool, "wallet1", "https://www.wallet.com", "www.wallet.com", "wallet1://")
	receiver := CreateReceiverFixture(t, ctx, dbConnectionPool, &Receiver{})

	m := CircleRecipientModel{dbConnectionPool: dbConnectionPool}

	t.Run("returns error if receiverWalletID is empty", func(t *testing.T) {
		circleRecipient, err := m.Insert(ctx, "")
		require.EqualError(t, err, "receiverWalletID is required")
		require.Nil(t, circleRecipient)
	})

	t.Run("🎉 successfully inserts a circle recipient", func(t *testing.T) {
		receiverWallet := CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver.ID, wallet.ID, RegisteredReceiversWalletStatus)
		defer DeleteAllReceiverWalletsFixtures(t, ctx, dbConnectionPool)

		circleRecipient, err := m.Insert(ctx, receiverWallet.ID)
		require.NoError(t, err)
		require.NotNil(t, circleRecipient)

		assert.Equal(t, receiverWallet.ID, circleRecipient.ReceiverWalletID)
		assert.NotEmpty(t, circleRecipient.UpdatedAt)
		assert.NotEmpty(t, circleRecipient.CreatedAt)
		assert.Empty(t, circleRecipient.SyncAttempts)
		assert.Empty(t, circleRecipient.LastSyncAttemptAt)
		assert.NoError(t, uuid.Validate(circleRecipient.IdempotencyKey), "idempotency key should be a valid UUID")
	})

	t.Run("database constraint that prevents repeated rows with the same receiverWalletID", func(t *testing.T) {
		receiverWallet := CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver.ID, wallet.ID, RegisteredReceiversWalletStatus)
		defer DeleteAllReceiverWalletsFixtures(t, ctx, dbConnectionPool)

		_, err := m.Insert(ctx, receiverWallet.ID)
		require.NoError(t, err)

		_, err = m.Insert(ctx, receiverWallet.ID)
		require.Error(t, err)
		require.ErrorContains(t, err, "duplicate key value violates unique constraint")
	})
}

func Test_CircleRecipientModel_Update(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, outerErr := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, outerErr)
	defer dbConnectionPool.Close()

	ctx := context.Background()
	wallet := CreateWalletFixture(t, ctx, dbConnectionPool, "wallet1", "https://www.wallet.com", "www.wallet.com", "wallet1://")
	receiver := CreateReceiverFixture(t, ctx, dbConnectionPool, &Receiver{})
	m := CircleRecipientModel{dbConnectionPool: dbConnectionPool}

	updateRequest := CircleRecipientUpdate{
		IdempotencyKey:    "new-idempotency-key",
		CircleRecipientID: "circle-recipient-id",
		Status:            CircleRecipientStatusActive,
		ResponseBody:      []byte(`{"foo":"bar"}`),
		SyncAttempts:      1,
		LastSyncAttemptAt: time.Now(),
	}

	t.Run("return an error if the receiverWalletID is empty", func(t *testing.T) {
		circleRecipient, err := m.Update(ctx, "", CircleRecipientUpdate{})
		require.Error(t, err)
		require.ErrorContains(t, err, "receiverWalletID is required")
		require.Nil(t, circleRecipient)
	})

	t.Run("return an error if the circle transfer request does not exist", func(t *testing.T) {
		circleRecipient, err := m.Update(ctx, "test-key", updateRequest)
		require.Error(t, err)
		require.ErrorContains(t, err, "circle recipient with receiver_wallet_id test-key not found")
		require.ErrorIs(t, err, ErrRecordNotFound)
		require.Nil(t, circleRecipient)
	})

	t.Run("🎉 successfully updates a circle recipient", func(t *testing.T) {
		receiverWallet := CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver.ID, wallet.ID, RegisteredReceiversWalletStatus)
		defer DeleteAllReceiverWalletsFixtures(t, ctx, dbConnectionPool)

		_, err := m.Insert(ctx, receiverWallet.ID)
		require.NoError(t, err)

		updatedCircleRecipient, err := m.Update(ctx, receiverWallet.ID, updateRequest)
		require.NoError(t, err)
		require.NotNil(t, updatedCircleRecipient)

		assert.Equal(t, updateRequest.IdempotencyKey, updatedCircleRecipient.IdempotencyKey)
		assert.Equal(t, updateRequest.CircleRecipientID, updatedCircleRecipient.CircleRecipientID)
		assert.Equal(t, updateRequest.Status, updatedCircleRecipient.Status)
		assert.JSONEq(t, string(updateRequest.ResponseBody), string(updatedCircleRecipient.ResponseBody))
		assert.Equal(t, updateRequest.SyncAttempts, updatedCircleRecipient.SyncAttempts)
		assert.Equalf(t, updateRequest.LastSyncAttemptAt.Unix(), updatedCircleRecipient.LastSyncAttemptAt.Unix(), "LastSyncAttemptAt doesn't match: %v != %v", updateRequest.LastSyncAttemptAt.Unix(), updatedCircleRecipient.LastSyncAttemptAt.Unix())
	})
}

func Test_CircleRecipientModel_GetByReceiverWalletID(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, outerErr := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, outerErr)
	defer dbConnectionPool.Close()

	ctx := context.Background()
	wallet := CreateWalletFixture(t, ctx, dbConnectionPool, "wallet1", "https://www.wallet.com", "www.wallet.com", "wallet1://")
	receiver := CreateReceiverFixture(t, ctx, dbConnectionPool, &Receiver{})
	m := CircleRecipientModel{dbConnectionPool: dbConnectionPool}

	t.Run("return an error if the receiverWalletID is empty", func(t *testing.T) {
		circleRecipient, err := m.GetByReceiverWalletID(ctx, "")
		require.Error(t, err)
		require.ErrorContains(t, err, "receiverWalletID is required")
		require.Nil(t, circleRecipient)
	})

	t.Run("return an error if the circle transfer request does not exist", func(t *testing.T) {
		circleRecipient, err := m.GetByReceiverWalletID(ctx, "test-key")
		require.Error(t, err)
		require.ErrorContains(t, err, "record not found")
		require.ErrorIs(t, err, ErrRecordNotFound)
		require.Nil(t, circleRecipient)
	})

	t.Run("🎉 successfully retrieves a circle recipient", func(t *testing.T) {
		receiverWallet := CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver.ID, wallet.ID, RegisteredReceiversWalletStatus)
		defer DeleteAllReceiverWalletsFixtures(t, ctx, dbConnectionPool)

		insertedCircleRecipient, err := m.Insert(ctx, receiverWallet.ID)
		require.NoError(t, err)

		fetchedCircleRecipient, err := m.GetByReceiverWalletID(ctx, receiverWallet.ID)
		require.NoError(t, err)
		require.NotNil(t, fetchedCircleRecipient)

		assert.Equal(t, insertedCircleRecipient, fetchedCircleRecipient)
	})
}

func Test_CircleRecipientModel_ResetRecipientsForRetryIfNeeded(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()

	m, err := NewModels(dbConnectionPool)
	require.NoError(t, err)

	asset := CreateAssetFixture(t, ctx, dbConnectionPool, "FOO1", "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVV")
	receiver := CreateReceiverFixture(t, ctx, dbConnectionPool, &Receiver{})

	walletA := CreateWalletFixture(t, ctx, dbConnectionPool, "walletA", "https://www.a.com", "www.a.com", "a://")
	disbursementA := CreateDisbursementFixture(t, ctx, dbConnectionPool, m.Disbursements, &Disbursement{
		Wallet:                              walletA,
		Status:                              ReadyDisbursementStatus,
		Asset:                               asset,
		ReceiverRegistrationMessageTemplate: "Disbursement SMS Registration Message Template A1",
	})
	rwA := CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver.ID, walletA.ID, RegisteredReceiversWalletStatus)
	paymentA1 := CreatePaymentFixture(t, ctx, dbConnectionPool, m.Payment, &Payment{
		ReceiverWallet: rwA,
		Disbursement:   disbursementA,
		Asset:          *asset,
		Status:         ReadyPaymentStatus,
		Amount:         "100",
	})
	paymentA2 := CreatePaymentFixture(t, ctx, dbConnectionPool, m.Payment, &Payment{
		ReceiverWallet: rwA,
		Disbursement:   disbursementA,
		Asset:          *asset,
		Status:         ReadyPaymentStatus,
		Amount:         "100",
	})

	walletB := CreateWalletFixture(t, ctx, dbConnectionPool, "walletB", "https://www.b.com", "www.b.com", "b://")
	disbursementB := CreateDisbursementFixture(t, ctx, dbConnectionPool, m.Disbursements, &Disbursement{
		Wallet:                              walletB,
		Status:                              ReadyDisbursementStatus,
		Asset:                               asset,
		ReceiverRegistrationMessageTemplate: "Disbursement SMS Registration Message Template A1",
	})
	rwB := CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver.ID, walletB.ID, RegisteredReceiversWalletStatus)
	paymentB1 := CreatePaymentFixture(t, ctx, dbConnectionPool, m.Payment, &Payment{
		ReceiverWallet: rwB,
		Disbursement:   disbursementB,
		Asset:          *asset,
		Status:         ReadyPaymentStatus,
		Amount:         "100",
	})
	paymentB2 := CreatePaymentFixture(t, ctx, dbConnectionPool, m.Payment, &Payment{
		ReceiverWallet: rwB,
		Disbursement:   disbursementA,
		Asset:          *asset,
		Status:         ReadyPaymentStatus,
		Amount:         "100",
	})

	t.Run("🔴 fails if no Payment IDs are provided", func(t *testing.T) {
		circleRecipients, err := m.CircleRecipient.ResetRecipientsForRetryIfNeeded(ctx, dbConnectionPool)
		require.ErrorContains(t, err, "at least one payment ID is required")
		require.Nil(t, circleRecipients)
	})

	now := time.Now()
	type TestCases struct {
		name              string
		prepareFixturesFn func(t *testing.T, models *Models)
		paymentIDs        []string
		wantUpdatedIDs    []string
	}
	testCases := []TestCases{
		{
			name:           "🟡 nothing happens if no paymentIDs are found",
			paymentIDs:     []string{"non-existent", "non-existent-2"},
			wantUpdatedIDs: []string{},
		},
		{
			name:           "🟡 nothing happens if no circle recipients are found for the given payment IDs",
			paymentIDs:     []string{paymentA1.ID, paymentA2.ID, paymentB1.ID, paymentB2.ID},
			wantUpdatedIDs: []string{},
		},
		{
			name: "🟢 only 'paymentA*' related recipients were updated",
			prepareFixturesFn: func(t *testing.T, models *Models) {
				for _, rwID := range []string{rwA.ID, rwB.ID} {
					CreateCircleRecipientFixture(t, ctx, dbConnectionPool, CircleRecipient{
						IdempotencyKey:   uuid.NewString(),
						ReceiverWalletID: rwID,
						UpdatedAt:        now,
						CreatedAt:        now,
						Status:           CircleRecipientStatusDenied,
					})
				}
			},
			paymentIDs:     []string{paymentA1.ID, paymentA2.ID},
			wantUpdatedIDs: []string{rwA.ID},
		},
		{
			name: "🟢 only 'payment*1' related recipients were updated",
			prepareFixturesFn: func(t *testing.T, models *Models) {
				for _, rwID := range []string{rwA.ID, rwB.ID} {
					CreateCircleRecipientFixture(t, ctx, dbConnectionPool, CircleRecipient{
						IdempotencyKey:   uuid.NewString(),
						ReceiverWalletID: rwID,
						UpdatedAt:        now,
						CreatedAt:        now,
						Status:           CircleRecipientStatusDenied,
					})
				}
			},
			paymentIDs:     []string{paymentA1.ID, paymentB1.ID},
			wantUpdatedIDs: []string{rwA.ID, rwB.ID},
		},
	}

	for _, nonActiveStatus := range []CircleRecipientStatus{
		CircleRecipientStatusDenied,
		CircleRecipientStatusInactive,
		CircleRecipientStatusPending,
		"",
	} {
		testCases = append(testCases, TestCases{
			name: fmt.Sprintf("🟢 all recipients were updated [status=%s]", nonActiveStatus),
			prepareFixturesFn: func(t *testing.T, models *Models) {
				for _, rwID := range []string{rwA.ID, rwB.ID} {
					CreateCircleRecipientFixture(t, ctx, dbConnectionPool, CircleRecipient{
						IdempotencyKey:   uuid.NewString(),
						ReceiverWalletID: rwID,
						UpdatedAt:        now,
						CreatedAt:        now,
						Status:           nonActiveStatus,
					})
				}
			},
			paymentIDs:     []string{paymentA1.ID, paymentA2.ID, paymentB1.ID, paymentB2.ID},
			wantUpdatedIDs: []string{rwA.ID, rwB.ID},
		})
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			defer DeleteAllCircleRecipientsFixtures(t, ctx, m.DBConnectionPool)
			if tc.prepareFixturesFn != nil {
				tc.prepareFixturesFn(t, m)
			}

			circleRecipients, err := m.CircleRecipient.ResetRecipientsForRetryIfNeeded(ctx, dbConnectionPool, tc.paymentIDs...)
			require.NoError(t, err)

			updatedIDs := make([]string, 0, len(circleRecipients))
			for _, cr := range circleRecipients {
				updatedIDs = append(updatedIDs, cr.ReceiverWalletID)
			}
			require.ElementsMatch(t, tc.wantUpdatedIDs, updatedIDs)

			if len(updatedIDs) > 0 {
				for _, rwID := range updatedIDs {
					circleRecipient, err := m.CircleRecipient.GetByReceiverWalletID(ctx, rwID)
					require.NoError(t, err)
					require.NotNil(t, circleRecipient)
					assert.Empty(t, circleRecipient.Status)
					assert.Empty(t, circleRecipient.SyncAttempts)
					assert.Empty(t, circleRecipient.LastSyncAttemptAt)
					assert.Empty(t, circleRecipient.ResponseBody)
				}
			}
		})
	}
}
