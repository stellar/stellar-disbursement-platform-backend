package signing

import (
	"context"
	"math"
	"reflect"
	"testing"

	"github.com/stellar/go/keypair"
	"github.com/stellar/go/network"
	"github.com/stellar/go/txnbuild"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/db/dbtest"
	preconditionsMocks "github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine/preconditions/mocks"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/store"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/utils"
)

func Test_ChannelAccountDBSignatureClientOptions_Validate(t *testing.T) {
	dbt := dbtest.OpenWithTSSMigrationsOnly(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	testCases := []struct {
		name            string
		opts            ChannelAccountDBSignatureClientOptions
		wantErrContains string
	}{
		{
			name:            "return an error if network passphrase is empty",
			wantErrContains: "network passphrase cannot be empty",
		},
		{
			name: "return an error if dbConnectionPool is nil",
			opts: ChannelAccountDBSignatureClientOptions{
				NetworkPassphrase: network.TestNetworkPassphrase,
			},
			wantErrContains: "database connection pool cannot be nil",
		},
		{
			name: "return an error if encryption passphrase is empty",
			opts: ChannelAccountDBSignatureClientOptions{
				NetworkPassphrase: network.TestNetworkPassphrase,
				DBConnectionPool:  dbConnectionPool,
			},
			wantErrContains: "encryption passphrase is not a valid Ed25519 secret",
		},
		{
			name: "return an error if encryption passphrase is invalid",
			opts: ChannelAccountDBSignatureClientOptions{
				NetworkPassphrase:    network.TestNetworkPassphrase,
				DBConnectionPool:     dbConnectionPool,
				EncryptionPassphrase: "invalid",
			},
			wantErrContains: "encryption passphrase is not a valid Ed25519 secret",
		},
		{
			name: "return an error if the ledger number tracker is nil",
			opts: ChannelAccountDBSignatureClientOptions{
				NetworkPassphrase:    network.TestNetworkPassphrase,
				DBConnectionPool:     dbConnectionPool,
				EncryptionPassphrase: "SCPGNK3MRMXKNWGZ4ET3JZ6RUJIN7FMHT4ASVXDG7YPBL4WKBQNEL63F",
			},
			wantErrContains: "ledger number tracker cannot be nil",
		},
		{
			name: "🎉 Successfully validates options",
			opts: ChannelAccountDBSignatureClientOptions{
				NetworkPassphrase:    network.TestNetworkPassphrase,
				DBConnectionPool:     dbConnectionPool,
				EncryptionPassphrase: "SCPGNK3MRMXKNWGZ4ET3JZ6RUJIN7FMHT4ASVXDG7YPBL4WKBQNEL63F",
				LedgerNumberTracker:  preconditionsMocks.NewMockLedgerNumberTracker(t),
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

func Test_NewChannelAccountDBSignatureClient(t *testing.T) {
	dbt := dbtest.OpenWithTSSMigrationsOnly(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	mLedgerNumberTracker := preconditionsMocks.NewMockLedgerNumberTracker(t)

	testCases := []struct {
		name                  string
		opts                  ChannelAccountDBSignatureClientOptions
		wantEncrypterTypeName string
		wantErrContains       string
	}{
		{
			name:            "return an error if validation fails with an empty networkPassphrase",
			wantErrContains: "validating options: network passphrase cannot be empty",
		},
		{
			name: "🎉 Successfully instantiates a new channel ccount DB signature client with default encrypter",
			opts: ChannelAccountDBSignatureClientOptions{
				NetworkPassphrase:    network.TestNetworkPassphrase,
				DBConnectionPool:     dbConnectionPool,
				EncryptionPassphrase: "SCPGNK3MRMXKNWGZ4ET3JZ6RUJIN7FMHT4ASVXDG7YPBL4WKBQNEL63F",
				LedgerNumberTracker:  mLedgerNumberTracker,
			},
			wantEncrypterTypeName: reflect.TypeOf(&utils.DefaultPrivateKeyEncrypter{}).String(),
		},
		{
			name: "🎉 Successfully instantiates a new channel ccount DB signature client with a custom encrypter",
			opts: ChannelAccountDBSignatureClientOptions{
				NetworkPassphrase:    network.TestNetworkPassphrase,
				DBConnectionPool:     dbConnectionPool,
				EncryptionPassphrase: "SCPGNK3MRMXKNWGZ4ET3JZ6RUJIN7FMHT4ASVXDG7YPBL4WKBQNEL63F",
				LedgerNumberTracker:  mLedgerNumberTracker,
				Encrypter:            &utils.PrivateKeyEncrypterMock{},
			},
			wantEncrypterTypeName: reflect.TypeOf(&utils.PrivateKeyEncrypterMock{}).String(),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			sigClient, err := NewChannelAccountDBSignatureClient(tc.opts)
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

func Test_ChannelAccountDBSignatureClientOptions_NetworkPassphrase(t *testing.T) {
	// test with testnet passphrase
	sigClient := &ChannelAccountDBSignatureClient{networkPassphrase: network.TestNetworkPassphrase}
	assert.Equal(t, network.TestNetworkPassphrase, sigClient.NetworkPassphrase())

	// test with public network passphrase, to make sure it's changing accordingly
	sigClient = &ChannelAccountDBSignatureClient{networkPassphrase: network.PublicNetworkPassphrase}
	assert.Equal(t, network.PublicNetworkPassphrase, sigClient.NetworkPassphrase())
}

func Test_ChannelAccountDBSignatureClient_getKPsForAccounts(t *testing.T) {
	dbt := dbtest.OpenWithTSSMigrationsOnly(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()
	chAccountStore := store.NewChannelAccountModel(dbConnectionPool)

	// create default encrypter
	encrypter := &utils.DefaultPrivateKeyEncrypter{}
	encrypterPass := keypair.MustRandom().Seed()

	// create channel accounts in the DB
	channelAccounts := store.CreateChannelAccountFixturesEncryptedKPs(t, ctx, dbConnectionPool, encrypter, encrypterPass, 2)
	chAccKP1, chAccKP2 := channelAccounts[0], channelAccounts[1]

	// create channel account that's not in the DB
	nonExistentChannelAccountKP, err := keypair.Random()
	require.NoError(t, err)

	// create Channel account with private key encrypted by a different passphrase
	undecryptableKeyChAccKP := keypair.MustRandom()
	undecryptableKeyChAccKPSeed, err := encrypter.Encrypt(undecryptableKeyChAccKP.Seed(), keypair.MustRandom().Seed())
	require.NoError(t, err)
	err = chAccountStore.Insert(ctx, chAccountStore.DBConnectionPool, undecryptableKeyChAccKP.Address(), undecryptableKeyChAccKPSeed)
	require.NoError(t, err)

	// create signature client
	sigClient, err := NewChannelAccountDBSignatureClient(ChannelAccountDBSignatureClientOptions{
		NetworkPassphrase:    network.TestNetworkPassphrase,
		DBConnectionPool:     dbConnectionPool,
		EncryptionPassphrase: encrypterPass,
		Encrypter:            encrypter,
		LedgerNumberTracker:  preconditionsMocks.NewMockLedgerNumberTracker(t),
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
			wantErrContains: "no accounts provided",
		},
		{
			name:            "return an error if one of the accounts is empty",
			accounts:        []string{""},
			wantErrContains: "account 0 is empty",
		},
		{
			name:            "return an error if one of the accounts doesn't exist in the database",
			accounts:        []string{nonExistentChannelAccountKP.Address()},
			wantErrContains: store.ErrRecordNotFound.Error(),
		},
		{
			name:         "🎉 Successfully one result if there are repeated values in the input array",
			accounts:     []string{chAccKP1.Address(), chAccKP1.Address()},
			wantKeypairs: []*keypair.Full{chAccKP1},
		},
		{
			name:         "🎉 Successfully returns all results if they're all distinct addresses in the DB",
			accounts:     []string{chAccKP1.Address(), chAccKP2.Address()},
			wantKeypairs: []*keypair.Full{chAccKP1, chAccKP2},
		},
		{
			name:            "return an error if one of the encrypted seeds cannot be decrypted with the expected passphrase",
			accounts:        []string{undecryptableKeyChAccKP.Address()},
			wantErrContains: "cannot decrypt private key: cipher: message authentication failed",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			kps, err := sigClient.getKPsForAccounts(ctx, tc.accounts...)
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

func Test_ChannelAccountDBSignatureClient_SignStellarTransaction(t *testing.T) {
	dbt := dbtest.OpenWithTSSMigrationsOnly(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()

	// create default encrypter
	encrypterPass := keypair.MustRandom().Seed()
	encrypter := &utils.DefaultPrivateKeyEncrypter{}

	// create channel accounts in the DB
	channelAccounts := store.CreateChannelAccountFixturesEncryptedKPs(t, ctx, dbConnectionPool, encrypter, encrypterPass, 2)
	chAccKP1, chAccKP2 := channelAccounts[0], channelAccounts[1]

	sigClient, err := NewChannelAccountDBSignatureClient(ChannelAccountDBSignatureClientOptions{
		NetworkPassphrase:    network.TestNetworkPassphrase,
		DBConnectionPool:     dbConnectionPool,
		EncryptionPassphrase: encrypterPass,
		Encrypter:            encrypter,
		LedgerNumberTracker:  preconditionsMocks.NewMockLedgerNumberTracker(t),
	})
	require.NoError(t, err)

	// create stellar transaction
	chSourceAccount := txnbuild.NewSimpleAccount(chAccKP1.Address(), int64(9605939170639897))
	stellarTx, err := txnbuild.NewTransaction(
		txnbuild.TransactionParams{
			SourceAccount:        &chSourceAccount,
			IncrementSequenceNum: true,
			Operations: []txnbuild.Operation{&txnbuild.Payment{
				Destination:   "GCCOBXW2XQNUSL467IEILE6MMCNRR66SSVL4YQADUNYYNUVREF3FIV2Z",
				Amount:        "10",
				Asset:         txnbuild.NativeAsset{},
				SourceAccount: chAccKP2.Address(),
			}},
			BaseFee:       txnbuild.MinBaseFee,
			Preconditions: txnbuild.Preconditions{TimeBounds: txnbuild.NewTimeout(60)},
		},
	)
	require.NoError(t, err)

	wantSignedStellarTx, err := stellarTx.Sign(network.TestNetworkPassphrase, chAccKP1, chAccKP2)
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
			wantErrContains: "no accounts provided",
		},
		{
			name:                "🎉 Successfully sign transaction when all incoming addresses are correct",
			stellarTx:           stellarTx,
			accounts:            []string{chAccKP1.Address(), chAccKP2.Address()},
			wantSignedStellarTx: wantSignedStellarTx,
		},
		{
			name:                "🎉 Successfully sign transaction when some incoming address are repeated",
			stellarTx:           stellarTx,
			accounts:            []string{chAccKP1.Address(), chAccKP2.Address(), chAccKP2.Address()},
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

func Test_ChannelAccountDBSignatureClient_SignFeeBumpStellarTransaction(t *testing.T) {
	dbt := dbtest.OpenWithTSSMigrationsOnly(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()

	// create default encrypter
	encrypterPass := keypair.MustRandom().Seed()
	encrypter := &utils.DefaultPrivateKeyEncrypter{}

	// create channel accounts in the DB
	channelAccounts := store.CreateChannelAccountFixturesEncryptedKPs(t, ctx, dbConnectionPool, encrypter, encrypterPass, 2)
	chAccKP1, chAccKP2 := channelAccounts[0], channelAccounts[1]

	sigClient, err := NewChannelAccountDBSignatureClient(ChannelAccountDBSignatureClientOptions{
		NetworkPassphrase:    network.TestNetworkPassphrase,
		DBConnectionPool:     dbConnectionPool,
		EncryptionPassphrase: encrypterPass,
		Encrypter:            &utils.DefaultPrivateKeyEncrypter{},
		LedgerNumberTracker:  preconditionsMocks.NewMockLedgerNumberTracker(t),
	})
	require.NoError(t, err)

	// create stellar transaction
	chSourceAccount := txnbuild.NewSimpleAccount(chAccKP1.Address(), int64(9605939170639897))
	stellarTx, err := txnbuild.NewTransaction(
		txnbuild.TransactionParams{
			SourceAccount:        &chSourceAccount,
			IncrementSequenceNum: true,
			Operations: []txnbuild.Operation{&txnbuild.Payment{
				Destination:   "GCCOBXW2XQNUSL467IEILE6MMCNRR66SSVL4YQADUNYYNUVREF3FIV2Z",
				Amount:        "10",
				Asset:         txnbuild.NativeAsset{},
				SourceAccount: chAccKP2.Address(),
			}},
			BaseFee:       txnbuild.MinBaseFee,
			Preconditions: txnbuild.Preconditions{TimeBounds: txnbuild.NewTimeout(60)},
		},
	)
	require.NoError(t, err)
	signedStellarTx, err := stellarTx.Sign(network.TestNetworkPassphrase, chAccKP1, chAccKP2)
	require.NoError(t, err)

	feeBumpStellarTx, err := txnbuild.NewFeeBumpTransaction(
		txnbuild.FeeBumpTransactionParams{
			Inner:      signedStellarTx,
			FeeAccount: chAccKP2.Address(),
			BaseFee:    txnbuild.MinBaseFee,
		},
	)
	require.NoError(t, err)

	wantSignedFeeBumpStellarTx, err := feeBumpStellarTx.Sign(network.TestNetworkPassphrase, chAccKP2)
	assert.NoError(t, err)

	testCases := []struct {
		name                       string
		feeBumpStellarTx           *txnbuild.FeeBumpTransaction
		accounts                   []string
		wantErrContains            string
		wantSignedFeeBumpStellarTx *txnbuild.FeeBumpTransaction
	}{
		{
			name:             "return an error if stellar transaction is nil",
			feeBumpStellarTx: nil,
			accounts:         []string{},
			wantErrContains:  "stellarTx cannot be nil",
		},
		{
			name:             "return an error if no accounts are passed",
			feeBumpStellarTx: feeBumpStellarTx,
			accounts:         []string{},
			wantErrContains:  "no accounts provided",
		},
		{
			name:                       "🎉 Successfully sign transaction when all incoming addresses are correct",
			feeBumpStellarTx:           feeBumpStellarTx,
			accounts:                   []string{chAccKP2.Address()},
			wantSignedFeeBumpStellarTx: wantSignedFeeBumpStellarTx,
		},
		{
			name:                       "🎉 Successfully sign transaction when all some address are repeated",
			feeBumpStellarTx:           feeBumpStellarTx,
			accounts:                   []string{chAccKP2.Address(), chAccKP2.Address()},
			wantSignedFeeBumpStellarTx: wantSignedFeeBumpStellarTx,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			gotSignedFeeBumpStellarTx, err := sigClient.SignFeeBumpStellarTransaction(ctx, tc.feeBumpStellarTx, tc.accounts...)
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

func Test_ChannelAccountDBSignatureClient_BatchInsert(t *testing.T) {
	dbt := dbtest.OpenWithTSSMigrationsOnly(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()
	chAccountStore := &store.ChannelAccountModel{DBConnectionPool: dbConnectionPool}

	distributionKP, err := keypair.Random()
	require.NoError(t, err)

	testCase := []struct {
		name            string
		amount          int
		wantErrContains string
	}{
		{
			name:            "if amount<=0, return an error",
			wantErrContains: "the amount of accounts to insert need to be greater than zero",
		},
		{
			name:   "🎉 successfully bulk insert",
			amount: 2,
		},
	}

	mLedgerNumberTracker := preconditionsMocks.NewMockLedgerNumberTracker(t)
	mLedgerNumberTracker.On("GetLedgerNumber").Return(100, nil).Once()

	defaultEncrypter := &utils.DefaultPrivateKeyEncrypter{}
	encrypterPass := distributionKP.Seed()
	sigClient, err := NewChannelAccountDBSignatureClient(ChannelAccountDBSignatureClientOptions{
		NetworkPassphrase:    network.TestNetworkPassphrase,
		DBConnectionPool:     dbConnectionPool,
		EncryptionPassphrase: encrypterPass,
		Encrypter:            defaultEncrypter,
		LedgerNumberTracker:  mLedgerNumberTracker,
	})
	require.NoError(t, err)

	for _, tc := range testCase {
		t.Run(tc.name, func(t *testing.T) {
			count, err := chAccountStore.Count(ctx)
			require.NoError(t, err)
			require.Equal(t, 0, count, "this test should have started with 0 channel accounts")

			publicKeys, err := sigClient.BatchInsert(ctx, tc.amount)
			if tc.wantErrContains != "" {
				require.Error(t, err)
				assert.ErrorContains(t, err, tc.wantErrContains)
				assert.Nil(t, publicKeys)
			} else {
				require.NoError(t, err)

				allChAccounts, err := chAccountStore.GetAll(ctx, dbConnectionPool, math.MaxInt32, 0)
				require.NoError(t, err)
				assert.Equal(t, tc.amount, len(publicKeys))
				assert.Equal(t, tc.amount, len(allChAccounts))

				// compare the accounts
				var alChAccPublicKeys []string
				for _, chAccount := range allChAccounts {
					alChAccPublicKeys = append(alChAccPublicKeys, chAccount.PublicKey)

					// Check if the private key is the actual seed for the public key
					encryptedPrivateKey := chAccount.PrivateKey
					privateKey, err := defaultEncrypter.Decrypt(encryptedPrivateKey, encrypterPass)
					require.NoError(t, err)
					kp := keypair.MustParseFull(privateKey)
					assert.Equal(t, chAccount.PublicKey, kp.Address())
				}

				assert.ElementsMatch(t, alChAccPublicKeys, publicKeys)
			}

			store.DeleteAllFromChannelAccounts(t, ctx, dbConnectionPool)
		})
	}
}

func Test_ChannelAccountDBSignatureClient_Delete(t *testing.T) {
	dbt := dbtest.OpenWithTSSMigrationsOnly(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()
	chAccountStore := store.NewChannelAccountModel(dbConnectionPool)

	// create default encrypter
	encrypterPass := keypair.MustRandom().Seed()
	encrypter := &utils.DefaultPrivateKeyEncrypter{}

	// current ledger number
	currLedgerNumber := 0
	lockUntilLedgerNumber := 10
	mLedgerNumberTracker := preconditionsMocks.NewMockLedgerNumberTracker(t)
	mLedgerNumberTracker.
		On("GetLedgerNumber").
		Return(currLedgerNumber, nil).
		Times(3)

	// at start: count=0
	count, err := chAccountStore.Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, 0, count)

	// create 2 accounts: count=0->2
	channelAccounts := store.CreateChannelAccountFixtures(t, ctx, dbConnectionPool, 2)
	count, err = chAccountStore.Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, 2, count)

	for _, chAcc := range channelAccounts {
		_, err = chAccountStore.Lock(ctx, chAccountStore.DBConnectionPool, chAcc.PublicKey, int32(currLedgerNumber), int32(lockUntilLedgerNumber))
		require.NoError(t, err)
	}

	sigClient, err := NewChannelAccountDBSignatureClient(ChannelAccountDBSignatureClientOptions{
		NetworkPassphrase:    network.TestNetworkPassphrase,
		DBConnectionPool:     dbConnectionPool,
		EncryptionPassphrase: encrypterPass,
		Encrypter:            encrypter,
		LedgerNumberTracker:  mLedgerNumberTracker,
	})
	require.NoError(t, err)

	// delete one account: count=2->1
	err = sigClient.Delete(ctx, channelAccounts[0].PublicKey)
	require.NoError(t, err)
	count, err = chAccountStore.Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, count)

	// delete another account: count=1->0
	err = sigClient.Delete(ctx, channelAccounts[1].PublicKey)
	require.NoError(t, err)
	count, err = chAccountStore.Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, 0, count)

	// delete non-existing account: error expected
	err = sigClient.Delete(ctx, "non-existent-account")
	require.Error(t, err)
	assert.ErrorIs(t, err, store.ErrRecordNotFound)
}
