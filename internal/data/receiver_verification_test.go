package data

import (
	"context"
	"testing"
	"time"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/db"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/db/dbtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_ReceiverVerificationModel_GetByReceiverIdsAndVerificationField(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()

	receiver1 := CreateReceiverFixture(t, ctx, dbConnectionPool, &Receiver{})
	receiver2 := CreateReceiverFixture(t, ctx, dbConnectionPool, &Receiver{})
	receiver3 := CreateReceiverFixture(t, ctx, dbConnectionPool, &Receiver{})

	verification1 := CreateReceiverVerificationFixture(t, ctx, dbConnectionPool, ReceiverVerificationInsert{
		ReceiverID:        receiver1.ID,
		VerificationField: VerificationFieldDateOfBirth,
		VerificationValue: "1990-01-01",
	})
	verification2 := CreateReceiverVerificationFixture(t, ctx, dbConnectionPool, ReceiverVerificationInsert{
		ReceiverID:        receiver2.ID,
		VerificationField: VerificationFieldDateOfBirth,
		VerificationValue: "1990-01-02",
	})
	CreateReceiverVerificationFixture(t, ctx, dbConnectionPool, ReceiverVerificationInsert{
		ReceiverID:        receiver3.ID,
		VerificationField: VerificationFieldPin,
		VerificationValue: "1990-01-03",
	})

	verifiedReceivers := []string{receiver1.ID, receiver2.ID}
	verifieldValues := []string{verification1.HashedValue, verification2.HashedValue}

	receiverVerificationModel := ReceiverVerificationModel{}

	actualVerifications, err := receiverVerificationModel.GetByReceiverIdsAndVerificationField(ctx, dbConnectionPool, []string{receiver1.ID, receiver2.ID, receiver3.ID}, VerificationFieldDateOfBirth)
	require.NoError(t, err)
	assert.Equal(t, 2, len(actualVerifications))
	for _, v := range actualVerifications {
		assert.Equal(t, VerificationFieldDateOfBirth, v.VerificationField)
		assert.Contains(t, verifiedReceivers, v.ReceiverID)
		assert.Contains(t, verifieldValues, v.HashedValue)
	}
}

func Test_ReceiverVerificationModel_GetAllByReceiverId(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()

	receiver := CreateReceiverFixture(t, ctx, dbConnectionPool, &Receiver{})

	t.Run("returns empty when the receiver has no verifications registered", func(t *testing.T) {
		receiverVerificationModel := ReceiverVerificationModel{}
		actualVerifications, err := receiverVerificationModel.GetAllByReceiverId(ctx, dbConnectionPool, receiver.ID)
		require.NoError(t, err)
		assert.Len(t, actualVerifications, 0)

		assert.Equal(t, []ReceiverVerification{}, actualVerifications)
	})

	t.Run("returns all when the receiver has verifications registered", func(t *testing.T) {
		verification1 := CreateReceiverVerificationFixture(t, ctx, dbConnectionPool, ReceiverVerificationInsert{
			ReceiverID:        receiver.ID,
			VerificationField: VerificationFieldDateOfBirth,
			VerificationValue: "1990-01-01",
		})
		verification2 := CreateReceiverVerificationFixture(t, ctx, dbConnectionPool, ReceiverVerificationInsert{
			ReceiverID:        receiver.ID,
			VerificationField: VerificationFieldPin,
			VerificationValue: "1234",
		})
		verification3 := CreateReceiverVerificationFixture(t, ctx, dbConnectionPool, ReceiverVerificationInsert{
			ReceiverID:        receiver.ID,
			VerificationField: VerificationFieldNationalID,
			VerificationValue: "5678",
		})

		receiverVerificationModel := ReceiverVerificationModel{}
		actualVerifications, err := receiverVerificationModel.GetAllByReceiverId(ctx, dbConnectionPool, receiver.ID)
		require.NoError(t, err)
		assert.Len(t, actualVerifications, 3)

		assert.Equal(t, []ReceiverVerification{
			{
				ReceiverID:        receiver.ID,
				VerificationField: VerificationFieldDateOfBirth,
				HashedValue:       verification1.HashedValue,
				CreatedAt:         verification1.CreatedAt,
				UpdatedAt:         verification1.UpdatedAt,
			},
			{
				ReceiverID:        receiver.ID,
				VerificationField: VerificationFieldPin,
				HashedValue:       verification2.HashedValue,
				CreatedAt:         verification2.CreatedAt,
				UpdatedAt:         verification2.UpdatedAt,
			},
			{
				ReceiverID:        receiver.ID,
				VerificationField: VerificationFieldNationalID,
				HashedValue:       verification3.HashedValue,
				CreatedAt:         verification3.CreatedAt,
				UpdatedAt:         verification3.UpdatedAt,
			},
		}, actualVerifications)
	})
}

