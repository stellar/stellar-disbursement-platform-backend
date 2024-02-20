package store

import (
	"context"
	"crypto/rand"
	"math/big"
	"testing"
	"time"

	"github.com/lib/pq"
	"github.com/stellar/go/keypair"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/utils"
)

// CreateTransactionFixtures creates count number submitter transactions
func CreateTransactionFixturesNew(t *testing.T,
	ctx context.Context,
	sqlExec db.SQLExecuter,
	count int,
	txFixture TransactionFixture,
) []*Transaction {
	var txs []*Transaction
	for i := 0; i < count; i++ {
		txFixtureCopy := txFixture
		txFixtureCopy.ExternalID = keypair.MustRandom().Address()
		tx := CreateTransactionFixtureNew(t, ctx, sqlExec, txFixtureCopy)
		txs = append(txs, tx)
	}

	return txs
}

type TransactionFixture struct {
	ExternalID          string
	AssetCode           string
	AssetIssuer         string
	DestinationAddress  string
	Status              TransactionStatus
	Amount              float64
	TenantID            string
	DistributionAccount string
}

// CreateTransactionFixture creates a submitter transaction in the database
func CreateTransactionFixtureNew(
	t *testing.T,
	ctx context.Context,
	sqlExec db.SQLExecuter,
	txFixture TransactionFixture,
) *Transaction {
	if txFixture.AssetIssuer == "" {
		txFixture.AssetIssuer = keypair.MustRandom().Address()
	}

	if txFixture.DestinationAddress == "" {
		txFixture.DestinationAddress = keypair.MustRandom().Address()
	}

	completedAt := pq.NullTime{}
	if txFixture.Status == TransactionStatusSuccess || txFixture.Status == TransactionStatusError {
		timeElapsed, _ := rand.Int(rand.Reader, big.NewInt(time.Now().Unix()))
		randomCompletedAt := time.Unix(timeElapsed.Int64(), 0)
		completedAt = pq.NullTime{Time: randomCompletedAt, Valid: true}
	}

	const query = `
		INSERT INTO submitter_transactions
			(external_id, status, asset_code, asset_issuer, amount, destination, tenant_id, completed_at, started_at)
		VALUES
			($1, $2, $3, $4, $5, $6, $7, $8, NOW())
		RETURNING
			*
	`

	tx := Transaction{}
	err := sqlExec.GetContext(ctx, &tx, query,
		txFixture.ExternalID,
		string(txFixture.Status),
		txFixture.AssetCode,
		txFixture.AssetIssuer,
		txFixture.Amount,
		txFixture.DestinationAddress,
		txFixture.TenantID,
		completedAt,
	)
	require.NoError(t, err)

	return &tx
}

// DeleteAllTransactionFixtures deletes all submitter transactions in the database
func DeleteAllTransactionFixtures(t *testing.T, ctx context.Context, sqlExec db.SQLExecuter) {
	const query = "DELETE FROM submitter_transactions"
	_, err := sqlExec.ExecContext(ctx, query)
	require.NoError(t, err)
}

