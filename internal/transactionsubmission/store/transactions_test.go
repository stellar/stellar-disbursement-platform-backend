package store

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stellar/go/keypair"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/db"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/db/dbtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_Transaction_IsLocked(t *testing.T) {
	const currentLedgerNumber = 10

	testCases := []struct {
		name                    string
		lockedUntilLedgerNumber sql.NullInt32
		wantResult              bool
	}{
		{
			name:                    "returns false if lockedUntilLedgerNumber is null",
			lockedUntilLedgerNumber: sql.NullInt32{},
			wantResult:              false,
		},
		{
			name:                    "returns false if lockedUntilLedgerNumber is lower than currentLedgerNumber",
			lockedUntilLedgerNumber: sql.NullInt32{Int32: currentLedgerNumber - 1, Valid: true},
			wantResult:              false,
		},
		{
			name:                    "returns true if lockedUntilLedgerNumber is equal to currentLedgerNumber",
			lockedUntilLedgerNumber: sql.NullInt32{Int32: currentLedgerNumber, Valid: true},
			wantResult:              true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tx := &Transaction{LockedUntilLedgerNumber: tc.lockedUntilLedgerNumber}
			assert.Equal(t, tc.wantResult, tx.IsLocked(currentLedgerNumber))
		})
	}
}

func Test_TransactionModel_Insert(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()
	txModel := NewTransactionModel(dbConnectionPool)

	t.Run("return an error if the input parameters are invalid", func(t *testing.T) {
		tx, err := txModel.Insert(ctx, Transaction{ExternalID: "external-id-1"})
		require.Error(t, err)
		assert.EqualError(t, err, "inserting single transaction: validating transaction for insertion: asset code must have between 1 and 12 characters")
		assert.Nil(t, tx)
	})

	t.Run("🎉 successfully insert a new Transaction", func(t *testing.T) {
		transaction, err := txModel.Insert(ctx, Transaction{
			ExternalID:  "external-id-1",
			AssetCode:   "USDC",
			AssetIssuer: "GCBIRB7Q5T53H4L6P5QSI3O6LPD5MBWGM5GHE7A5NY4XT5OT4VCOEZFX",
			Amount:      1,
			Destination: "GBHNIYGWZUAVZX7KTLVSMILBXJMUACVO6XBEKIN6RW7AABDFH6S7GK2Y",
		})
		require.NoError(t, err)
		require.NotNil(t, transaction)

		refreshedTx, err := txModel.Get(ctx, transaction.ID)
		require.NoError(t, err)
		assert.Equal(t, transaction, refreshedTx)

		assert.Equal(t, "external-id-1", refreshedTx.ExternalID)
		assert.Equal(t, "USDC", refreshedTx.AssetCode)
		assert.Equal(t, "GCBIRB7Q5T53H4L6P5QSI3O6LPD5MBWGM5GHE7A5NY4XT5OT4VCOEZFX", refreshedTx.AssetIssuer)
		assert.Equal(t, float64(1), refreshedTx.Amount)
		assert.Equal(t, "GBHNIYGWZUAVZX7KTLVSMILBXJMUACVO6XBEKIN6RW7AABDFH6S7GK2Y", refreshedTx.Destination)
		assert.Equal(t, TransactionStatusPending, refreshedTx.Status)
	})
}

func Test_TransactionModel_BulkInsert(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()
	txModel := NewTransactionModel(dbConnectionPool)
	defer DeleteAllTransactionFixtures(t, ctx, dbConnectionPool)

	t.Run("return nil with no error if the input slice is nil", func(t *testing.T) {
		insertedTransactions, err := txModel.BulkInsert(ctx, dbConnectionPool, nil)
		require.NoError(t, err)
		assert.Nil(t, insertedTransactions)
	})

	t.Run("return nil with no error if the input slice is empty", func(t *testing.T) {
		insertedTransactions, err := txModel.BulkInsert(ctx, dbConnectionPool, []Transaction{})
		require.NoError(t, err)
		assert.Nil(t, insertedTransactions)
	})

	t.Run("return an error if the input parameters are invalid", func(t *testing.T) {
		transactionsToInsert := []Transaction{{ExternalID: "external-id-1"}}
		insertedTransactions, err := txModel.BulkInsert(ctx, dbConnectionPool, transactionsToInsert)
		require.Error(t, err)
		assert.EqualError(t, err, "validating transaction for insertion: asset code must have between 1 and 12 characters")
		assert.Nil(t, insertedTransactions)
	})

	t.Run("🎉 successfully inserts the transactions successfully", func(t *testing.T) {
		incomingTx1 := Transaction{
			ExternalID:  "external-id-1",
			AssetCode:   "USDC",
			AssetIssuer: keypair.MustRandom().Address(),
			Amount:      1,
			Destination: keypair.MustRandom().Address(),
		}
		incomingTx2 := Transaction{
			ExternalID:  "external-id-2",
			AssetCode:   "USDC",
			AssetIssuer: keypair.MustRandom().Address(),
			Amount:      2,
			Destination: keypair.MustRandom().Address(),
		}
		insertedTransactions, err := txModel.BulkInsert(ctx, dbConnectionPool, []Transaction{incomingTx1, incomingTx2})
		require.NoError(t, err)
		assert.NotNil(t, insertedTransactions)
		assert.Len(t, insertedTransactions, 2)

		var insertedTx1, insertedTx2 Transaction
		for _, tx := range insertedTransactions {
			if tx.ExternalID == incomingTx1.ExternalID {
				insertedTx1 = tx
			} else if tx.ExternalID == incomingTx2.ExternalID {
				insertedTx2 = tx
			} else {
				require.FailNow(t, "unexpected transaction: %v", tx)
			}
		}

		assert.Equal(t, incomingTx1.ExternalID, insertedTx1.ExternalID)
		assert.Equal(t, incomingTx1.AssetCode, insertedTx1.AssetCode)
		assert.Equal(t, incomingTx1.AssetIssuer, insertedTx1.AssetIssuer)
		assert.Equal(t, incomingTx1.Amount, insertedTx1.Amount)
		assert.Equal(t, incomingTx1.Destination, insertedTx1.Destination)
		assert.Equal(t, TransactionStatusPending, insertedTx1.Status)

		assert.Equal(t, incomingTx2.ExternalID, insertedTx2.ExternalID)
		assert.Equal(t, incomingTx2.AssetCode, insertedTx2.AssetCode)
		assert.Equal(t, incomingTx2.AssetIssuer, insertedTx2.AssetIssuer)
		assert.Equal(t, incomingTx2.Amount, insertedTx2.Amount)
		assert.Equal(t, incomingTx2.Destination, insertedTx2.Destination)
		assert.Equal(t, TransactionStatusPending, insertedTx2.Status)
	})
}

