package store

import (
	"context"
	"crypto/rand"
	"math/big"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"github.com/stellar/go/keypair"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/db"
)

// CreateTransactionFixtures creates count number submitter transactions
func CreateTransactionFixtures(t *testing.T,
	ctx context.Context,
	sqlExec db.SQLExecuter,
	count int,
	code, issuer, destination string,
	status TransactionStatus,
	amount float64,
) []*Transaction {
	var txs []*Transaction
	for i := 0; i < count; i++ {
		tx := CreateTransactionFixture(t, ctx, sqlExec, uuid.NewString(), code, issuer, destination, status, amount)
		txs = append(txs, tx)
	}

	return txs
}

// CreateTransactionFixture creates a submitter transaction in the database
func CreateTransactionFixture(
	t *testing.T,
	ctx context.Context,
	sqlExec db.SQLExecuter,
	externalID, assetCode, assetIssuer, destinationAddress string,
	status TransactionStatus,
	amount float64,
) *Transaction {
	if assetIssuer == "" {
		assetIssuer = keypair.MustRandom().Address()
	}

	if destinationAddress == "" {
		destinationAddress = keypair.MustRandom().Address()
	}

	completedAt := pq.NullTime{}
	if status == TransactionStatusSuccess || status == TransactionStatusError {
		timeElapsed, _ := rand.Int(rand.Reader, big.NewInt(time.Now().Unix()))
		randomCompletedAt := time.Unix(timeElapsed.Int64(), 0)
		completedAt = pq.NullTime{Time: randomCompletedAt, Valid: true}
	}

	const query = `
		INSERT INTO submitter_transactions
			(external_id, status, asset_code, asset_issuer, amount, destination, completed_at, started_at)
		VALUES
			($1, $2, $3, $4, $5, $6, $7, NOW())
		RETURNING
			*
	`

	tx := Transaction{}
	err := sqlExec.GetContext(ctx, &tx, query, externalID, string(status), assetCode, assetIssuer, amount, destinationAddress, completedAt)
	require.NoError(t, err)

	return &tx
}

// DeleteAllTransactionFixtures deletes all submitter transactions in the database
func DeleteAllTransactionFixtures(t *testing.T, ctx context.Context, sqlExec db.SQLExecuter) {
	const query = "DELETE FROM submitter_transactions"
	_, err := sqlExec.ExecContext(ctx, query)
	require.NoError(t, err)
}

// CreateChannelAccountFixtures craetes count number of channel accounts
func CreateChannelAccountFixtures(t *testing.T, ctx context.Context, dbConnectionPool db.DBConnectionPool, count int) []*ChannelAccount {
	caModel := ChannelAccountModel{DBConnectionPool: dbConnectionPool}
	for i := 0; i < count; i++ {
		generatedKeypair := keypair.MustRandom()
		err := caModel.Insert(ctx, dbConnectionPool, generatedKeypair.Address(), generatedKeypair.Seed())
		require.NoError(t, err)
	}

	channelAccounts, err := caModel.GetAll(ctx, dbConnectionPool, 0, count)
	require.NoError(t, err)

	return channelAccounts
}

func DeleteAllFromChannelAccounts(t *testing.T, ctx context.Context, sqlExec db.SQLExecuter) {
	query := `DELETE FROM channel_accounts`
	_, err := sqlExec.ExecContext(ctx, query)
	require.NoError(t, err)
}
