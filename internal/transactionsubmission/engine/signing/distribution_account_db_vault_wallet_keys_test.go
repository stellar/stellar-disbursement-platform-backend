package signing

import (
	"context"
	"testing"

	"github.com/stellar/go-stellar-sdk/keypair"
	"github.com/stellar/go-stellar-sdk/network"
	"github.com/stellar/go-stellar-sdk/txnbuild"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/db/dbtest"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/store"
	sdpUtils "github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
)

// Test_DistributionAccountDBVaultSignatureClient_walletKeysPrecedence proves the multi-wallet
// signing path: per-wallet envelope-encrypted keys take precedence over the legacy vault, the
// vault remains the fallback for pre-multi-wallet accounts, and a corrupted wallet key fails
// closed without falling through to the vault.
func Test_DistributionAccountDBVaultSignatureClient_walletKeysPrecedence(t *testing.T) {
	dbt := dbtest.OpenWithTSSMigrationsOnly(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()
	encrypterPass := keypair.MustRandom().Seed()
	encrypter := &sdpUtils.DefaultPrivateKeyEncrypter{}
	envelope := sdpUtils.EnvelopeEncrypter{}
	walletKeyModel := store.NewDistributionWalletKeyModel(dbConnectionPool)

	// Wallet account: real seed in distribution_wallet_keys (envelope), DECOY seed in vault.
	walletKP := keypair.MustRandom()
	ciphertext, encryptedDEK, err := envelope.EncryptWithNewDEK(walletKP.Seed(), encrypterPass)
	require.NoError(t, err)
	require.NoError(t, walletKeyModel.Insert(ctx, store.DistributionWalletKey{
		PublicKey:           walletKP.Address(),
		EncryptedPrivateKey: ciphertext,
		EncryptedDEK:        encryptedDEK,
	}))

	decoySeed := keypair.MustRandom().Seed()
	encryptedDecoy, err := encrypter.Encrypt(decoySeed, encrypterPass)
	require.NoError(t, err)
	require.NoError(t, store.NewDBVaultModel(dbConnectionPool).BatchInsert(ctx, []*store.DBVaultEntry{
		{PublicKey: walletKP.Address(), EncryptedPrivateKey: encryptedDecoy},
	}))

	// Legacy account: seed only in the vault.
	legacyAccounts := store.CreateDBVaultFixturesEncryptedKPs(t, ctx, dbConnectionPool, encrypter, encrypterPass, 1)
	legacyKP := legacyAccounts[0]

	sigClient, err := NewDistributionAccountDBVaultSignatureClient(DistributionAccountDBVaultSignatureClientOptions{
		NetworkPassphrase:    network.TestNetworkPassphrase,
		DBConnectionPool:     dbConnectionPool,
		EncryptionPassphrase: encrypterPass,
		Encrypter:            encrypter,
	})
	require.NoError(t, err)

	newTx := func() *txnbuild.Transaction {
		sourceAccount := txnbuild.NewSimpleAccount(walletKP.Address(), 9605939170639897)
		tx, txErr := txnbuild.NewTransaction(txnbuild.TransactionParams{
			SourceAccount:        &sourceAccount,
			IncrementSequenceNum: true,
			Operations: []txnbuild.Operation{&txnbuild.Payment{
				Destination:   "GCCOBXW2XQNUSL467IEILE6MMCNRR66SSVL4YQADUNYYNUVREF3FIV2Z",
				Amount:        "10",
				Asset:         txnbuild.NativeAsset{},
				SourceAccount: legacyKP.Address(),
			}},
			BaseFee:       txnbuild.MinBaseFee,
			Preconditions: txnbuild.Preconditions{TimeBounds: txnbuild.NewTimeout(60)},
		})
		require.NoError(t, txErr)
		return tx
	}

	t.Run("wallet keys take precedence over vault; vault still serves legacy accounts", func(t *testing.T) {
		tx := newTx()
		want, sErr := tx.Sign(network.TestNetworkPassphrase, walletKP, legacyKP)
		require.NoError(t, sErr)

		got, sErr := sigClient.SignStellarTransaction(ctx, tx, walletKP.Address(), legacyKP.Address())
		require.NoError(t, sErr)
		assert.Equal(t, want.Signatures(), got.Signatures(),
			"wallet account must sign with the envelope key (not the vault decoy); legacy account with the vault key")
	})

	t.Run("corrupted wallet key fails closed without vault fallthrough", func(t *testing.T) {
		require.NoError(t, walletKeyModel.UpdateCiphertext(ctx, walletKP.Address(), "Y29ycnVwdGVkY29ycnVwdGVk", encryptedDEK))

		_, sErr := sigClient.SignStellarTransaction(ctx, newTx(), walletKP.Address())
		require.Error(t, sErr)
		assert.ErrorContains(t, sErr, "decrypting distribution wallet key",
			"must fail on the wallet-keys path, not silently fall back to the (decoy) vault entry")
	})
}