func Test_TransactionModel_UpdateStatusToSuccess(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()
	txModel := NewTransactionModel(dbConnectionPool)

	testCases := []struct {
		name              string
		transactionStatus TransactionStatus
		wantErrContains   string
	}{
		{
			name:              "cannot transition PENDING->SUCCESS",
			transactionStatus: TransactionStatusPending,
			wantErrContains:   "attempting to transition transaction status to TransactionStatusSuccess: cannot transition from PENDING to SUCCESS",
		},
		{
			name:              "🎉 successfully transition PROCESSING->SUCCESS",
			transactionStatus: TransactionStatusProcessing,
		},
		{
			name:              "cannot transition SUCCESS->SUCCESS",
			transactionStatus: TransactionStatusSuccess,
			wantErrContains:   "attempting to transition transaction status to TransactionStatusSuccess: cannot transition from SUCCESS to SUCCESS",
		},
		{
			name:              "cannot transition ERROR->SUCCESS",
			transactionStatus: TransactionStatusError,
			wantErrContains:   "attempting to transition transaction status to TransactionStatusSuccess: cannot transition from ERROR to SUCCESS",
		},
	}

	unphazedTx := CreateTransactionFixture(
		t,
		ctx,
		dbConnectionPool,
		uuid.NewString(),
		"USDC",
		"GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5",
		"GBHNIYGWZUAVZX7KTLVSMILBXJMUACVO6XBEKIN6RW7AABDFH6S7GK2Y",
		TransactionStatusPending,
		1.23,
	)

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tx := CreateTransactionFixture(
				t,
				ctx,
				dbConnectionPool,
				uuid.NewString(),
				"USDC",
				"GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5",
				"GBHNIYGWZUAVZX7KTLVSMILBXJMUACVO6XBEKIN6RW7AABDFH6S7GK2Y",
				tc.transactionStatus,
				1.23,
			)
			if (tc.transactionStatus != TransactionStatusSuccess) && (tc.transactionStatus != TransactionStatusError) {
				assert.Empty(t, tx.CompletedAt)
			} else {
				assert.NotEmpty(t, tx.CompletedAt)
			}

			updatedTx, err := txModel.UpdateStatusToSuccess(ctx, *tx)
			if tc.wantErrContains != "" {
				require.Error(t, err)
				assert.ErrorContains(t, err, tc.wantErrContains)
			} else {
				require.NoError(t, err)
				assert.Equal(t, TransactionStatusSuccess, updatedTx.Status)
				assert.NotEmpty(t, updatedTx.CompletedAt)

				// verify that the only fields that changed are updated_at, completed_at, status and status_history:
				tx.UpdatedAt = updatedTx.UpdatedAt
				tx.CompletedAt = updatedTx.CompletedAt
				tx.Status = updatedTx.Status
				tx.StatusHistory = append(TransactionStatusHistory{}, updatedTx.StatusHistory...)
				assert.Equal(t, tx, updatedTx)
			}

			// verify the unphazed transaction was not updated
			refreshedUnphazedTx, err := txModel.Get(ctx, unphazedTx.ID)
			require.NoError(t, err)
			assert.Equal(t, unphazedTx, refreshedUnphazedTx)
		})
	}
}

