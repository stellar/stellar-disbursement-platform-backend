package signing

import (
	"context"
	"reflect"
	"testing"

	sdpUtils "github.com/stellar/stellar-disbursement-platform-backend/internal/utils"

	"github.com/stellar/go/keypair"
	"github.com/stellar/go/network"
	"github.com/stellar/go/txnbuild"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/db/dbtest"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/store"
)

func Test_DistributionAccountDBVaultSignatureClientOptions_Validate(t *testing.T) {
	dbt := dbtest.OpenWithTSSMigrationsOnly(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	testCases := []struct {
		name            string
		opts            DistributionAccountDBVaultSignatureClientOptions
		wantErrContains string
	}{
		{
			name:            "return an error if network passphrase is empty",
			wantErrContains: "network passphrase cannot be empty",
		},
		{
			name: "return an error if dbConnectionPool is nil",
			opts: DistributionAccountDBVaultSignatureClientOptions{
				NetworkPassphrase: network.TestNetworkPassphrase,
			},
			wantErrContains: "database connection pool cannot be nil",
		},
		{
			name: "return an error if encryption passphrase is empty",
			opts: DistributionAccountDBVaultSignatureClientOptions{
				NetworkPassphrase: network.TestNetworkPassphrase,
				DBConnectionPool:  dbConnectionPool,
			},
			wantErrContains: "encryption passphrase is not a valid Ed25519 secret",
		},
		{
			name: "return an error if encryption passphrase is invalid",
			opts: DistributionAccountDBVaultSignatureClientOptions{
				NetworkPassphrase:    network.TestNetworkPassphrase,
				DBConnectionPool:     dbConnectionPool,
				EncryptionPassphrase: "invalid",
			},
			wantErrContains: "encryption passphrase is not a valid Ed25519 secret",
		},
		{
			name: "🎉 Successfully validates options",
			opts: DistributionAccountDBVaultSignatureClientOptions{
				NetworkPassphrase:    network.TestNetworkPassphrase,
				DBConnectionPool:     dbConnectionPool,
				EncryptionPassphrase: "SCPGNK3MRMXKNWGZ4ET3JZ6RUJIN7FMHT4ASVXDG7YPBL4WKBQNEL63F",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.opts.Validate()
			if tc.wantErrContains != "" {
				require.Error(t, err)
				assert.ErrorContains(t, err, tc.wantErrContains)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func Test_NewDistributionAccountDBVaultSignatureClient(t *testing.T) {
	dbt := dbtest.OpenWithTSSMigrationsOnly(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	testCases := []struct {
		name                  string
		opts                  DistributionAccountDBVaultSignatureClientOptions
		wantEncrypterTypeName string
		wantErrContains       string
	}{
		{
			name:            "return an error if validation fails with an empty networkPassphrase",
			wantErrContains: "validating options: network passphrase cannot be empty",
		},
		{
			name: "🎉 Successfully instantiates a new distribution account DB signature client with default encrypter",
			opts: DistributionAccountDBVaultSignatureClientOptions{
				NetworkPassphrase:    network.TestNetworkPassphrase,
				DBConnectionPool:     dbConnectionPool,
				EncryptionPassphrase: "SCPGNK3MRMXKNWGZ4ET3JZ6RUJIN7FMHT4ASVXDG7YPBL4WKBQNEL63F",
			},
			wantEncrypterTypeName: reflect.TypeOf(&sdpUtils.DefaultPrivateKeyEncrypter{}).String(),
		},
		{
			name: "🎉 Successfully instantiates a new distribution account DB signature client with a custom encrypter",
			opts: DistributionAccountDBVaultSignatureClientOptions{
				NetworkPassphrase:    network.TestNetworkPassphrase,
				DBConnectionPool:     dbConnectionPool,
				EncryptionPassphrase: "SCPGNK3MRMXKNWGZ4ET3JZ6RUJIN7FMHT4ASVXDG7YPBL4WKBQNEL63F",
				Encrypter:            &sdpUtils.PrivateKeyEncrypterMock{},
			},
			wantEncrypterTypeName: reflect.TypeOf(&sdpUtils.PrivateKeyEncrypterMock{}).String(),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			sigClient, err := NewDistributionAccountDBVaultSignatureClient(tc.opts)
			if tc.wantErrContains != "" {
				require.Error(t, err)
				assert.ErrorContains(t, err, tc.wantErrContains)
				assert.Nil(t, sigClient)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, sigClient)
				assert.Equal(t, tc.wantEncrypterTypeName, reflect.TypeOf(sigClient.encrypter).String())
			}
		})
	}
}

func Test_DistributionAccountDBVaultSignatureClientOptions_NetworkPassphrase(t *testing.T) {
	// test with testnet passphrase
	sigClient := &DistributionAccountDBVaultSignatureClient{networkPassphrase: network.TestNetworkPassphrase}
	assert.Equal(t, network.TestNetworkPassphrase, sigClient.NetworkPassphrase())

	// test with public network passphrase, to make sure it's changing accordingly
	sigClient = &DistributionAccountDBVaultSignatureClient{networkPassphrase: network.PublicNetworkPassphrase}
	assert.Equal(t, network.PublicNetworkPassphrase, sigClient.NetworkPassphrase())
}

func Test_DistributionAccountDBVaultSignatureClient_getKPsForAccounts(t *testing.T) {
	dbt := dbtest.OpenWithTSSMigrationsOnly(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()
	dbVaultStore := store.NewDBVaultModel(dbConnectionPool)

	// create default encrypter
	encrypter := &sdpUtils.DefaultPrivateKeyEncrypter{}
	encrypterPass := keypair.MustRandom().Seed()

	// create distribution accounts in the DB
	distributionAccounts := store.CreateDBVaultFixturesEncryptedKPs(t, ctx, dbConnectionPool, encrypter, encrypterPass, 2)
	require.Len(t, distributionAccounts, 2)
	distAccKP1, distAccKP2 := distributionAccounts[0], distributionAccounts[1]

	// create distribution account that's not in the DB
	nonExistentDistributionAccountKP, err := keypair.Random()
	require.NoError(t, err)

	// create Distribution account with private key encrypted by a different passphrase
	undecryptableKeyChAccKP := keypair.MustRandom()
	undecryptableKeyChAccKPSeed, err := encrypter.Encrypt(undecryptableKeyChAccKP.Seed(), keypair.MustRandom().Seed())
	require.NoError(t, err)
	err = dbVaultStore.BatchInsert(ctx, []*store.DBVaultEntry{{PublicKey: undecryptableKeyChAccKP.Address(), EncryptedPrivateKey: undecryptableKeyChAccKPSeed}})
	require.NoError(t, err)

	// create signature client
	sigClient, err := NewDistributionAccountDBVaultSignatureClient(DistributionAccountDBVaultSignatureClientOptions{
		NetworkPassphrase:    network.TestNetworkPassphrase,
		DBConnectionPool:     dbConnectionPool,
		EncryptionPassphrase: encrypterPass,
		Encrypter:            encrypter,
	})
	require.NoError(t, err)

	testCases := []struct {
		name            string
		accounts        []string
		wantErrContains string
		wantKeypairs    []*keypair.Full
	}{
		{
			name:            "return an error if no accounts are passed",
			accounts:        []string{},
			wantErrContains: "no publicKeys provided",
		},
		{
			name:            "return an error if one of the accounts is empty",
			accounts:        []string{""},
			wantErrContains: "publicKey 0 is empty",
		},
		{
			name:            "return an error if one of the accounts doesn't exist in the database",
			accounts:        []string{nonExistentDistributionAccountKP.Address()},
			wantErrContains: store.ErrRecordNotFound.Error(),
		},
		{
			name:         "🎉 Successfully one result if there are repeated values in the input array",
			accounts:     []string{distAccKP1.Address(), distAccKP1.Address()},
			wantKeypairs: []*keypair.Full{distAccKP1},
		},
		{
			name:         "🎉 Successfully returns all results if they're all distinct addresses in the DB",
			accounts:     []string{distAccKP1.Address(), distAccKP2.Address()},
			wantKeypairs: []*keypair.Full{distAccKP1, distAccKP2},
		},
		{
			name:            "return an error if one of the encrypted seeds cannot be decrypted with the expected passphrase",
			accounts:        []string{undecryptableKeyChAccKP.Address()},
			wantErrContains: "cannot decrypt private key: decrypting and authenticating message: cipher: message authentication failed",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			kps, err := sigClient.getKPsForPublicKeys(ctx, tc.accounts...)
			if tc.wantErrContains != "" {
				require.Error(t, err)
				assert.ErrorContains(t, err, tc.wantErrContains)
				assert.Nil(t, kps)
			} else {
				require.NoError(t, err)
				assert.Len(t, kps, len(tc.wantKeypairs))
				assert.Equal(t, tc.wantKeypairs, kps)
			}
		})
	}
}

func Test_DistributionAccountDBVaultSignatureClient_SignStellarTransaction(t *testing.T) {
	dbt := dbtest.OpenWithTSSMigrationsOnly(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()

	// create default encrypter
	encrypterPass := keypair.MustRandom().Seed()
	encrypter := &sdpUtils.DefaultPrivateKeyEncrypter{}

	// create distribution accounts in the DB
	distributionAccounts := store.CreateDBVaultFixturesEncryptedKPs(t, ctx, dbConnectionPool, encrypter, encrypterPass, 2)
	require.Len(t, distributionAccounts, 2)
	distAccKP1, distAccKP2 := distributionAccounts[0], distributionAccounts[1]

	sigClient, err := NewDistributionAccountDBVaultSignatureClient(DistributionAccountDBVaultSignatureClientOptions{
		NetworkPassphrase:    network.TestNetworkPassphrase,
		DBConnectionPool:     dbConnectionPool,
		EncryptionPassphrase: encrypterPass,
		Encrypter:            encrypter,
	})
	require.NoError(t, err)

	// create stellar transaction
	distSourceAccount := txnbuild.NewSimpleAccount(distAccKP1.Address(), int64(9605939170639897))
	stellarTx, err := txnbuild.NewTransaction(
		txnbuild.TransactionParams{
			SourceAccount:        &distSourceAccount,
			IncrementSequenceNum: true,
			Operations: []txnbuild.Operation{&txnbuild.Payment{
				Destination:   "GCCOBXW2XQNUSL467IEILE6MMCNRR66SSVL4YQADUNYYNUVREF3FIV2Z",
				Amount:        "10",
				Asset:         txnbuild.NativeAsset{},
				SourceAccount: distAccKP2.Address(),
			}},
			BaseFee:       txnbuild.MinBaseFee,
			Preconditions: txnbuild.Preconditions{TimeBounds: txnbuild.NewTimeout(60)},
		},
	)
	require.NoError(t, err)

	wantSignedStellarTx, err := stellarTx.Sign(network.TestNetworkPassphrase, distAccKP1, distAccKP2)
	require.NoError(t, err)

	testCases := []struct {
		name                string
		stellarTx           *txnbuild.Transaction
		accounts            []string
		wantErrContains     string
		wantSignedStellarTx *txnbuild.Transaction
	}{
		{
			name:            "return an error if stellar transaction is nil",
			stellarTx:       nil,
			accounts:        []string{},
			wantErrContains: "stellarTx cannot be nil",
		},
		{
			name:            "return an error if no accounts are passed",
			stellarTx:       stellarTx,
			accounts:        []string{},
			wantErrContains: "no publicKeys provided",
		},
		{
			name:                "🎉 Successfully sign transaction when all incoming addresses are correct",
			stellarTx:           stellarTx,
			accounts:            []string{distAccKP1.Address(), distAccKP2.Address()},
			wantSignedStellarTx: wantSignedStellarTx,
		},
		{
			name:                "🎉 Successfully sign transaction when some incoming address are repeated",
			stellarTx:           stellarTx,
			accounts:            []string{distAccKP1.Address(), distAccKP2.Address(), distAccKP2.Address()},
			wantSignedStellarTx: wantSignedStellarTx,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			gotSignedStellarTx, err := sigClient.SignStellarTransaction(ctx, tc.stellarTx, tc.accounts...)
			if tc.wantErrContains != "" {
				require.Error(t, err)
				assert.ErrorContains(t, err, tc.wantErrContains)
				assert.Nil(t, gotSignedStellarTx)
			} else {
				require.NoError(t, err)
				assert.ElementsMatch(t, tc.wantSignedStellarTx.Signatures(), gotSignedStellarTx.Signatures())
			}
		})
	}
}

func Test_DistributionAccountDBVaultSignatureClient_SignFeeBumpStellarTransaction(t *testing.T) {
	dbt := dbtest.OpenWithTSSMigrationsOnly(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()

	// create default encrypter
	encrypterPass := keypair.MustRandom().Seed()
	encrypter := &sdpUtils.DefaultPrivateKeyEncrypter{}

	// create distribution accounts in the DB
	distributionAccounts := store.CreateDBVaultFixturesEncryptedKPs(t, ctx, dbConnectionPool, encrypter, encrypterPass, 2)
	require.Len(t, distributionAccounts, 2)
	distAccKP1, distAccKP2 := distributionAccounts[0], distributionAccounts[1]

	sigClient, err := NewDistributionAccountDBVaultSignatureClient(DistributionAccountDBVaultSignatureClientOptions{
		NetworkPassphrase:    network.TestNetworkPassphrase,
		DBConnectionPool:     dbConnectionPool,
		EncryptionPassphrase: encrypterPass,
		Encrypter:            &sdpUtils.DefaultPrivateKeyEncrypter{},
	})
	require.NoError(t, err)

	// create stellar transaction
	distSourceAccount := txnbuild.NewSimpleAccount(distAccKP1.Address(), int64(9605939170639897))
	stellarTx, err := txnbuild.NewTransaction(
		txnbuild.TransactionParams{
			SourceAccount:        &distSourceAccount,
			IncrementSequenceNum: true,
			Operations: []txnbuild.Operation{&txnbuild.Payment{
				Destination:   "GCCOBXW2XQNUSL467IEILE6MMCNRR66SSVL4YQADUNYYNUVREF3FIV2Z",
				Amount:        "10",
				Asset:         txnbuild.NativeAsset{},
				SourceAccount: distAccKP2.Address(),
			}},
			BaseFee:       txnbuild.MinBaseFee,
			Preconditions: txnbuild.Preconditions{TimeBounds: txnbuild.NewTimeout(60)},
		},
	)
	require.NoError(t, err)
	signedStellarTx, err := stellarTx.Sign(network.TestNetworkPassphrase, distAccKP1, distAccKP2)
	require.NoError(t, err)

	feeBumpStellarTx, err := txnbuild.NewFeeBumpTransaction(
		txnbuild.FeeBumpTransactionParams{
			Inner:      signedStellarTx,
			FeeAccount: distAccKP2.Address(),
			BaseFee:    txnbuild.MinBaseFee,
		},
	)
	require.NoError(t, err)

	wantSignedFeeBumpStellarTx, err := feeBumpStellarTx.Sign(network.TestNetworkPassphrase, distAccKP2)
	assert.NoError(t, err)

	testCases := []struct {
		name                       string
		feeBumpStellarTx           *txnbuild.FeeBumpTransaction
		publicKeys                 []string
		wantErrContains            string
		wantSignedFeeBumpStellarTx *txnbuild.FeeBumpTransaction
	}{
		{
			name:             "return an error if stellar transaction is nil",
			feeBumpStellarTx: nil,
			publicKeys:       []string{},
			wantErrContains:  "stellarTx cannot be nil",
		},
		{
			name:             "return an error if no accounts are passed",
			feeBumpStellarTx: feeBumpStellarTx,
			publicKeys:       []string{},
			wantErrContains:  "no publicKeys provided",
		},
		{
			name:                       "🎉 Successfully sign transaction when all incoming publicKeys are correct",
			feeBumpStellarTx:           feeBumpStellarTx,
			publicKeys:                 []string{distAccKP2.Address()},
			wantSignedFeeBumpStellarTx: wantSignedFeeBumpStellarTx,
		},
		{
			name:                       "🎉 Successfully sign transaction when all publicKeys are repeated",
			feeBumpStellarTx:           feeBumpStellarTx,
			publicKeys:                 []string{distAccKP2.Address(), distAccKP2.Address()},
			wantSignedFeeBumpStellarTx: wantSignedFeeBumpStellarTx,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			gotSignedFeeBumpStellarTx, err := sigClient.SignFeeBumpStellarTransaction(ctx, tc.feeBumpStellarTx, tc.publicKeys...)
			if tc.wantErrContains != "" {
				require.Error(t, err)
				assert.ErrorContains(t, err, tc.wantErrContains)
				assert.Nil(t, gotSignedFeeBumpStellarTx)
			} else {
				require.NoError(t, err)
				assert.ElementsMatch(t, tc.wantSignedFeeBumpStellarTx.Signatures(), gotSignedFeeBumpStellarTx.Signatures())
			}
		})
	}
}

// allDBVaultEntries is a test helper that returns all the dbVaultEntries from the DB.
func allDBVaultEntries(t *testing.T, ctx context.Context, dbConnectionPool db.DBConnectionPool) []store.DBVaultEntry {
	t.Helper()

	var dbVaultEntries []store.DBVaultEntry
	err := dbConnectionPool.SelectContext(ctx, &dbVaultEntries, "SELECT * FROM vault")
	require.NoError(t, err)
	return dbVaultEntries
}

func Test_DistributionAccountDBVaultSignatureClient_BatchInsert(t *testing.T) {
	dbt := dbtest.OpenWithTSSMigrationsOnly(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()

	distributionKP, err := keypair.Random()
	require.NoError(t, err)

	testCase := []struct {
		name            string
		amount          int
		wantErrContains string
	}{
		{
			name:            "if number<=0, return an error",
			wantErrContains: "the number of publicKeys to insert needs to be greater than zero",
		},
		{
			name:   "🎉 successfully bulk insert",
			amount: 2,
		},
	}

	defaultEncrypter := &sdpUtils.DefaultPrivateKeyEncrypter{}
	encrypterPass := distributionKP.Seed()
	sigClient, err := NewDistributionAccountDBVaultSignatureClient(DistributionAccountDBVaultSignatureClientOptions{
		NetworkPassphrase:    network.TestNetworkPassphrase,
		DBConnectionPool:     dbConnectionPool,
		EncryptionPassphrase: encrypterPass,
		Encrypter:            defaultEncrypter,
	})
	require.NoError(t, err)

	for _, tc := range testCase {
		t.Run(tc.name, func(t *testing.T) {
			dbVaultEntries := allDBVaultEntries(t, ctx, dbConnectionPool)
			require.Len(t, dbVaultEntries, 0, "this test should have started with 0 distribution accounts")

			publicKeys, err := sigClient.BatchInsert(ctx, tc.amount)
			if tc.wantErrContains != "" {
				require.Error(t, err)
				assert.ErrorContains(t, err, tc.wantErrContains)
				assert.Nil(t, publicKeys)
			} else {
				require.NoError(t, err)

				dbVaultEntries = allDBVaultEntries(t, ctx, dbConnectionPool)
				assert.Equal(t, tc.amount, len(publicKeys))
				assert.Equal(t, tc.amount, len(dbVaultEntries))

				// compare the accounts
				var alChAccPublicKeys []string
				for _, distAccount := range dbVaultEntries {
					alChAccPublicKeys = append(alChAccPublicKeys, distAccount.PublicKey)

					// Check if the private key is the actual seed for the public key
					privateKey, err := defaultEncrypter.Decrypt(distAccount.EncryptedPrivateKey, encrypterPass)
					require.NoError(t, err)
					kp := keypair.MustParseFull(privateKey)
					assert.Equal(t, distAccount.PublicKey, kp.Address())
				}

				assert.ElementsMatch(t, alChAccPublicKeys, publicKeys)
			}

			store.DeleteAllFromDBVaultEntries(t, ctx, dbConnectionPool)
		})
	}
}

func Test_DistributionAccountDBVaultSignatureClient_Delete(t *testing.T) {
	dbt := dbtest.OpenWithTSSMigrationsOnly(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()

	// create default encrypter
	encrypterPass := keypair.MustRandom().Seed()
	encrypter := &sdpUtils.DefaultPrivateKeyEncrypter{}

	// at start: count=0
	allDistAccounts := allDBVaultEntries(t, ctx, dbConnectionPool)
	require.Len(t, allDistAccounts, 0)

	// create 2 accounts: count=0->2
	distributionAccounts := store.CreateDBVaultFixturesEncryptedKPs(t, ctx, dbConnectionPool, encrypter, encrypterPass, 2)
	allDistAccounts = allDBVaultEntries(t, ctx, dbConnectionPool)
	require.Len(t, allDistAccounts, 2)

	sigClient, err := NewDistributionAccountDBVaultSignatureClient(DistributionAccountDBVaultSignatureClientOptions{
		NetworkPassphrase:    network.TestNetworkPassphrase,
		DBConnectionPool:     dbConnectionPool,
		EncryptionPassphrase: encrypterPass,
		Encrypter:            encrypter,
	})
	require.NoError(t, err)

	// delete one account: count=2->1
	err = sigClient.Delete(ctx, distributionAccounts[0].Address())
	require.NoError(t, err)
	allDistAccounts = allDBVaultEntries(t, ctx, dbConnectionPool)
	require.Len(t, allDistAccounts, 1)

	// delete another account: count=1->0
	err = sigClient.Delete(ctx, distributionAccounts[1].Address())
	require.NoError(t, err)
	allDistAccounts = allDBVaultEntries(t, ctx, dbConnectionPool)
	require.Len(t, allDistAccounts, 0)

	// delete non-existing account: error expected
	err = sigClient.Delete(ctx, "non-existent-account")
	require.Error(t, err)
	assert.ErrorIs(t, err, store.ErrRecordNotFound)
}
