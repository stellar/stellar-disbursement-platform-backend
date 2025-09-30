package data

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/db/dbtest"
)

func Test_SponsoredTransactionColumnNames(t *testing.T) {
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
				"account",
				"operation_xdr",
				"created_at",
				"updated_at",
				"status",
				`COALESCE(transaction_hash, '') AS "transaction_hash"`,
			}, ", "),
		},
		{
			tableReference: "st",
			resultAlias:    "",
			expected: strings.Join([]string{
				"st.id",
				"st.account",
				"st.operation_xdr",
				"st.created_at",
				"st.updated_at",
				"st.status",
				`COALESCE(st.transaction_hash, '') AS "transaction_hash"`,
			}, ", "),
		},
		{
			tableReference: "st",
			resultAlias:    "sponsored_transactions",
			expected: strings.Join([]string{
				`st.id AS "sponsored_transactions.id"`,
				`st.account AS "sponsored_transactions.account"`,
				`st.operation_xdr AS "sponsored_transactions.operation_xdr"`,
				`st.created_at AS "sponsored_transactions.created_at"`,
				`st.updated_at AS "sponsored_transactions.updated_at"`,
				`st.status AS "sponsored_transactions.status"`,
				`COALESCE(st.transaction_hash, '') AS "sponsored_transactions.transaction_hash"`,
			}, ", "),
		},
	}

	for _, tc := range testCases {
		t.Run(testCaseNameForScanText(t, tc.tableReference, tc.resultAlias), func(t *testing.T) {
			actual := SponsoredTransactionColumnNames(tc.tableReference, tc.resultAlias)
			assert.Equal(t, tc.expected, actual)
		})
	}
}