func Test_TransactionModel_UpdateStatusToError(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()
	txModel := NewTransactionModel(dbConnectionPool)

	testCases := []struct {
		name              string
		transactionStatus TransactionStatus
		wantErrContains   string
	}{
		{
			name:              "cannot transition PENDING->ERROR",
			transactionStatus: TransactionStatusPending,
			wantErrContains:   "attempting to transition transaction status to TransactionStatusError: cannot transition from PENDING to ERROR",
		},
		{
			name:              "🎉 successfully transition PROCESSING->ERROR",
			transactionStatus: TransactionStatusProcessing,
		},
		{
			name:              "cannot transition SUCCESS->ERROR",
			transactionStatus: TransactionStatusSuccess,
			wantErrContains:   "attempting to transition transaction status to TransactionStatusError: cannot transition from SUCCESS to ERROR",
		},
		{
			name:              "cannot transition ERROR->ERROR",
			transactionStatus: TransactionStatusError,
			wantErrContains:   "attempting to transition transaction status to TransactionStatusError: cannot transition from ERROR to ERROR",
		},
	}

	unphazedTx := CreateTransactionFixture(
		t,
		ctx,
		dbConnectionPool,
		uuid.NewString(),
		"USDC",
		"GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5",
		"GBHNIYGWZUAVZX7KTLVSMILBXJMUACVO6XBEKIN6RW7AABDFH6S7GK2Y",
		TransactionStatusPending,
		1.23,
	)

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tx := CreateTransactionFixture(
				t,
				ctx,
				dbConnectionPool,
				uuid.NewString(),
				"USDC",
				"GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5",
				"GBHNIYGWZUAVZX7KTLVSMILBXJMUACVO6XBEKIN6RW7AABDFH6S7GK2Y",
				tc.transactionStatus,
				1.23,
			)
			assert.Empty(t, tx.StatusMessage)
			if (tc.transactionStatus != TransactionStatusSuccess) && (tc.transactionStatus != TransactionStatusError) {
				assert.Empty(t, tx.CompletedAt)
			} else {
				assert.NotEmpty(t, tx.CompletedAt)
			}

			const someErrMessage = "some error message"
			updatedTx, err := txModel.UpdateStatusToError(ctx, *tx, someErrMessage)
			if tc.wantErrContains != "" {
				require.Error(t, err)
				assert.ErrorContains(t, err, tc.wantErrContains)
			} else {
				require.NoError(t, err)
				assert.Equal(t, TransactionStatusError, updatedTx.Status)
				assert.NotEmpty(t, updatedTx.CompletedAt)

				// verify that the only fields that changed are updated_at, completed_at, status, status_message and status history:
				tx.UpdatedAt = updatedTx.UpdatedAt
				tx.CompletedAt = updatedTx.CompletedAt
				tx.Status = updatedTx.Status
				tx.StatusMessage = sql.NullString{String: someErrMessage, Valid: true}
				tx.StatusHistory = append(TransactionStatusHistory{}, updatedTx.StatusHistory...)
				assert.Equal(t, tx, updatedTx)
			}

			// verify the unphazed transaction was not updated
			refreshedUnphazedTx, err := txModel.Get(ctx, unphazedTx.ID)
			require.NoError(t, err)
			assert.Equal(t, unphazedTx, refreshedUnphazedTx)
		})
	}
}

