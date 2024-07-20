package data

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/db/dbtest"
)

func Test_ReceiverVerificationModel_GetByReceiverIDsAndVerificationField(t *testing.T) {
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

	actualVerifications, err := receiverVerificationModel.GetByReceiverIDsAndVerificationField(ctx, dbConnectionPool, []string{receiver1.ID, receiver2.ID, receiver3.ID}, VerificationFieldDateOfBirth)
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

func Test_ReceiverVerificationModel_GetReceiverVerificationByReceiverId(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()

	receiver := CreateReceiverFixture(t, ctx, dbConnectionPool, &Receiver{
		PhoneNumber: "+13334445555",
	})

	t.Run("returns error when the receiver has no verifications registered", func(t *testing.T) {
		receiverVerificationModel := ReceiverVerificationModel{dbConnectionPool: dbConnectionPool}
		_, err := receiverVerificationModel.GetLatestByPhoneNumber(ctx, receiver.PhoneNumber)
		require.Error(t, err, fmt.Errorf("cannot query any receiver verifications for phone number %s", receiver.PhoneNumber))
	})

	t.Run("returns the latest receiver verification for a list of receiver verifications", func(t *testing.T) {
		earlierTime := time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC)
		verification1 := CreateReceiverVerificationFixture(t, ctx, dbConnectionPool, ReceiverVerificationInsert{
			ReceiverID:        receiver.ID,
			VerificationField: VerificationFieldDateOfBirth,
			VerificationValue: "1990-01-01",
		})
		verification1.UpdatedAt = earlierTime

		verification2 := CreateReceiverVerificationFixture(t, ctx, dbConnectionPool, ReceiverVerificationInsert{
			ReceiverID:        receiver.ID,
			VerificationField: VerificationFieldPin,
			VerificationValue: "1234",
		})
		verification2.UpdatedAt = earlierTime

		verification3 := CreateReceiverVerificationFixture(t, ctx, dbConnectionPool, ReceiverVerificationInsert{
			ReceiverID:        receiver.ID,
			VerificationField: VerificationFieldNationalID,
			VerificationValue: "5678",
		})

		receiverVerificationModel := ReceiverVerificationModel{dbConnectionPool: dbConnectionPool}
		actualVerification, err := receiverVerificationModel.GetLatestByPhoneNumber(ctx, receiver.PhoneNumber)
		require.NoError(t, err)

		assert.Equal(t,
			ReceiverVerification{
				ReceiverID:        receiver.ID,
				VerificationField: VerificationFieldNationalID,
				HashedValue:       verification3.HashedValue,
				CreatedAt:         verification3.CreatedAt,
				UpdatedAt:         verification3.UpdatedAt,
			}, *actualVerification)
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

	actualVerification, err := receiverVerificationModel.GetByReceiverIDsAndVerificationField(ctx, dbConnectionPool, []string{receiver.ID}, VerificationFieldDateOfBirth)
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

	actualAfterUpdate, err := receiverVerificationModel.GetByReceiverIDsAndVerificationField(ctx, dbConnectionPool, []string{receiver.ID}, VerificationFieldDateOfBirth)
	require.NoError(t, err)
	verified = CompareVerificationValue(actualAfterUpdate[0].HashedValue, newExpectedValue)
	assert.True(t, verified)
}

func Test_ReceiverVerificationModel_UpsertVerificationValue(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()
	receiver := CreateReceiverFixture(t, ctx, dbConnectionPool, &Receiver{})
	receiverVerificationModel := ReceiverVerificationModel{}
	getReceiverVerificationHashedValue := func(t *testing.T, ctx context.Context, dbConnectionPool db.DBConnectionPool, receiverID string, verificationField VerificationField) string {
		const q = "SELECT hashed_value FROM receiver_verifications WHERE receiver_id = $1 AND verification_field = $2"
		var hashedValue string
		qErr := dbConnectionPool.GetContext(ctx, &hashedValue, q, receiverID, verificationField)
		require.NoError(t, qErr)
		return hashedValue
	}

	t.Run("upserts the verification value successfully", func(t *testing.T) {
		// Inserts the verification value
		firstVerificationValue := "123456"
		err = receiverVerificationModel.UpsertVerificationValue(ctx, dbConnectionPool, receiver.ID, VerificationFieldPin, firstVerificationValue)
		require.NoError(t, err)

		currentHashedValue := getReceiverVerificationHashedValue(t, ctx, dbConnectionPool, receiver.ID, VerificationFieldPin)
		assert.NotEmpty(t, currentHashedValue)
		verified := CompareVerificationValue(currentHashedValue, firstVerificationValue)
		assert.True(t, verified)

		// Updates the verification value
		newVerificationValue := "654321"
		err = receiverVerificationModel.UpsertVerificationValue(ctx, dbConnectionPool, receiver.ID, VerificationFieldPin, newVerificationValue)
		require.NoError(t, err)

		afterUpdateHashedValue := getReceiverVerificationHashedValue(t, ctx, dbConnectionPool, receiver.ID, VerificationFieldPin)
		assert.NotEmpty(t, afterUpdateHashedValue)

		// Checking if the hashed value is NOT the first one.
		verified = CompareVerificationValue(afterUpdateHashedValue, firstVerificationValue)
		assert.False(t, verified)
		// Checking if the hashed value is equal the updated verification value
		verified = CompareVerificationValue(afterUpdateHashedValue, newVerificationValue)
		assert.True(t, verified)
	})

	t.Run("doesn't update the verification value when it was confirmed by the receiver", func(t *testing.T) {
		// Inserts the verification value
		firstVerificationValue := "0301016957187"
		err := receiverVerificationModel.UpsertVerificationValue(ctx, dbConnectionPool, receiver.ID, VerificationFieldNationalID, firstVerificationValue)
		require.NoError(t, err)

		currentHashedValue := getReceiverVerificationHashedValue(t, ctx, dbConnectionPool, receiver.ID, VerificationFieldNationalID)
		assert.NotEmpty(t, currentHashedValue)
		verified := CompareVerificationValue(currentHashedValue, firstVerificationValue)
		assert.True(t, verified)

		// Receiver confirmed the verification value
		now := time.Now()
		err = receiverVerificationModel.UpdateReceiverVerification(ctx, ReceiverVerification{
			ReceiverID:        receiver.ID,
			VerificationField: VerificationFieldNationalID,
			Attempts:          0,
			ConfirmedAt:       &now,
			FailedAt:          nil,
		}, dbConnectionPool)
		require.NoError(t, err)

		newVerificationValue := "0301017821085"
		err = receiverVerificationModel.UpsertVerificationValue(ctx, dbConnectionPool, receiver.ID, VerificationFieldNationalID, newVerificationValue)
		require.NoError(t, err)

		afterUpdateHashedValue := getReceiverVerificationHashedValue(t, ctx, dbConnectionPool, receiver.ID, VerificationFieldNationalID)
		assert.NotEmpty(t, currentHashedValue)

		// Checking if the hashed value is NOT the new one.
		verified = CompareVerificationValue(afterUpdateHashedValue, newVerificationValue)
		assert.False(t, verified)
		// Checking if the hashed value is equal the first verification value
		verified = CompareVerificationValue(afterUpdateHashedValue, firstVerificationValue)
		assert.True(t, verified)

		assert.Equal(t, currentHashedValue, afterUpdateHashedValue)
	})
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
		attempts := 15
		e := receiverVerificationModel.ExceededAttempts(attempts)
		assert.True(t, e)
	})

	t.Run("attempts have not exceeded the max value", func(t *testing.T) {
		attempts := 1
		e := receiverVerificationModel.ExceededAttempts(attempts)
		assert.False(t, e)
	})
}

func Test_ReceiverVerificationModel_GetLatestByPhoneNumber(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()

	receiver := CreateReceiverFixture(t, ctx, dbConnectionPool, &Receiver{})
	receiverVerificationModel := ReceiverVerificationModel{dbConnectionPool: dbConnectionPool}

	err = receiverVerificationModel.UpsertVerificationValue(ctx, dbConnectionPool, receiver.ID, VerificationFieldDateOfBirth, "1990-01-01")
	require.NoError(t, err)
	err = receiverVerificationModel.UpsertVerificationValue(ctx, dbConnectionPool, receiver.ID, VerificationFieldPin, "123456")
	require.NoError(t, err)

	verification, err := receiverVerificationModel.GetLatestByPhoneNumber(ctx, receiver.PhoneNumber)
	require.NoError(t, err)

	assert.Equal(t, VerificationFieldPin, verification.VerificationField)
	assert.True(t, CompareVerificationValue(verification.HashedValue, "123456"))
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