func Test_SponsoredTransactionStatus_Validate(t *testing.T) {
	testCases := []struct {
		status    SponsoredTransactionStatus
		expectErr bool
	}{
		{PendingSponsoredTransactionStatus, false},
		{ProcessingSponsoredTransactionStatus, false},
		{SuccessSponsoredTransactionStatus, false},
		{FailedSponsoredTransactionStatus, false},
		{SponsoredTransactionStatus("INVALID"), true},
		{SponsoredTransactionStatus(""), true},
		{SponsoredTransactionStatus("pending"), false},
		{SponsoredTransactionStatus("SUCCESS"), false},
	}

	for _, tc := range testCases {
		t.Run(string(tc.status), func(t *testing.T) {
			err := tc.status.Validate()
			if tc.expectErr {
				require.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func Test_SponsoredTransactionInsert_Validate(t *testing.T) {
	t.Run("returns error if id is empty", func(t *testing.T) {
		insert := SponsoredTransactionInsert{
			Account:      "CDLZFC3SYJYDZT7K67VZ75HPJVIEUVNIXF47ZG2FB2RMQQAUHKENYZCH",
			OperationXDR: "valid-xdr",
			Status:       PendingSponsoredTransactionStatus,
		}
		err := insert.Validate()
		require.Error(t, err)
		assert.EqualError(t, err, "id cannot be empty")
	})

	t.Run("returns error if account is empty", func(t *testing.T) {
		insert := SponsoredTransactionInsert{
			ID:           "test-id-123",
			OperationXDR: "valid-xdr",
			Status:       PendingSponsoredTransactionStatus,
		}
		err := insert.Validate()
		require.Error(t, err)
		assert.EqualError(t, err, "account cannot be empty")
	})

	t.Run("returns error if operation XDR is empty", func(t *testing.T) {
		insert := SponsoredTransactionInsert{
			ID:      "test-id-123",
			Account: "CDLZFC3SYJYDZT7K67VZ75HPJVIEUVNIXF47ZG2FB2RMQQAUHKENYZCH",
			Status:  PendingSponsoredTransactionStatus,
		}
		err := insert.Validate()
		require.Error(t, err)
		assert.EqualError(t, err, "operation XDR cannot be empty")
	})

	t.Run("returns error for invalid status", func(t *testing.T) {
		insert := SponsoredTransactionInsert{
			ID:           "test-id-123",
			Account:      "CDLZFC3SYJYDZT7K67VZ75HPJVIEUVNIXF47ZG2FB2RMQQAUHKENYZCH",
			OperationXDR: "valid-xdr",
			Status:       SponsoredTransactionStatus("INVALID"),
		}
		err := insert.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "validating status")
	})

	t.Run("validates a full valid insert", func(t *testing.T) {
		insert := SponsoredTransactionInsert{
			ID:           "test-id-123",
			Account:      "CDLZFC3SYJYDZT7K67VZ75HPJVIEUVNIXF47ZG2FB2RMQQAUHKENYZCH",
			OperationXDR: "dGVzdA==",
			Status:       PendingSponsoredTransactionStatus,
		}
		err := insert.Validate()
		require.NoError(t, err)
	})
}

func Test_SponsoredTransactionUpdate_Validate(t *testing.T) {
	t.Run("returns error if no values provided", func(t *testing.T) {
		update := SponsoredTransactionUpdate{}
		err := update.Validate()
		require.Error(t, err)
		assert.EqualError(t, err, "no values provided to update sponsored transaction")
	})

	t.Run("validates Status", func(t *testing.T) {
		update := SponsoredTransactionUpdate{Status: SponsoredTransactionStatus("INVALID")}
		err := update.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "validating status")

		update = SponsoredTransactionUpdate{Status: SuccessSponsoredTransactionStatus}
		err = update.Validate()
		require.NoError(t, err)
	})

	t.Run("validates TransactionHash", func(t *testing.T) {
		update := SponsoredTransactionUpdate{TransactionHash: "too-short"}
		err := update.Validate()
		require.Error(t, err)
		assert.EqualError(t, err, "transaction hash must be 64 characters, got 9")

		update = SponsoredTransactionUpdate{
			TransactionHash: "1234567890123456789012345678901234567890123456789012345678901234",
		}
		err = update.Validate()
		require.NoError(t, err)
	})

	t.Run("validates a full valid update", func(t *testing.T) {
		update := SponsoredTransactionUpdate{
			Status:          SuccessSponsoredTransactionStatus,
			TransactionHash: "1234567890123456789012345678901234567890123456789012345678901234",
		}
		err := update.Validate()
		require.NoError(t, err)
	})
}

func Test_SponsoredTransactionModel_Insert(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, outerErr := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, outerErr)
	defer dbConnectionPool.Close()

	ctx := context.Background()
	model := &SponsoredTransactionModel{}

	DeleteAllSponsoredTransactionsFixtures(t, ctx, dbConnectionPool)
	defer DeleteAllSponsoredTransactionsFixtures(t, ctx, dbConnectionPool)

	t.Run("returns sponsored transaction when successfully inserted", func(t *testing.T) {
		account := "CDLZFC3SYJYDZT7K67VZ75HPJVIEUVNIXF47ZG2FB2RMQQAUHKENYZCH"
		operationXDR := "dGVzdA=="
		expectedID := "test-transaction-id-123"

		insert := SponsoredTransactionInsert{
			ID:           expectedID,
			Account:      account,
			OperationXDR: operationXDR,
			Status:       PendingSponsoredTransactionStatus,
		}
		transaction, err := model.Insert(ctx, dbConnectionPool, insert)
		require.NoError(t, err)
		require.NotNil(t, transaction)

		assert.Equal(t, expectedID, transaction.ID)
		assert.Equal(t, account, transaction.Account)
		assert.Equal(t, operationXDR, transaction.OperationXDR)
		assert.Equal(t, string(PendingSponsoredTransactionStatus), transaction.Status)
		assert.NotNil(t, transaction.CreatedAt)
		assert.NotNil(t, transaction.UpdatedAt)
		assert.Empty(t, transaction.TransactionHash)
	})

	t.Run("returns error when insert validation fails", func(t *testing.T) {
		invalidInsert := SponsoredTransactionInsert{
			ID:           "",
			Account:      "CDLZFC3SYJYDZT7K67VZ75HPJVIEUVNIXF47ZG2FB2RMQQAUHKENYZCH",
			OperationXDR: "valid-xdr",
			Status:       PendingSponsoredTransactionStatus,
		}
		transaction, err := model.Insert(ctx, dbConnectionPool, invalidInsert)
		require.Error(t, err)
		assert.Nil(t, transaction)
		assert.Contains(t, err.Error(), "validating sponsored transaction insert")
	})
}

func Test_SponsoredTransactionModel_GetByID(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, outerErr := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, outerErr)
	defer dbConnectionPool.Close()

	ctx := context.Background()
	model := &SponsoredTransactionModel{}

	DeleteAllSponsoredTransactionsFixtures(t, ctx, dbConnectionPool)
	defer DeleteAllSponsoredTransactionsFixtures(t, ctx, dbConnectionPool)

	t.Run("returns error when transaction does not exist", func(t *testing.T) {
		transaction, err := model.GetByID(ctx, dbConnectionPool, "non-existent-id")
		require.Error(t, err)
		assert.Equal(t, ErrRecordNotFound, err)
		assert.Nil(t, transaction)
	})

	t.Run("returns error when ID is empty", func(t *testing.T) {
		transaction, err := model.GetByID(ctx, dbConnectionPool, "")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "transaction ID is required")
		assert.Nil(t, transaction)
	})

	t.Run("returns transaction when it exists", func(t *testing.T) {
		expectedAccount := "CDLZFC3SYJYDZT7K67VZ75HPJVIEUVNIXF47ZG2FB2RMQQAUHKENYZCH"
		expectedOperationXDR := "AAAAAQAAAAC7JAuE3XvquOnbsgv2SRztjuk4RoBVefQ0rlrFMMQvfAAAAAEAAAAA"
		createdTransaction := CreateSponsoredTransactionFixture(t, ctx, dbConnectionPool, expectedAccount, expectedOperationXDR)

		transaction, err := model.GetByID(ctx, dbConnectionPool, createdTransaction.ID)
		require.NoError(t, err)
		require.NotNil(t, transaction)

		assert.Equal(t, createdTransaction.ID, transaction.ID)
		assert.Equal(t, expectedAccount, transaction.Account)
		assert.Equal(t, expectedOperationXDR, transaction.OperationXDR)
		assert.Equal(t, string(PendingSponsoredTransactionStatus), transaction.Status)
		assert.NotNil(t, transaction.CreatedAt)
		assert.NotNil(t, transaction.UpdatedAt)
	})
}

