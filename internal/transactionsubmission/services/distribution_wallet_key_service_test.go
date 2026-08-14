package services

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/db/dbtest"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/store"
)

// Test_DistributionWalletKeyService_isolation proves the secret-material isolation
// acceptance criteria: per-wallet envelopes are fully independent for storage, rotation,
// and failure.
func Test_DistributionWalletKeyService_isolation(t *testing.T) {
	dbt := dbtest.OpenWithTSSMigrationsOnly(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()
	const kek = "host-distribution-kek"

	svc, err := NewDistributionWalletKeyService(dbConnectionPool, kek)
	require.NoError(t, err)
	keyModel := store.NewDistributionWalletKeyModel(dbConnectionPool)

	const (
		pubA  = "GWALLETAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
		seedA = "SSEEDAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
		pubB  = "GWALLETBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBB"
		seedB = "SSEEDBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBB"
	)

	require.NoError(t, svc.StoreKey(ctx, pubA, seedA))
	require.NoError(t, svc.StoreKey(ctx, pubB, seedB))

	t.Run("both wallets round-trip independently", func(t *testing.T) {
		gotA, gErr := svc.GetPrivateKey(ctx, pubA)
		require.NoError(t, gErr)
		assert.Equal(t, seedA, gotA)

		gotB, gErr := svc.GetPrivateKey(ctx, pubB)
		require.NoError(t, gErr)
		assert.Equal(t, seedB, gotB)
	})

	t.Run("ciphertexts are independent even for identical plaintext", func(t *testing.T) {
		const pubC = "GWALLETCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCC"
		require.NoError(t, svc.StoreKey(ctx, pubC, seedA)) // same seed as wallet A

		rowA, gErr := keyModel.Get(ctx, pubA)
		require.NoError(t, gErr)
		rowC, gErr := keyModel.Get(ctx, pubC)
		require.NoError(t, gErr)

		assert.NotEqual(t, rowA.EncryptedPrivateKey, rowC.EncryptedPrivateKey)
		assert.NotEqual(t, rowA.EncryptedDEK, rowC.EncryptedDEK)
	})

	t.Run("rotating wallet A never touches wallet B ciphertext", func(t *testing.T) {
		beforeA, gErr := keyModel.Get(ctx, pubA)
		require.NoError(t, gErr)
		beforeB, gErr := keyModel.Get(ctx, pubB)
		require.NoError(t, gErr)

		require.NoError(t, svc.RotateWalletDEK(ctx, pubA))

		afterA, gErr := keyModel.Get(ctx, pubA)
		require.NoError(t, gErr)
		afterB, gErr := keyModel.Get(ctx, pubB)
		require.NoError(t, gErr)

		// A: fully re-encrypted, still decrypts to the same seed.
		assert.NotEqual(t, beforeA.EncryptedPrivateKey, afterA.EncryptedPrivateKey)
		assert.NotEqual(t, beforeA.EncryptedDEK, afterA.EncryptedDEK)
		gotA, gErr := svc.GetPrivateKey(ctx, pubA)
		require.NoError(t, gErr)
		assert.Equal(t, seedA, gotA)

		// B: byte-identical ciphertext, untouched timestamps, still decrypts.
		assert.Equal(t, beforeB.EncryptedPrivateKey, afterB.EncryptedPrivateKey)
		assert.Equal(t, beforeB.EncryptedDEK, afterB.EncryptedDEK)
		assert.Equal(t, beforeB.UpdatedAt, afterB.UpdatedAt)
		gotB, gErr := svc.GetPrivateKey(ctx, pubB)
		require.NoError(t, gErr)
		assert.Equal(t, seedB, gotB)
	})

	t.Run("corrupted wallet fails closed alone", func(t *testing.T) {
		// Corrupt A's wrapped DEK with another envelope's DEK (valid base64, wrong key).
		_, foreignDEK, eErr := svc.encrypter.EncryptWithNewDEK("decoy", kek)
		require.NoError(t, eErr)
		require.NoError(t, keyModel.UpdateCiphertext(ctx, pubA, "Y29ycnVwdGVk", foreignDEK))

		_, gErr := svc.GetPrivateKey(ctx, pubA)
		require.Error(t, gErr, "wallet A must fail closed")

		gotB, gErr := svc.GetPrivateKey(ctx, pubB)
		require.NoError(t, gErr, "wallet B must be unaffected by A's corruption")
		assert.Equal(t, seedB, gotB)
	})

	t.Run("missing wallet key fails closed with ErrRecordNotFound", func(t *testing.T) {
		_, gErr := svc.GetPrivateKey(ctx, "GMISSINGWALLETXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXX")
		require.Error(t, gErr)
		assert.ErrorIs(t, gErr, store.ErrRecordNotFound)
	})

	t.Run("service constructor validations", func(t *testing.T) {
		_, cErr := NewDistributionWalletKeyService(nil, kek)
		require.Error(t, cErr)
		_, cErr = NewDistributionWalletKeyService(dbConnectionPool, "")
		require.Error(t, cErr)
	})
}