func Test_ReceiverVerificationModel_Insert(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()

	receiver := CreateReceiverFixture(t, ctx, dbConnectionPool, &Receiver{})

	receiverVerificationModel := ReceiverVerificationModel{}

	verification := ReceiverVerificationInsert{
		ReceiverID:        receiver.ID,
		VerificationField: VerificationFieldDateOfBirth,
		VerificationValue: "1990-01-01",
	}

	_, err = receiverVerificationModel.Insert(ctx, dbConnectionPool, verification)
	require.NoError(t, err)

	actualVerification, err := receiverVerificationModel.GetByReceiverIdsAndVerificationField(ctx, dbConnectionPool, []string{receiver.ID}, VerificationFieldDateOfBirth)
	require.NoError(t, err)
	verified := CompareVerificationValue(actualVerification[0].HashedValue, verification.VerificationValue)
	assert.True(t, verified)
	assert.Equal(t, verification.ReceiverID, actualVerification[0].ReceiverID)
	assert.Equal(t, verification.VerificationField, actualVerification[0].VerificationField)
}

func Test_ReceiverVerificationModel_UpdateVerificationValue(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()

	receiver := CreateReceiverFixture(t, ctx, dbConnectionPool, &Receiver{})

	receiverVerificationModel := ReceiverVerificationModel{}

	oldExpectedValue := "1990-01-01"
	actualBeforeUpdate, err := receiverVerificationModel.Insert(ctx, dbConnectionPool, ReceiverVerificationInsert{
		ReceiverID:        receiver.ID,
		VerificationField: VerificationFieldDateOfBirth,
		VerificationValue: oldExpectedValue,
	})
	require.NoError(t, err)
	assert.NotEmpty(t, actualBeforeUpdate)
	verified := CompareVerificationValue(actualBeforeUpdate, oldExpectedValue)
	assert.True(t, verified)

	newExpectedValue := "1990-01-02"
	err = receiverVerificationModel.UpdateVerificationValue(ctx, dbConnectionPool, receiver.ID, VerificationFieldDateOfBirth, newExpectedValue)
	require.NoError(t, err)

	actualAfterUpdate, err := receiverVerificationModel.GetByReceiverIdsAndVerificationField(ctx, dbConnectionPool, []string{receiver.ID}, VerificationFieldDateOfBirth)
	require.NoError(t, err)
	verified = CompareVerificationValue(actualAfterUpdate[0].HashedValue, newExpectedValue)
	assert.True(t, verified)
}

func Test_ReceiverVerificationModel_UpdateReceiverVerification(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()

	receiver := CreateReceiverFixture(t, ctx, dbConnectionPool, &Receiver{})
	receiverVerificationModel := ReceiverVerificationModel{}

	verification := CreateReceiverVerificationFixture(t, ctx, dbConnectionPool, ReceiverVerificationInsert{
		ReceiverID:        receiver.ID,
		VerificationField: VerificationFieldDateOfBirth,
		VerificationValue: "1990-01-01",
	})

	assert.Empty(t, verification.ConfirmedAt)
	assert.Empty(t, verification.FailedAt)
	assert.Equal(t, 0, verification.Attempts)

	date := time.Date(2023, 1, 10, 23, 40, 20, 1000, time.UTC)
	verification.Attempts = 5
	verification.ConfirmedAt = &date
	verification.FailedAt = &date

	err = receiverVerificationModel.UpdateReceiverVerification(ctx, *verification, dbConnectionPool)
	require.NoError(t, err)

	// validate if the receiver verification has been updated
	query := `
		SELECT
			rv.attempts,
			rv.confirmed_at,
			rv.failed_at
		FROM
			receiver_verifications rv
		WHERE
			rv.receiver_id = $1 AND rv.verification_field = $2
	`
	receiverVerificationUpdated := ReceiverVerification{}
	err = dbConnectionPool.GetContext(ctx, &receiverVerificationUpdated, query, verification.ReceiverID, verification.VerificationField)
	require.NoError(t, err)

	assert.Equal(t, &date, receiverVerificationUpdated.ConfirmedAt)
	assert.Equal(t, &date, receiverVerificationUpdated.FailedAt)
	assert.Equal(t, 5, receiverVerificationUpdated.Attempts)
}

func Test_ReceiverVerificationModel_CheckTotalAttempts(t *testing.T) {
	receiverVerificationModel := &ReceiverVerificationModel{}

	t.Run("attempts exceeded the max value", func(t *testing.T) {
		attempts := 6
		e := receiverVerificationModel.ExceededAttempts(attempts)
		assert.True(t, e)
	})

	t.Run("attempts have not exceeded the max value", func(t *testing.T) {
		attempts := 1
		e := receiverVerificationModel.ExceededAttempts(attempts)
		assert.False(t, e)
	})
}

func Test_ReceiverVerification_HashAndCompareVerificationValue(t *testing.T) {
	verificationValue := "1987-01-01"
	hashedVerificationInfo, err := HashVerificationValue(verificationValue)
	require.NoError(t, err)
	assert.NotEmpty(t, hashedVerificationInfo)

	assert.NotEqual(t, verificationValue, hashedVerificationInfo)
	assert.Len(t, hashedVerificationInfo, 60)

	compare := CompareVerificationValue(hashedVerificationInfo, verificationValue)
	assert.True(t, compare)
}