func Test_SponsoredTransactionModel_Update(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, outerErr := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, outerErr)
	defer dbConnectionPool.Close()

	ctx := context.Background()
	model := &SponsoredTransactionModel{}

	DeleteAllSponsoredTransactionsFixtures(t, ctx, dbConnectionPool)
	defer DeleteAllSponsoredTransactionsFixtures(t, ctx, dbConnectionPool)

	t.Run("returns error when ID is empty", func(t *testing.T) {
		update := SponsoredTransactionUpdate{
			Status: SuccessSponsoredTransactionStatus,
		}
		err := model.Update(ctx, dbConnectionPool, "", update)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "transaction ID is required")
	})

	t.Run("returns error when no values provided to update", func(t *testing.T) {
		transaction := CreateSponsoredTransactionFixture(t, ctx, dbConnectionPool,
			"CDLZFC3SYJYDZT7K67VZ75HPJVIEUVNIXF47ZG2FB2RMQQAUHKENYZCH",
			"AAAAAQAAAAC7JAuE3XvquOnbsgv2SRztjuk4RoBVefQ0rlrFMMQvfAAAAAEAAAAA")

		update := SponsoredTransactionUpdate{}
		err := model.Update(ctx, dbConnectionPool, transaction.ID, update)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no values provided")
	})

	t.Run("returns error when update validation fails", func(t *testing.T) {
		transaction := CreateSponsoredTransactionFixture(t, ctx, dbConnectionPool,
			"CDLZFC3SYJYDZT7K67VZ75HPJVIEUVNIXF47ZG2FB2RMQQAUHKENYZCH",
			"AAAAAQAAAAC7JAuE3XvquOnbsgv2SRztjuk4RoBVefQ0rlrFMMQvfAAAAAEAAAAA")

		update := SponsoredTransactionUpdate{
			TransactionHash: "invalid-hash",
		}
		err := model.Update(ctx, dbConnectionPool, transaction.ID, update)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "validating sponsored transaction update")
	})

	t.Run("returns ErrRecordNotFound when transaction does not exist", func(t *testing.T) {
		update := SponsoredTransactionUpdate{
			Status: SuccessSponsoredTransactionStatus,
		}
		err := model.Update(ctx, dbConnectionPool, "non-existent-id", update)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrRecordNotFound)
	})

	t.Run("successfully updates both status and transaction hash", func(t *testing.T) {
		transaction := CreateSponsoredTransactionFixture(t, ctx, dbConnectionPool,
			"CDLZFC3SYJYDZT7K67VZ75HPJVIEUVNIXF47ZG2FB2RMQQAUHKENYZCH",
			"AAAAAQAAAAC7JAuE3XvquOnbsgv2SRztjuk4RoBVefQ0rlrFMMQvfAAAAAEAAAAA")

		expectedHash := "1234567890123456789012345678901234567890123456789012345678901234"
		update := SponsoredTransactionUpdate{
			Status:          SuccessSponsoredTransactionStatus,
			TransactionHash: expectedHash,
		}
		err := model.Update(ctx, dbConnectionPool, transaction.ID, update)
		require.NoError(t, err)

		updated, err := model.GetByID(ctx, dbConnectionPool, transaction.ID)
		require.NoError(t, err)
		assert.Equal(t, string(SuccessSponsoredTransactionStatus), updated.Status)
		assert.Equal(t, expectedHash, updated.TransactionHash)
		assert.NotNil(t, updated.UpdatedAt)
	})
}