func Test_TransactionModel_UpdateStellarTransactionHashAndXDRSent(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()
	txModel := NewTransactionModel(dbConnectionPool)

	const txHash = "3389e9f0f1a65f19736cacf544c2e825313e8447f569233bb8db39aa607c8889"
	const envelopeXDR = "AAAAAGL8HQvQkbK2HA3WVjRrKmjX00fG8sLI7m0ERwJW/AX3AAAACgAAAAAAAAABAAAAAAAAAAAAAAABAAAAAAAAAAAAAAAArqN6LeOagjxMaUP96Bzfs9e0corNZXzBWJkFoK7kvkwAAAAAO5rKAAAAAAAAAAABVvwF9wAAAEAKZ7IPj/46PuWU6ZOtyMosctNAkXRNX9WCAI5RnfRk+AyxDLoDZP/9l3NvsxQtWj9juQOuoBlFLnWu8intgxQA"
	const resultXDR = "AAAAAAAAAGQAAAAAAAAAAQAAAAAAAAAOAAAAAAAAAABw2JZZYIt4n/WXKcnDow3mbTBMPrOnldetgvGUlpTSEQAAAAA="

	testCases := []struct {
		name            string
		transaction     Transaction
		txHash          string
		xdrSent         string
		wantErrContains string
	}{
		{
			name:            "returns an error if the size of the txHash if invalid",
			txHash:          "invalid-tx-hash",
			wantErrContains: `invalid transaction hash "invalid-tx-hash"`,
		},
		{
			name:            "returns an error if XDR is empty",
			txHash:          txHash,
			wantErrContains: "invalid XDR envelope: decoding EnvelopeType: decoding EnvelopeType: xdr:DecodeInt: EOF while decoding 4 bytes - read: '[]'",
		},
		{
			name:            "returns an error if XDR is not a valid base64 encoded",
			txHash:          txHash,
			xdrSent:         "not-base-64-encoded",
			wantErrContains: "invalid XDR envelope: decoding EnvelopeType: decoding EnvelopeType: xdr:DecodeInt: illegal base64 data at input byte",
		},
		{
			name:            "returns an error if XDR is not a transaction envelope",
			txHash:          txHash,
			xdrSent:         resultXDR,
			wantErrContains: "invalid XDR envelope: decoding TransactionV0Envelope: decoding TransactionV0: decoding TimeBounds",
		},
		{
			name:    "🎉 successfully validate both the tx hash and the XDR envelope, and save them to the DB",
			txHash:  txHash,
			xdrSent: envelopeXDR,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// create a new transaction
			tx, err := txModel.Insert(ctx, Transaction{
				ExternalID:  uuid.NewString(),
				AssetCode:   "USDC",
				AssetIssuer: "GCBIRB7Q5T53H4L6P5QSI3O6LPD5MBWGM5GHE7A5NY4XT5OT4VCOEZFX",
				Amount:      1,
				Destination: "GBHNIYGWZUAVZX7KTLVSMILBXJMUACVO6XBEKIN6RW7AABDFH6S7GK2Y",
			})
			require.NoError(t, err)
			require.NotNil(t, tx)

			// verify the transaction was created
			originalTx, err := txModel.Get(ctx, tx.ID)
			require.NoError(t, err)

			assert.False(t, originalTx.XDRSent.Valid)
			assert.Equal(t, "", originalTx.XDRSent.String)
			assert.False(t, originalTx.StellarTransactionHash.Valid)
			assert.Equal(t, "", originalTx.StellarTransactionHash.String)
			assert.Nil(t, originalTx.SentAt)
			assert.Len(t, originalTx.StatusHistory, 1)
			initialStatusHistory := originalTx.StatusHistory[0]

			updatedTx, err := txModel.UpdateStellarTransactionHashAndXDRSent(ctx, tx.ID, tc.txHash, tc.xdrSent)
			if tc.wantErrContains != "" {
				require.Error(t, err)
				assert.ErrorContains(t, err, tc.wantErrContains)
				assert.Nil(t, updatedTx)
			} else {
				// check if object has been updated correctly
				require.NoError(t, err)
				assert.True(t, updatedTx.XDRSent.Valid)
				assert.Equal(t, envelopeXDR, updatedTx.XDRSent.String)
				assert.True(t, updatedTx.StellarTransactionHash.Valid)
				assert.Equal(t, txHash, updatedTx.StellarTransactionHash.String)
				assert.NotNil(t, updatedTx.SentAt)
				assert.Equal(t, originalTx.AttemptsCount+1, updatedTx.AttemptsCount)

				// assert new status history info:
				assert.Len(t, updatedTx.StatusHistory, 2)
				newStatusHist := updatedTx.StatusHistory[1]
				assert.Equal(t, string(updatedTx.Status), newStatusHist.Status)
				assert.Equal(t, updatedTx.StellarTransactionHash.String, newStatusHist.StellarTransactionHash)
				assert.Equal(t, updatedTx.XDRSent.String, newStatusHist.XDRSent)
				assert.Empty(t, updatedTx.XDRReceived)
				wantStatusHistory := TransactionStatusHistory{initialStatusHistory, newStatusHist}

				// retrieve the transaction from the database and check if values are updated
				refreshedTx, err := txModel.Get(ctx, tx.ID)
				require.NoError(t, err)
				assert.Equal(t, updatedTx, refreshedTx)

				// make sure only the expected fields were updated:
				originalTx.XDRSent = refreshedTx.XDRSent
				originalTx.StellarTransactionHash = refreshedTx.StellarTransactionHash
				originalTx.SentAt = refreshedTx.SentAt
				originalTx.UpdatedAt = refreshedTx.UpdatedAt
				originalTx.StatusHistory = wantStatusHistory
				originalTx.AttemptsCount += 1
				assert.Equal(t, refreshedTx, originalTx)
			}
		})
	}
}

func Test_TransactionModel_UpdateStellarTransactionXDRReceived(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()
	txModel := NewTransactionModel(dbConnectionPool)

	const envelopeXDR = "AAAAAGL8HQvQkbK2HA3WVjRrKmjX00fG8sLI7m0ERwJW/AX3AAAACgAAAAAAAAABAAAAAAAAAAAAAAABAAAAAAAAAAAAAAAArqN6LeOagjxMaUP96Bzfs9e0corNZXzBWJkFoK7kvkwAAAAAO5rKAAAAAAAAAAABVvwF9wAAAEAKZ7IPj/46PuWU6ZOtyMosctNAkXRNX9WCAI5RnfRk+AyxDLoDZP/9l3NvsxQtWj9juQOuoBlFLnWu8intgxQA"
	const resultXDR = "AAAAAAAAAGQAAAAAAAAAAQAAAAAAAAAOAAAAAAAAAABw2JZZYIt4n/WXKcnDow3mbTBMPrOnldetgvGUlpTSEQAAAAA="

	testCases := []struct {
		name            string
		transaction     Transaction
		xdrReceived     string
		wantErrContains string
	}{
		{
			name:            "returns an error if XDR is empty",
			xdrReceived:     "",
			wantErrContains: "invalid XDR result: decoding Int64: decoding Hyper: xdr:DecodeHyper: EOF while decoding 8 bytes - read: '[]'",
		},
		{
			name:            "returns an error if XDR is not a valid base64 encoded",
			xdrReceived:     "not-base-64-encoded",
			wantErrContains: "invalid XDR result: decoding Int64: decoding Hyper: xdr:DecodeHyper: illegal base64 data",
		},
		{
			name:            "returns an error if XDR is not a transaction envelope",
			xdrReceived:     envelopeXDR,
			wantErrContains: "invalid XDR result: decoding TransactionResultResult: decoding TransactionResultCode: '-795757898' is not a valid TransactionResultCode enum value",
		},
		{
			name:        "🎉 successfully validate a transaction result and save it in the DB",
			xdrReceived: resultXDR,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// create a new transaction
			tx, err := txModel.Insert(ctx, Transaction{
				ExternalID:  uuid.NewString(),
				AssetCode:   "USDC",
				AssetIssuer: "GCBIRB7Q5T53H4L6P5QSI3O6LPD5MBWGM5GHE7A5NY4XT5OT4VCOEZFX",
				Amount:      1,
				Destination: "GBHNIYGWZUAVZX7KTLVSMILBXJMUACVO6XBEKIN6RW7AABDFH6S7GK2Y",
			})
			require.NoError(t, err)
			require.NotNil(t, tx)

			assert.Equal(t, false, tx.XDRReceived.Valid)
			assert.Equal(t, "", tx.XDRReceived.String)

			updatedTx, err := txModel.UpdateStellarTransactionXDRReceived(ctx, tx.ID, tc.xdrReceived)
			if tc.wantErrContains != "" {
				require.Error(t, err)
				assert.ErrorContains(t, err, tc.wantErrContains)
			} else {
				// check if object has been updated correctly
				require.NoError(t, err)
				assert.Equal(t, true, updatedTx.XDRReceived.Valid)
				assert.Equal(t, resultXDR, updatedTx.XDRReceived.String)

				// retrieve the transaction from the database and check if values are updated
				refreshedTx, err := txModel.Get(ctx, tx.ID)
				require.NoError(t, err)
				assert.Equal(t, refreshedTx, updatedTx)
			}
		})
	}
}

