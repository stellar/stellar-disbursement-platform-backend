package engine

import (
	"context"
	"math"
	"testing"

	"github.com/stellar/go/keypair"
	"github.com/stellar/go/network"
	"github.com/stellar/go/txnbuild"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/db"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/db/dbtest"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/store"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_NewDefaultSignatureService(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	chAccountStore := &store.ChannelAccountModel{DBConnectionPool: dbConnectionPool}

	testCases := []struct {
		name              string
		chAccountStore    store.ChannelAccountStore
		networkPassphrase string
		distributionSeed  string
		encrypter         utils.PrivateKeyEncrypter
		encrypterPass     string
		wantErrContains   string
	}{
		{
			name:            "return an error if dbConnectionPool is nil",
			wantErrContains: "channel account store cannot be nil",
		},
		{
			name:              "return an error if networkPassphrase is invalid",
			chAccountStore:    chAccountStore,
			networkPassphrase: "foo bar",
			wantErrContains:   `invalid network passphrase: "foo bar"`,
		},
		{
			name:              "return an error if distributionSeed is invalid",
			chAccountStore:    chAccountStore,
			networkPassphrase: network.TestNetworkPassphrase,
			distributionSeed:  "foo bar",
			wantErrContains:   "parsing distribution seed: base32 decode failed: illegal base32 data at input byte 7",
		},
		{
			name:              "return an error if encrypter is nil",
			chAccountStore:    chAccountStore,
			networkPassphrase: network.TestNetworkPassphrase,
			distributionSeed:  "SCPGNK3MRMXKNWGZ4ET3JZ6RUJIN7FMHT4ASVXDG7YPBL4WKBQNEL63F",
			wantErrContains:   "private key encrypter cannot be nil",
		},
		{
			name:              "return an error if encrypterPass is empty",
			chAccountStore:    chAccountStore,
			networkPassphrase: network.TestNetworkPassphrase,
			distributionSeed:  "SCPGNK3MRMXKNWGZ4ET3JZ6RUJIN7FMHT4ASVXDG7YPBL4WKBQNEL63F",
			encrypter:         &utils.PrivateKeyEncrypterMock{},
			wantErrContains:   "private key encrypter passphrase cannot be empty",
		},
		{
			name:              "🎉 Successfully instantiates a new default signature service",
			chAccountStore:    chAccountStore,
			networkPassphrase: network.TestNetworkPassphrase,
			encrypter:         &utils.PrivateKeyEncrypterMock{},
			encrypterPass:     "SCPGNK3MRMXKNWGZ4ET3JZ6RUJIN7FMHT4ASVXDG7YPBL4WKBQNEL63F",
			distributionSeed:  "SCPGNK3MRMXKNWGZ4ET3JZ6RUJIN7FMHT4ASVXDG7YPBL4WKBQNEL63F",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			sigService, err := NewDefaultSignatureService(tc.networkPassphrase, dbConnectionPool, tc.distributionSeed, tc.chAccountStore, tc.encrypter, tc.encrypterPass)
			if tc.wantErrContains != "" {
				require.Error(t, err)
				assert.ErrorContains(t, err, tc.wantErrContains)
				assert.Nil(t, sigService)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, sigService)
			}
		})
	}
}

func Test_DefaultSignatureService_DistributionAccount(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	chAccountStore := &store.ChannelAccountModel{DBConnectionPool: dbConnectionPool}

	// test with the first KP:
	distributionKP, err := keypair.Random()
	require.NoError(t, err)
	defaultSigService, err := NewDefaultSignatureService(network.TestNetworkPassphrase, dbConnectionPool, distributionKP.Seed(), chAccountStore, &utils.PrivateKeyEncrypterMock{}, distributionKP.Seed())
	require.NoError(t, err)
	require.Equal(t, distributionKP.Address(), defaultSigService.DistributionAccount())

	// test with the second KP, to make sure it's changing accordingly:
	distributionKP, err = keypair.Random()
	require.NoError(t, err)
	defaultSigService, err = NewDefaultSignatureService(network.TestNetworkPassphrase, dbConnectionPool, distributionKP.Seed(), chAccountStore, &utils.PrivateKeyEncrypterMock{}, distributionKP.Seed())
	require.NoError(t, err)
	require.Equal(t, distributionKP.Address(), defaultSigService.DistributionAccount())
}

func Test_DefaultSignatureService_NetworkPassphrase(t *testing.T) {
	// test with testnet passphrase
	sigService := &DefaultSignatureService{networkPassphrase: network.TestNetworkPassphrase}
	assert.Equal(t, network.TestNetworkPassphrase, sigService.NetworkPassphrase())

	// test with public network passphrase, to make sure it's changing accordingly
	sigService = &DefaultSignatureService{networkPassphrase: network.PublicNetworkPassphrase}
	assert.Equal(t, network.PublicNetworkPassphrase, sigService.NetworkPassphrase())
}