// CreateChannelAccountFixtures creates 'count' number of channel accounts and store them in the DB, returning the
// channel accounts.
func CreateChannelAccountFixtures(t *testing.T, ctx context.Context, dbConnectionPool db.DBConnectionPool, count int) []*ChannelAccount {
	t.Helper()

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

// CreateChannelAccountFixturesEncrypted creates 'count' number of channel accounts, and store them in the DB with the
// private keys encrypted, returning the channel accounts.
func CreateChannelAccountFixturesEncrypted(
	t *testing.T,
	ctx context.Context,
	dbConnectionPool db.DBConnectionPool,
	encrypter utils.PrivateKeyEncrypter,
	encryptionPassphrase string,
	count int,
) []*ChannelAccount {
	caModel := ChannelAccountModel{DBConnectionPool: dbConnectionPool}
	for i := 0; i < count; i++ {
		kp := keypair.MustRandom()
		publicKey := kp.Address()
		privateKey := kp.Seed()
		encryptedPrivateKey, err := encrypter.Encrypt(privateKey, encryptionPassphrase)
		require.NoError(t, err)

		err = caModel.Insert(ctx, dbConnectionPool, publicKey, encryptedPrivateKey)
		require.NoError(t, err)
	}

	channelAccounts, err := caModel.GetAll(ctx, dbConnectionPool, 0, count)
	require.NoError(t, err)

	return channelAccounts
}

// CreateChannelAccountFixturesEncryptedKPs creates 'count' number of channel accounts, and store them in the DB with
// the private keys encrypted, returning the Keypairs.
func CreateChannelAccountFixturesEncryptedKPs(
	t *testing.T,
	ctx context.Context,
	dbConnectionPool db.DBConnectionPool,
	encrypter utils.PrivateKeyEncrypter,
	encryptionPassphrase string,
	count int,
) []*keypair.Full {
	channelAccounts := CreateChannelAccountFixturesEncrypted(t, ctx, dbConnectionPool, encrypter, encryptionPassphrase, count)
	var kps []*keypair.Full
	for _, ca := range channelAccounts {
		encryptedPrivateKey := ca.PrivateKey
		privateKey, err := encrypter.Decrypt(encryptedPrivateKey, encryptionPassphrase)
		require.NoError(t, err)

		kp, err := keypair.ParseFull(privateKey)
		require.NoError(t, err)
		kps = append(kps, kp)
	}

	return kps
}

func DeleteAllFromChannelAccounts(t *testing.T, ctx context.Context, sqlExec db.SQLExecuter) {
	query := `DELETE FROM channel_accounts`
	_, err := sqlExec.ExecContext(ctx, query)
	require.NoError(t, err)
}

// CreateStellarSignatoryFixturesEncrypted creates 'count' number of stellar signatories, and store them in the DB with
// the private keys encrypted, returning the stellar signatories.
func CreateStellarSignatoryFixturesEncrypted(
	t *testing.T,
	ctx context.Context,
	dbConnectionPool db.DBConnectionPool,
	encrypter utils.PrivateKeyEncrypter,
	encryptionPassphrase string,
	count int,
) []*StellarSignatory {
	t.Helper()

	ssModel := NewStellarSignatoryModel(dbConnectionPool)
	stellarSignatories := make([]*StellarSignatory, count)
	publicKeys := make([]string, count)
	for i := 0; i < count; i++ {
		kp := keypair.MustRandom()
		publicKey := kp.Address()
		privateKey := kp.Seed()
		encryptedPrivateKey, err := encrypter.Encrypt(privateKey, encryptionPassphrase)
		require.NoError(t, err)

		stellarSignatories[i] = &StellarSignatory{
			PublicKey:           publicKey,
			EncryptedPrivateKey: encryptedPrivateKey,
		}
		publicKeys[i] = publicKey
	}

	err := ssModel.BatchInsert(ctx, stellarSignatories)
	require.NoError(t, err)

	err = dbConnectionPool.SelectContext(ctx, &stellarSignatories, "SELECT * FROM stellar_signatories WHERE public_key = ANY($1)", pq.Array(publicKeys))
	require.NoError(t, err)

	return stellarSignatories
}

// CreateStellarSignatoryFixturesEncryptedKPs creates 'count' number of stellar signatories, and store them in the DB
// with the private keys encrypted, returning the Keypairs.
func CreateStellarSignatoryFixturesEncryptedKPs(
	t *testing.T,
	ctx context.Context,
	dbConnectionPool db.DBConnectionPool,
	encrypter utils.PrivateKeyEncrypter,
	encryptionPassphrase string,
	count int,
) []*keypair.Full {
	t.Helper()

	stellarSignatories := CreateStellarSignatoryFixturesEncrypted(t, ctx, dbConnectionPool, encrypter, encryptionPassphrase, count)
	var kps []*keypair.Full
	for _, ss := range stellarSignatories {
		privateKey, err := encrypter.Decrypt(ss.EncryptedPrivateKey, encryptionPassphrase)
		require.NoError(t, err)

		kp, err := keypair.ParseFull(privateKey)
		require.NoError(t, err)
		kps = append(kps, kp)
	}

	return kps
}

func DeleteAllFromStellarSignatories(t *testing.T, ctx context.Context, sqlExec db.SQLExecuter) {
	t.Helper()

	_, err := sqlExec.ExecContext(ctx, "DELETE FROM stellar_signatories")
	require.NoError(t, err)
}