func Test_Transaction_validate(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()
	require.NoError(t, err)

	testCases := []struct {
		name            string
		transaction     Transaction
		wantErrContains string
	}{
		{
			name:            "validate ExternalID",
			transaction:     Transaction{},
			wantErrContains: "external ID is required",
		},
		{
			name: "validate AssetCode (min size)",
			transaction: Transaction{
				ExternalID: "123",
			},
			wantErrContains: "asset code must have between 1 and 12 characters",
		},
		{
			name: "validate AssetCode (max size)",
			transaction: Transaction{
				ExternalID: "123",
				AssetCode:  "1234567890123",
			},
			wantErrContains: "asset code must have between 1 and 12 characters",
		},
		{
			name: "validate AssetIssuer (cannot be nil)",
			transaction: Transaction{
				ExternalID: "123",
				AssetCode:  "USDC",
			},
			wantErrContains: "asset issuer is required",
		},
		{
			name: "validate AssetIssuer (not a valid public key)",
			transaction: Transaction{
				ExternalID:  "123",
				AssetCode:   "USDC",
				AssetIssuer: "invalid-issuer",
			},
			wantErrContains: `asset issuer "invalid-issuer" is not a valid ed25519 public key`,
		},
		{
			name: "validate Amount",
			transaction: Transaction{
				ExternalID:  "123",
				AssetCode:   "USDC",
				AssetIssuer: "GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5",
			},
			wantErrContains: "amount must be positive",
		},
		{
			name: "validate Destination",
			transaction: Transaction{
				ExternalID:  "123",
				AssetCode:   "USDC",
				AssetIssuer: "GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5",
				Amount:      100.0,
				Destination: "invalid-destination",
			},
			wantErrContains: `destination "invalid-destination" is not a valid ed25519 public key`,
		},
		{
			name: "🎉 successfully validate USDC transaction",
			transaction: Transaction{
				ExternalID:  "123",
				AssetCode:   "USDC",
				AssetIssuer: "GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5",
				Amount:      100.0,
				Destination: "GDUCE34WW5Z34GMCEPURYANUCUP47J6NORJLKC6GJNMDLN4ZI4PMI2MG",
			},
		},
		{
			name: "🎉 successfully validate XLM transaction",
			transaction: Transaction{
				ExternalID:  "123",
				AssetCode:   "xLm",
				Amount:      100.0,
				Destination: "GDUCE34WW5Z34GMCEPURYANUCUP47J6NORJLKC6GJNMDLN4ZI4PMI2MG",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.transaction.validate()
			if tc.wantErrContains == "" {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
				assert.ErrorContains(t, err, tc.wantErrContains)
			}
		})
	}
}