func Test_DefaultSignatureService_getKPsForAccounts(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()
	chAccountStore := &store.ChannelAccountModel{DBConnectionPool: dbConnectionPool}

	// create distribution account
	distributionKP, err := keypair.Random()
	require.NoError(t, err)

	// create default encrypter
	encrypter := &utils.DefaultPrivateKeyEncrypter{}
	encrypterPass := distributionKP.Seed()

	// create channel accounts in the DB
	channelAccounts := store.CreateChannelAccountFixtures(t, ctx, dbConnectionPool, 2)
	chAccKP1, err := keypair.ParseFull(channelAccounts[0].PrivateKey)
	require.NoError(t, err)
	chAccKP2, err := keypair.ParseFull(channelAccounts[1].PrivateKey)
	require.NoError(t, err)

	// create channel account that's not in the DB
	nonExistentChannelAccountKP, err := keypair.Random()
	require.NoError(t, err)

	// create channel account with encrypted private key
	decryptableKeyChAccKP, err := keypair.Random()
	require.NoError(t, err)
	decryptableKeyChAccKPSeed, err := encrypter.Encrypt(decryptableKeyChAccKP.Seed(), encrypterPass)
	require.NoError(t, err)
	err = chAccountStore.Insert(ctx, chAccountStore.DBConnectionPool, decryptableKeyChAccKP.Address(), decryptableKeyChAccKPSeed)
	require.NoError(t, err)

	// create Channel account with private key encrypted by a different passphrase
	undecryptableKeyChAccKP, err := keypair.Random()
	require.NoError(t, err)
	undecryptableKeyChAccKPSeed, err := encrypter.Encrypt(undecryptableKeyChAccKP.Seed(), keypair.MustRandom().Seed())
	require.NoError(t, err)
	err = chAccountStore.Insert(ctx, chAccountStore.DBConnectionPool, undecryptableKeyChAccKP.Address(), undecryptableKeyChAccKPSeed)
	require.NoError(t, err)

	// create default signature service
	defaultSigService, err := NewDefaultSignatureService(network.TestNetworkPassphrase, dbConnectionPool, distributionKP.Seed(), chAccountStore, encrypter, encrypterPass)
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
			name:         "🎉 Successfully returns the distribution KP",
			accounts:     []string{distributionKP.Address()},
			wantKeypairs: []*keypair.Full{distributionKP},
		},
		{
			name:         "🎉 Successfully one result if there are repeated values in the input array",
			accounts:     []string{distributionKP.Address(), distributionKP.Address(), chAccKP1.Address(), chAccKP1.Address()},
			wantKeypairs: []*keypair.Full{distributionKP, chAccKP1},
		},
		{
			name:         "🎉 Successfully returns distribution and channel accounts KPs, for unencrypted seeds",
			accounts:     []string{distributionKP.Address(), chAccKP1.Address(), chAccKP2.Address()},
			wantKeypairs: []*keypair.Full{distributionKP, chAccKP1, chAccKP2},
		},
		{
			name:         "🎉 Successfully returns distribution and channel accounts KPs, with 1 encrypted seed",
			accounts:     []string{distributionKP.Address(), chAccKP1.Address(), chAccKP2.Address(), decryptableKeyChAccKP.Address()},
			wantKeypairs: []*keypair.Full{distributionKP, chAccKP1, chAccKP2, decryptableKeyChAccKP},
		},
		{
			name:            "return an error if one of the encrypted seeds cannot be decrypted with the expected passphrase",
			accounts:        []string{undecryptableKeyChAccKP.Address()},
			wantErrContains: "cannot decrypt private key: cipher: message authentication failed",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			kps, err := defaultSigService.getKPsForAccounts(ctx, tc.accounts...)
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

func Test_DefaultSignatureService_SignStellarTransaction(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	chAccountStore := &store.ChannelAccountModel{DBConnectionPool: dbConnectionPool}
	ctx := context.Background()

	// create channel accounts in the DB
	channelAccounts := store.CreateChannelAccountFixtures(t, ctx, dbConnectionPool, 1)
	chAccKP, err := keypair.ParseFull(channelAccounts[0].PrivateKey)
	require.NoError(t, err)

	// create distribution account
	distributionKP, err := keypair.Random()
	require.NoError(t, err)

	defaultSigService, err := NewDefaultSignatureService(network.TestNetworkPassphrase, dbConnectionPool, distributionKP.Seed(), chAccountStore, &utils.DefaultPrivateKeyEncrypter{}, distributionKP.Seed())
	require.NoError(t, err)

	// create stellar transaction
	chSourceAccount := txnbuild.NewSimpleAccount(chAccKP.Address(), int64(9605939170639897))
	stellarTx, err := txnbuild.NewTransaction(
		txnbuild.TransactionParams{
			SourceAccount:        &chSourceAccount,
			IncrementSequenceNum: true,
			Operations: []txnbuild.Operation{&txnbuild.Payment{
				Destination:   "GCCOBXW2XQNUSL467IEILE6MMCNRR66SSVL4YQADUNYYNUVREF3FIV2Z",
				Amount:        "10",
				Asset:         txnbuild.NativeAsset{},
				SourceAccount: distributionKP.Address(),
			}},
			BaseFee:       txnbuild.MinBaseFee,
			Preconditions: txnbuild.Preconditions{TimeBounds: txnbuild.NewTimeout(60)},
		},
	)
	require.NoError(t, err)

	wantSignedStellarTx, err := stellarTx.Sign(network.TestNetworkPassphrase, distributionKP, chAccKP)
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
			accounts:            []string{distributionKP.Address(), chAccKP.Address()},
			wantSignedStellarTx: wantSignedStellarTx,
		},
		{
			name:                "🎉 Successfully sign transaction when all some address are repeated",
			stellarTx:           stellarTx,
			accounts:            []string{distributionKP.Address(), chAccKP.Address(), chAccKP.Address()},
			wantSignedStellarTx: wantSignedStellarTx,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			gotSignedStellarTx, err := defaultSigService.SignStellarTransaction(ctx, tc.stellarTx, tc.accounts...)
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

func Test_DefaultSignatureService_SignFeeBumpStellarTransaction(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	chAccountStore := &store.ChannelAccountModel{DBConnectionPool: dbConnectionPool}
	ctx := context.Background()

	// create channel accounts in the DB
	channelAccounts := store.CreateChannelAccountFixtures(t, ctx, dbConnectionPool, 1)
	chAccKP, err := keypair.ParseFull(channelAccounts[0].PrivateKey)
	require.NoError(t, err)

	// create distribution account
	distributionKP, err := keypair.Random()
	require.NoError(t, err)

	defaultSigService, err := NewDefaultSignatureService(network.TestNetworkPassphrase, dbConnectionPool, distributionKP.Seed(), chAccountStore, &utils.DefaultPrivateKeyEncrypter{}, distributionKP.Seed())
	require.NoError(t, err)

	// create stellar transaction
	chSourceAccount := txnbuild.NewSimpleAccount(chAccKP.Address(), int64(9605939170639897))
	stellarTx, err := txnbuild.NewTransaction(
		txnbuild.TransactionParams{
			SourceAccount:        &chSourceAccount,
			IncrementSequenceNum: true,
			Operations: []txnbuild.Operation{&txnbuild.Payment{
				Destination:   "GCCOBXW2XQNUSL467IEILE6MMCNRR66SSVL4YQADUNYYNUVREF3FIV2Z",
				Amount:        "10",
				Asset:         txnbuild.NativeAsset{},
				SourceAccount: distributionKP.Address(),
			}},
			BaseFee:       txnbuild.MinBaseFee,
			Preconditions: txnbuild.Preconditions{TimeBounds: txnbuild.NewTimeout(60)},
		},
	)
	require.NoError(t, err)
	signedStellarTx, err := stellarTx.Sign(network.TestNetworkPassphrase, distributionKP, chAccKP)
	require.NoError(t, err)

	feeBumpStellarTx, err := txnbuild.NewFeeBumpTransaction(
		txnbuild.FeeBumpTransactionParams{
			Inner:      signedStellarTx,
			FeeAccount: distributionKP.Address(),
			BaseFee:    txnbuild.MinBaseFee,
		},
	)
	require.NoError(t, err)

	wantSignedFeeBumpStellarTx, err := feeBumpStellarTx.Sign(network.TestNetworkPassphrase, distributionKP)
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
			accounts:                   []string{distributionKP.Address()},
			wantSignedFeeBumpStellarTx: wantSignedFeeBumpStellarTx,
		},
		{
			name:                       "🎉 Successfully sign transaction when all some address are repeated",
			feeBumpStellarTx:           feeBumpStellarTx,
			accounts:                   []string{distributionKP.Address(), distributionKP.Address()},
			wantSignedFeeBumpStellarTx: wantSignedFeeBumpStellarTx,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			gotSignedFeeBumpStellarTx, err := defaultSigService.SignFeeBumpStellarTransaction(ctx, tc.feeBumpStellarTx, tc.accounts...)
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

func Test_DefaultSignatureService_BatchInsert(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()
	chAccountStore := &store.ChannelAccountModel{DBConnectionPool: dbConnectionPool}

	distributionKP, err := keypair.Random()
	require.NoError(t, err)

	signerKP1 := keypair.MustRandom()
	signerKP2 := keypair.MustRandom()

	testCase := []struct {
		name              string
		shouldEncryptSeed bool
		kps               []*keypair.Full
		wantErrContains   string
	}{
		{
			name:            "if KPs is empty, return an error",
			wantErrContains: "no keypairs provided",
		},
		{
			name:              "🎉 successfully bulk insert without encryption",
			shouldEncryptSeed: false,
			kps:               []*keypair.Full{signerKP1, signerKP2},
		},
		{
			name:              "🎉 successfully bulk insert with encryption",
			shouldEncryptSeed: true,
			kps:               []*keypair.Full{signerKP1, signerKP2},
		},
	}

	type comparableChAccount struct {
		PublicKey  string
		PrivateKey string
	}

	defaultEncrypter := &utils.DefaultPrivateKeyEncrypter{}
	encrypterPass := distributionKP.Seed()
	defaultSigService, err := NewDefaultSignatureService(network.TestNetworkPassphrase, dbConnectionPool, distributionKP.Seed(), chAccountStore, defaultEncrypter, encrypterPass)
	require.NoError(t, err)

	for _, tc := range testCase {
		t.Run(tc.name, func(t *testing.T) {
			count, err := chAccountStore.Count(ctx)
			require.NoError(t, err)
			require.Equal(t, 0, count, "this test should have started with 0 channel accounts")

			err = defaultSigService.BatchInsert(ctx, tc.kps, tc.shouldEncryptSeed, 0)
			if tc.wantErrContains != "" {
				require.Error(t, err)
				assert.ErrorContains(t, err, tc.wantErrContains)
			} else {
				require.NoError(t, err)

				allChAccounts, err := chAccountStore.GetAll(ctx, dbConnectionPool, math.MaxInt32, 0)
				require.NoError(t, err)
				assert.Equal(t, len(tc.kps), len(allChAccounts))

				// compare the accounts
				var allChAccountsComparable []comparableChAccount
				for _, chAccount := range allChAccounts {
					publicKey := chAccount.PublicKey
					privateKey := chAccount.PrivateKey

					if tc.shouldEncryptSeed {
						privateKey, err = defaultEncrypter.Decrypt(privateKey, encrypterPass)
						require.NoError(t, err)
					}

					allChAccountsComparable = append(allChAccountsComparable, comparableChAccount{
						PublicKey:  publicKey,
						PrivateKey: privateKey,
					})
				}

				var tcChAccountsComparable []comparableChAccount
				for _, kp := range tc.kps {
					tcChAccountsComparable = append(tcChAccountsComparable, comparableChAccount{
						PublicKey:  kp.Address(),
						PrivateKey: kp.Seed(),
					})
				}
				assert.ElementsMatch(t, tcChAccountsComparable, allChAccountsComparable)
			}

			store.DeleteAllFromChannelAccounts(t, ctx, dbConnectionPool)
		})
	}
}

func Test_DefaultSignatureService_Delete(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()
	chAccountStore := &store.ChannelAccountModel{DBConnectionPool: dbConnectionPool}

	distributionKP, err := keypair.Random()
	require.NoError(t, err)
	defaultSigService, err := NewDefaultSignatureService(network.TestNetworkPassphrase, dbConnectionPool, distributionKP.Seed(), chAccountStore, &utils.PrivateKeyEncrypterMock{}, distributionKP.Seed())
	require.NoError(t, err)

	// at start: count=0
	count, err := chAccountStore.Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, 0, count)

	// create 2 accounts: count=0->2
	channelAccounts := store.CreateChannelAccountFixtures(t, ctx, dbConnectionPool, 2)
	count, err = chAccountStore.Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, 2, count)

	currLedgerNumber := 0
	lockUntilLedgerNumber := 10
	for _, chAcc := range channelAccounts {
		_, err = chAccountStore.Lock(ctx, chAccountStore.DBConnectionPool, chAcc.PublicKey, int32(currLedgerNumber), int32(lockUntilLedgerNumber))
		require.NoError(t, err)
	}

	// delete one account: count=2->1
	err = defaultSigService.Delete(ctx, channelAccounts[0].PublicKey, lockUntilLedgerNumber)
	require.NoError(t, err)
	count, err = chAccountStore.Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, count)

	// delete another account: count=1->0
	err = defaultSigService.Delete(ctx, channelAccounts[1].PublicKey, lockUntilLedgerNumber)
	require.NoError(t, err)
	count, err = chAccountStore.Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, 0, count)

	// delete non-existing account: error expected
	err = defaultSigService.Delete(ctx, "non-existent-account", 0)
	require.Error(t, err)
	assert.ErrorIs(t, err, store.ErrRecordNotFound)
}