func Test_TransactionModel_GetTransactionBatchForUpdate(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()
	txModel := NewTransactionModel(dbConnectionPool)

	testCase := []struct {
		name              string
		transactionStatus TransactionStatus
		shouldBeFound     bool
		batchSize         int
		wantErrContains   string
	}{
		{
			name:              "batchSize must be >= 0",
			transactionStatus: TransactionStatusSuccess,
			batchSize:         0,
			wantErrContains:   "batch size must be greater than 0",
			shouldBeFound:     false,
		},
		{
			name:              "no transactions found (empty database)",
			transactionStatus: "",
			batchSize:         100,
			shouldBeFound:     false,
		},
		{
			name:              "no transactions found (PENDING)",
			transactionStatus: TransactionStatusPending,
			batchSize:         100,
			shouldBeFound:     false,
		},
		{
			name:              "no transactions found (PROCESSING)",
			transactionStatus: TransactionStatusProcessing,
			batchSize:         100,
			shouldBeFound:     false,
		},
		{
			name:              "🎉 transactions successfully found (SUCCESS)",
			transactionStatus: TransactionStatusSuccess,
			batchSize:         100,
			shouldBeFound:     true,
		},
		{
			name:              "🎉 transactions successfully found (ERROR)",
			transactionStatus: TransactionStatusError,
			batchSize:         100,
			shouldBeFound:     true,
		},
	}

	const txCount = 3
	for _, tc := range testCase {
		t.Run(tc.name, func(t *testing.T) {
			dbTx, err := dbConnectionPool.BeginTxx(ctx, nil)
			require.NoError(t, err)
			defer func() {
				err = dbTx.Rollback()
				require.NoError(t, err)
			}()

			var transactions []*Transaction
			if tc.transactionStatus != "" {
				// create transactions and get their IDs
				transactions = CreateTransactionFixtures(
					t,
					ctx,
					dbTx,
					txCount,
					"USDC",
					"GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5",
					"GBHNIYGWZUAVZX7KTLVSMILBXJMUACVO6XBEKIN6RW7AABDFH6S7GK2Y",
					tc.transactionStatus,
					1.2,
				)
			}
			var txIDs []string
			for _, tx := range transactions {
				txIDs = append(txIDs, tx.ID)
			}

			foundTransactions, err := txModel.GetTransactionBatchForUpdate(ctx, dbTx, tc.batchSize)
			if tc.wantErrContains == "" {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
				assert.ErrorContains(t, err, tc.wantErrContains)
			}

			var foundTxIDs []string
			for _, tx := range foundTransactions {
				foundTxIDs = append(foundTxIDs, tx.ID)
			}

			if !tc.shouldBeFound {
				assert.Equal(t, 0, len(foundTransactions))
			} else {
				assert.Equal(t, txCount, len(foundTransactions))
				assert.ElementsMatch(t, txIDs, foundTxIDs)
			}
		})
	}

	DeleteAllTransactionFixtures(t, ctx, dbConnectionPool)
}

func Test_TransactionModel_UpdateSyncedTransactions(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()
	txModel := NewTransactionModel(dbConnectionPool)

	testCase := []struct {
		name                 string
		shouldSendEmptyIDs   bool
		shouldSendInvalidIDs bool
		transactionStatus    TransactionStatus
		wantErrContains      string
	}{
		{
			name:               "rerturn an error if txIDs is empty",
			shouldSendEmptyIDs: true,
			wantErrContains:    "no transaction IDs provided",
		},
		{
			name:                 "rerturn an error if the IDs sent don't exist",
			shouldSendInvalidIDs: true,
			wantErrContains:      "expected 1 rows to be affected, got 0",
		},
		{
			name:              "rerturn an error if the IDs sent were not ready to be synched (PENDING)",
			transactionStatus: TransactionStatusPending,
			wantErrContains:   "expected 3 rows to be affected, got 0",
		},
		{
			name:              "rerturn an error if the IDs sent were not ready to be synched (PROCESSING)",
			transactionStatus: TransactionStatusProcessing,
			wantErrContains:   "expected 3 rows to be affected, got 0",
		},
		{
			name:              "🎉 successfully set the status of transactions to synched (SUCCESS)",
			transactionStatus: TransactionStatusSuccess,
		},
		{
			name:              "🎉 successfully set the status of transactions to synched (ERROR)",
			transactionStatus: TransactionStatusError,
		},
	}

	const txCount = 3
	for _, tc := range testCase {
		t.Run(tc.name, func(t *testing.T) {
			dbTx, err := dbConnectionPool.BeginTxx(ctx, nil)
			require.NoError(t, err)
			defer func() {
				err = dbTx.Rollback()
				require.NoError(t, err)
			}()

			// create transactions and get their IDs
			var txIDs []string
			if tc.shouldSendEmptyIDs {
				txIDs = []string{}
			} else if tc.shouldSendInvalidIDs {
				txIDs = []string{"invalid-id"}
			} else {
				transactions := CreateTransactionFixtures(
					t,
					ctx,
					dbTx,
					txCount,
					"USDC",
					"GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5",
					"GBHNIYGWZUAVZX7KTLVSMILBXJMUACVO6XBEKIN6RW7AABDFH6S7GK2Y",
					tc.transactionStatus,
					1.2,
				)
				for _, tx := range transactions {
					txIDs = append(txIDs, tx.ID)
				}
			}

			err = txModel.UpdateSyncedTransactions(ctx, dbTx, txIDs)

			// count the number of transactions that were synched
			var count int
			countErr := dbTx.GetContext(ctx, &count, "SELECT COUNT(*) FROM submitter_transactions WHERE synced_at IS NOT NULL")
			require.NoError(t, countErr)

			if tc.wantErrContains != "" {
				require.Error(t, err)
				assert.ErrorContains(t, err, tc.wantErrContains)
				assert.Equal(t, 0, count)
			} else {
				require.NoError(t, err)
				assert.Equal(t, txCount, count)
			}
		})
	}

	DeleteAllTransactionFixtures(t, ctx, dbConnectionPool)
}

func Test_TransactionModel_queryFilterForLockedState(t *testing.T) {
	txModel := &TransactionModel{}

	testCases := []struct {
		name         string
		locked       bool
		ledgerNumber int32
		wantFilter   string
	}{
		{
			name:         "locked to ledgerNumber=10",
			locked:       true,
			ledgerNumber: 10,
			wantFilter:   "(locked_until_ledger_number >= 10)",
		},
		{
			name:         "unlocked or expired on ledgerNumber=20",
			locked:       false,
			ledgerNumber: 20,
			wantFilter:   "(locked_until_ledger_number IS NULL OR locked_until_ledger_number < 20)",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			gotFilter := txModel.queryFilterForLockedState(tc.locked, tc.ledgerNumber)
			assert.Equal(t, tc.wantFilter, gotFilter)
		})
	}
}

func Test_TransactionModel_Lock(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()
	transactionModel := TransactionModel{DBConnectionPool: dbConnectionPool}

	const currentLedger int32 = 10
	const nextLedgerLock int32 = 20

	testCases := []struct {
		name                     string
		initialLockedAt          sql.NullTime
		initialSyncedAt          sql.NullTime
		initialStatus            TransactionStatus
		initialLockedUntilLedger sql.NullInt32
		expectedErrContains      string
	}{
		{
			name:          "🎉 successfully locks transaction without any previous lock (PENDING)",
			initialStatus: TransactionStatusPending,
		},
		{
			name:          "🎉 successfully locks transaction without any previous lock (PROCESSING)",
			initialStatus: TransactionStatusProcessing,
		},
		{
			name:                     "🎉 successfully locks transaction with lock expired",
			initialStatus:            TransactionStatusPending,
			initialLockedAt:          sql.NullTime{Time: time.Now().Add(-1 * time.Hour), Valid: true},
			initialLockedUntilLedger: sql.NullInt32{Int32: currentLedger - 1, Valid: true},
		},
		{
			name:                     "🚧 cannot be locked again if still locked",
			initialStatus:            TransactionStatusPending,
			initialLockedAt:          sql.NullTime{Time: time.Now().Add(-1 * time.Hour), Valid: true},
			initialLockedUntilLedger: sql.NullInt32{Int32: currentLedger, Valid: true},
			expectedErrContains:      ErrRecordNotFound.Error(),
		},
		{
			name:                "🚧 cannot be locked if the status is SUCCESS",
			initialStatus:       TransactionStatusSuccess,
			expectedErrContains: ErrRecordNotFound.Error(),
		},
		{
			name:                "🚧 cannot be locked if the status is ERROR",
			initialStatus:       TransactionStatusError,
			expectedErrContains: ErrRecordNotFound.Error(),
		},
		{
			name:                "🚧 cannot be locked if siced_at is not empty",
			initialStatus:       TransactionStatusPending,
			initialSyncedAt:     sql.NullTime{Time: time.Now().Add(-1 * time.Hour), Valid: true},
			expectedErrContains: ErrRecordNotFound.Error(),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tx := CreateTransactionFixture(t, ctx, dbConnectionPool, uuid.NewString(), "USDC", "GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5", "GCBIRB7Q5T53H4L6P5QSI3O6LPD5MBWGM5GHE7A5NY4XT5OT4VCOEZFX", tc.initialStatus, 1)
			q := `UPDATE submitter_transactions SET locked_at = $1, locked_until_ledger_number = $2, synced_at = $3, status = $4 WHERE id = $5`
			_, err := dbConnectionPool.ExecContext(ctx, q, tc.initialLockedAt, tc.initialLockedUntilLedger, tc.initialSyncedAt, tc.initialStatus, tx.ID)
			require.NoError(t, err)

			tx, err = transactionModel.Lock(ctx, dbConnectionPool, tx.ID, currentLedger, nextLedgerLock)

			if tc.expectedErrContains == "" {
				require.NoError(t, err)
				tx, err = transactionModel.Get(ctx, tx.ID)
				require.NoError(t, err)
				assert.NotNil(t, tx.LockedAt)
				assert.True(t, tx.LockedUntilLedgerNumber.Valid)
				assert.Equal(t, nextLedgerLock, tx.LockedUntilLedgerNumber.Int32)
				assert.Equal(t, TransactionStatusProcessing, tx.Status)

				var txRefreshed *Transaction
				txRefreshed, err = transactionModel.Get(ctx, tx.ID)
				require.NoError(t, err)
				require.Equal(t, *txRefreshed, *tx)
			} else {
				require.Error(t, err)
				assert.ErrorContains(t, err, tc.expectedErrContains)
			}

			DeleteAllTransactionFixtures(t, ctx, dbConnectionPool)
		})
	}
}

func Test_TransactionModel_Unlock(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()
	transactionModel := TransactionModel{DBConnectionPool: dbConnectionPool}

	const currentLedger int32 = 10

	testCases := []struct {
		name                     string
		initialLockedAt          sql.NullTime
		initialSyncedAt          sql.NullTime
		initialStatus            TransactionStatus
		initialLockedUntilLedger sql.NullInt32
	}{
		{
			name:          "🎉 successfully locks transaction without any previous lock",
			initialStatus: TransactionStatusPending,
		},
		{
			name:                     "🎉 successfully locks transaction with lock expired",
			initialStatus:            TransactionStatusPending,
			initialLockedAt:          sql.NullTime{Time: time.Now().Add(-1 * time.Hour), Valid: true},
			initialLockedUntilLedger: sql.NullInt32{Int32: currentLedger - 1, Valid: true},
		},
		{
			name:                     "🎉 successfully unlocks locked transaction",
			initialStatus:            TransactionStatusPending,
			initialLockedAt:          sql.NullTime{Time: time.Now().Add(-1 * time.Hour), Valid: true},
			initialLockedUntilLedger: sql.NullInt32{Int32: currentLedger, Valid: true},
		},
		{
			name:          "🎉 successfully unlocks transaction with status is SUCCESS",
			initialStatus: TransactionStatusSuccess,
		},
		{
			name:          "🎉 successfully unlocks transaction with status is ERROR",
			initialStatus: TransactionStatusError,
		},
		{
			name:            "🎉 successfully unlocks transaction with siced_at not empty",
			initialStatus:   TransactionStatusPending,
			initialSyncedAt: sql.NullTime{Time: time.Now().Add(-1 * time.Hour), Valid: true},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tx := CreateTransactionFixture(t, ctx, dbConnectionPool, uuid.NewString(), "USDC", "GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5", "GCBIRB7Q5T53H4L6P5QSI3O6LPD5MBWGM5GHE7A5NY4XT5OT4VCOEZFX", tc.initialStatus, 1)
			q := `UPDATE submitter_transactions SET locked_at = $1, locked_until_ledger_number = $2, synced_at = $3, status = $4 WHERE id = $5`
			_, err := dbConnectionPool.ExecContext(ctx, q, tc.initialLockedAt, tc.initialLockedUntilLedger, tc.initialSyncedAt, tc.initialStatus, tx.ID)
			require.NoError(t, err)

			tx, err = transactionModel.Unlock(ctx, dbConnectionPool, tx.ID)
			require.NoError(t, err)

			tx, err = transactionModel.Get(ctx, tx.ID)
			require.NoError(t, err)
			assert.Nil(t, tx.LockedAt)
			assert.False(t, tx.LockedUntilLedgerNumber.Valid)

			var txRefreshed *Transaction
			txRefreshed, err = transactionModel.Get(ctx, tx.ID)
			require.NoError(t, err)
			require.Equal(t, *txRefreshed, *tx)

			DeleteAllTransactionFixtures(t, ctx, dbConnectionPool)
		})
	}
}

func Test_TransactionModel_PrepareTransactionForReprocessing(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()
	transactionModel := NewTransactionModel(dbConnectionPool)

	testCases := []struct {
		name      string
		status    TransactionStatus
		synchedAt sql.NullTime
		wantError error
	}{
		{
			name:      "cannot mark for reporcessing if the status is SUCCESS",
			status:    TransactionStatusSuccess,
			wantError: ErrRecordNotFound,
		},
		{
			name:      "cannot mark for reporcessing if the status is ERROR",
			status:    TransactionStatusError,
			wantError: ErrRecordNotFound,
		},
		{
			name:      "cannot mark for reporcessing if synced_at is not empty",
			status:    TransactionStatusProcessing,
			synchedAt: sql.NullTime{Time: time.Now().Add(-1 * time.Hour), Valid: true},
			wantError: ErrRecordNotFound,
		},
		{
			name:   "🎉 successfully mark as processing if tx is PENDING and not synced transaction",
			status: TransactionStatusPending,
		},
		{
			name:   "🎉 successfully mark as processing if tx is PROCESSING and not synced transaction",
			status: TransactionStatusProcessing,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			defer DeleteAllTransactionFixtures(t, ctx, dbConnectionPool)
			const lockedUntilLedger = 2

			// create and prepare the transaction:
			tx := CreateTransactionFixture(t, ctx, dbConnectionPool, uuid.NewString(), "USDC", "GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5", "GCBIRB7Q5T53H4L6P5QSI3O6LPD5MBWGM5GHE7A5NY4XT5OT4VCOEZFX", tc.status, 1)
			q := `UPDATE submitter_transactions SET status = $1, synced_at = $2, locked_at = NOW(), locked_until_ledger_number=$3 WHERE id = $4`
			_, err = dbConnectionPool.ExecContext(ctx, q, tc.status, tc.synchedAt, lockedUntilLedger, tx.ID)
			require.NoError(t, err)

			// mark the transaction for reprocessing:
			updatedTx, err := transactionModel.PrepareTransactionForReprocessing(ctx, dbConnectionPool, tx.ID)

			// check the result:
			if tc.wantError != nil {
				require.Error(t, err)
				assert.Equal(t, tc.wantError, err)
				assert.Nil(t, updatedTx)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tc.status, updatedTx.Status)
				assert.Nil(t, updatedTx.SyncedAt)

				// Check if only the expected fields were updated:
				assert.Nil(t, updatedTx.LockedAt)
				assert.False(t, updatedTx.LockedUntilLedgerNumber.Valid)
				assert.False(t, updatedTx.StellarTransactionHash.Valid)
				assert.False(t, updatedTx.XDRSent.Valid)
				assert.False(t, updatedTx.XDRReceived.Valid)

				// Check if the returned transaction is exactly the same as a fresh one from the DB:
				refreshedTx, err := transactionModel.Get(ctx, tx.ID)
				require.NoError(t, err)
				require.Equal(t, refreshedTx, updatedTx)
			}
		})
	}
}
