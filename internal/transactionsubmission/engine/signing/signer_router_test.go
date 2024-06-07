package signing

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/stellar/go/keypair"
	"github.com/stellar/go/network"
	"github.com/stellar/go/txnbuild"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/db/dbtest"
	preconditionsMocks "github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine/preconditions/mocks"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine/signing/mocks"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/store"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/utils"
	"github.com/stellar/stellar-disbursement-platform-backend/pkg/schema"
)

func Test_NewSignerRouter(t *testing.T) {
	dbt := dbtest.OpenWithoutMigrations(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	// Create valid SignatureRouterOptions
	networkPassphrase := network.TestNetworkPassphrase
	chAccEncryptionPassphrase := keypair.MustRandom().Seed()
	distAccEncryptionPassphrase := keypair.MustRandom().Seed()
	distributionKP := keypair.MustRandom()
	hostKP := keypair.MustRandom()
	mLedgerNumberTracker := preconditionsMocks.NewMockLedgerNumberTracker(t)
	validOptions := SignatureRouterOptions{
		NetworkPassphrase:           networkPassphrase,
		DBConnectionPool:            dbConnectionPool,
		ChAccEncryptionPassphrase:   chAccEncryptionPassphrase,
		LedgerNumberTracker:         mLedgerNumberTracker,
		DistAccEncryptionPassphrase: distAccEncryptionPassphrase,
		DistributionPrivateKey:      distributionKP.Seed(),
		HostPrivateKey:              hostKP.Seed(),
		Encrypter:                   &utils.DefaultPrivateKeyEncrypter{},
	}

	// Create valid SignerClients:
	wantHostAccStellarEnvSigner := &AccountEnvSignatureClient{
		networkPassphrase:   networkPassphrase,
		distributionAccount: hostKP.Address(),
		distributionKP:      hostKP,
	}
	wantChAccStellarDBSigner := &ChannelAccountDBSignatureClient{
		networkPassphrase:    networkPassphrase,
		dbConnectionPool:     dbConnectionPool,
		encryptionPassphrase: chAccEncryptionPassphrase,
		ledgerNumberTracker:  mLedgerNumberTracker,
		chAccModel:           store.NewChannelAccountModel(dbConnectionPool),
		encrypter:            &utils.DefaultPrivateKeyEncrypter{},
	}
	wantDistAccStelarEnvSigner := &AccountEnvSignatureClient{
		networkPassphrase:   networkPassphrase,
		distributionAccount: distributionKP.Address(),
		distributionKP:      distributionKP,
	}
	wantDistAccStellarDBVaultSigner := &DistributionAccountDBVaultSignatureClient{
		networkPassphrase:    networkPassphrase,
		encryptionPassphrase: distAccEncryptionPassphrase,
		dbVault:              store.NewDBVaultModel(dbConnectionPool),
		encrypter:            &utils.DefaultPrivateKeyEncrypter{},
	}

	testCases := []struct {
		name             string
		opts             SignatureRouterOptions
		accountTypes     []schema.AccountType
		wantErrContains  string
		wantSignerRouter SignerRouter
	}{
		{
			name:         "error when HOST.STELLAR.ENV fails to be instantiated",
			accountTypes: []schema.AccountType{schema.HostStellarEnv},
			opts: SignatureRouterOptions{
				NetworkPassphrase: networkPassphrase,
			},
			wantErrContains: `creating a new "HOST.STELLAR.ENV" signature client`,
		},
		{
			name:         "error when CHANNEL_ACCOUNT.STELLAR.DB fails to be instantiated",
			accountTypes: []schema.AccountType{schema.ChannelAccountStellarDB},
			opts: SignatureRouterOptions{
				NetworkPassphrase: networkPassphrase,
			},
			wantErrContains: `creating a new "CHANNEL_ACCOUNT.STELLAR.DB" signature client`,
		},
		{
			name:         "error when DISTRIBUTION_ACCOUNT.STELLAR.ENV fails to be instantiated",
			accountTypes: []schema.AccountType{schema.DistributionAccountStellarEnv},
			opts: SignatureRouterOptions{
				NetworkPassphrase: networkPassphrase,
			},
			wantErrContains: `creating a new "DISTRIBUTION_ACCOUNT.STELLAR.ENV" signature client`,
		},
		{
			name:         "error when DISTRIBUTION_ACCOUNT.STELLAR.DB_VAULT fails to be instantiated",
			accountTypes: []schema.AccountType{schema.DistributionAccountStellarDBVault},
			opts: SignatureRouterOptions{
				NetworkPassphrase: networkPassphrase,
			},
			wantErrContains: `creating a new "DISTRIBUTION_ACCOUNT.STELLAR.DB_VAULT" signature client`,
		},
		{
			name:            "error when an invalid account type is passed",
			accountTypes:    []schema.AccountType{"INVALID"},
			wantErrContains: "cannot find a Stellar signature client for accountType=INVALID",
		},
		{
			name:         "🎉 successfully instantiate new signature router with accountTypes=[HOST.STELLAR.ENV]",
			accountTypes: []schema.AccountType{schema.HostStellarEnv},
			opts:         validOptions,
			wantSignerRouter: &SignerRouterImpl{
				strategies: map[schema.AccountType]SignatureClient{
					schema.HostStellarEnv: wantHostAccStellarEnvSigner,
				},
				networkPassphrase: networkPassphrase,
			},
		},
		{
			name:         "🎉 successfully instantiate new signature router with accountTypes=[CHANNEL_ACCOUNT.STELLAR.DB]",
			accountTypes: []schema.AccountType{schema.ChannelAccountStellarDB},
			opts:         validOptions,
			wantSignerRouter: &SignerRouterImpl{
				strategies: map[schema.AccountType]SignatureClient{
					schema.ChannelAccountStellarDB: wantChAccStellarDBSigner,
				},
				networkPassphrase: networkPassphrase,
			},
		},
		{
			name:         "🎉 successfully instantiate new signature router with accountTypes=[DISTRIBUTION_ACCOUNT.STELLAR.ENV]",
			accountTypes: []schema.AccountType{schema.DistributionAccountStellarEnv},
			opts:         validOptions,
			wantSignerRouter: &SignerRouterImpl{
				strategies: map[schema.AccountType]SignatureClient{
					schema.DistributionAccountStellarEnv: wantDistAccStelarEnvSigner,
				},
				networkPassphrase: networkPassphrase,
			},
		},
		{
			name:         "🎉 successfully instantiate new signature router with accountTypes=[DISTRIBUTION_ACCOUNT.STELLAR.DB_VAULT]",
			accountTypes: []schema.AccountType{schema.DistributionAccountStellarDBVault},
			opts:         validOptions,
			wantSignerRouter: &SignerRouterImpl{
				strategies: map[schema.AccountType]SignatureClient{
					schema.DistributionAccountStellarDBVault: wantDistAccStellarDBVaultSigner,
				},
				networkPassphrase: networkPassphrase,
			},
		},
		{
			name: "🎉 successfully instantiate new signature router with ALL types (non-empty accountTypes parameter)",
			accountTypes: []schema.AccountType{
				schema.HostStellarEnv,
				schema.ChannelAccountStellarDB,
				schema.DistributionAccountStellarEnv,
				schema.DistributionAccountStellarDBVault,
			},
			opts: validOptions,
			wantSignerRouter: &SignerRouterImpl{
				strategies: map[schema.AccountType]SignatureClient{
					schema.HostStellarEnv:                    wantHostAccStellarEnvSigner,
					schema.ChannelAccountStellarDB:           wantChAccStellarDBSigner,
					schema.DistributionAccountStellarEnv:     wantDistAccStelarEnvSigner,
					schema.DistributionAccountStellarDBVault: wantDistAccStellarDBVaultSigner,
				},
				networkPassphrase: networkPassphrase,
			},
		},
		{
			name: "🎉 successfully instantiate new signature router with ALL types (empty accountTypes parameter)",
			opts: validOptions,
			wantSignerRouter: &SignerRouterImpl{
				strategies: map[schema.AccountType]SignatureClient{
					schema.HostStellarEnv:                    wantHostAccStellarEnvSigner,
					schema.ChannelAccountStellarDB:           wantChAccStellarDBSigner,
					schema.DistributionAccountStellarEnv:     wantDistAccStelarEnvSigner,
					schema.DistributionAccountStellarDBVault: wantDistAccStellarDBVaultSigner,
				},
				networkPassphrase: networkPassphrase,
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			sigRouter, err := NewSignerRouter(tc.opts, tc.accountTypes...)

			if tc.wantErrContains != "" {
				assert.ErrorContains(t, err, tc.wantErrContains)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tc.wantSignerRouter, sigRouter)
			}
		})
	}
}

func Test_SignerRouterImpl_RouteSigner(t *testing.T) {
	dbt := dbtest.OpenWithoutMigrations(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	// Create valid SignatureRouterOptions
	networkPassphrase := network.TestNetworkPassphrase
	chAccEncryptionPassphrase := keypair.MustRandom().Seed()
	distAccEncryptionPassphrase := keypair.MustRandom().Seed()
	distributionKP := keypair.MustRandom()
	hostKP := keypair.MustRandom()
	mLedgerNumberTracker := preconditionsMocks.NewMockLedgerNumberTracker(t)
	validOptions := SignatureRouterOptions{
		NetworkPassphrase:           networkPassphrase,
		DBConnectionPool:            dbConnectionPool,
		ChAccEncryptionPassphrase:   chAccEncryptionPassphrase,
		LedgerNumberTracker:         mLedgerNumberTracker,
		DistAccEncryptionPassphrase: distAccEncryptionPassphrase,
		DistributionPrivateKey:      distributionKP.Seed(),
		HostPrivateKey:              hostKP.Seed(),
		Encrypter:                   &utils.DefaultPrivateKeyEncrypter{},
	}

	sigRouter, err := NewSignerRouter(validOptions)
	require.NoError(t, err)

	sigRouterImpl, ok := sigRouter.(*SignerRouterImpl)
	require.True(t, ok)

	testCases := []struct {
		name            string
		accountType     schema.AccountType
		wantErrContains string
		wantSignerType  interface{}
	}{
		{
			name:            "returns an error if an INVALID accountType is provided",
			accountType:     schema.AccountType("INVALID"),
			wantErrContains: `type "INVALID" is not supported by SignerRouter`,
		},
		{
			name:           fmt.Sprintf("🎉 successfully routes to %s", schema.HostStellarEnv),
			accountType:    schema.HostStellarEnv,
			wantSignerType: &AccountEnvSignatureClient{},
		},
		{
			name:           fmt.Sprintf("🎉 successfully routes to %s", schema.ChannelAccountStellarDB),
			accountType:    schema.ChannelAccountStellarDB,
			wantSignerType: &ChannelAccountDBSignatureClient{},
		},
		{
			name:           fmt.Sprintf("🎉 successfully routes to %s", schema.DistributionAccountStellarEnv),
			accountType:    schema.DistributionAccountStellarEnv,
			wantSignerType: &AccountEnvSignatureClient{},
		},
		{
			name:           fmt.Sprintf("🎉 successfully routes to %s", schema.DistributionAccountStellarDBVault),
			accountType:    schema.DistributionAccountStellarDBVault,
			wantSignerType: &DistributionAccountDBVaultSignatureClient{},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			sigClient, err := sigRouterImpl.RouteSigner(tc.accountType)
			if tc.wantErrContains != "" {
				assert.ErrorContains(t, err, tc.wantErrContains)
			} else {
				require.NoError(t, err)
				assert.IsType(t, tc.wantSignerType, sigClient)
			}
		})
	}
}

func Test_SignerRouterImpl_SignStellarTransaction(t *testing.T) {
	dbt := dbtest.OpenWithoutMigrations(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	hostAccount := schema.NewDefaultHostAccount(keypair.MustRandom().Address())
	channelAccount := schema.NewDefaultChannelAccount(keypair.MustRandom().Address())
	distributionDBVaultAccount := schema.NewDefaultStellarTransactionAccount(keypair.MustRandom().Address())
	distributionEnvAccount := schema.TransactionAccount{
		Type:    schema.DistributionAccountStellarEnv,
		Address: keypair.MustRandom().Address(),
		Status:  schema.AccountStatusActive,
	}
	ctx := context.Background()

	testCases := []struct {
		name               string
		accounts           []schema.TransactionAccount
		mockSignerRouterFn func(t *testing.T, sigRouter *SignerRouterImpl)
		wantErrContains    string
	}{
		{
			name:            "returns an error if zero accounts are provided",
			accounts:        []schema.TransactionAccount{},
			wantErrContains: "no accounts provided to sign the transaction",
		},
		{
			name: "returns an error if an INVALID accountType is provided",
			accounts: []schema.TransactionAccount{{
				Address: keypair.MustRandom().Address(),
				Type:    schema.AccountType("INVALID"),
			}},
			wantErrContains: "routing signer",
		},
		{
			name:     fmt.Sprintf("returns an error if the SigClient fails (%s)", schema.HostStellarEnv),
			accounts: []schema.TransactionAccount{hostAccount},
			mockSignerRouterFn: func(t *testing.T, sigRouter *SignerRouterImpl) {
				sigClient := mocks.NewMockSignatureClient(t)
				sigClient.
					On("SignStellarTransaction", ctx, &txnbuild.Transaction{}, hostAccount.Address).
					Return(nil, fmt.Errorf("some error occurred")).
					Once()
				sigRouter.strategies[schema.HostStellarEnv] = sigClient
			},
			wantErrContains: fmt.Sprintf("signing stellar transaction for strategy=%s", schema.HostStellarEnv),
		},
		{
			name:     fmt.Sprintf("🎉 successfully signs for %s", schema.HostStellarEnv),
			accounts: []schema.TransactionAccount{hostAccount},
			mockSignerRouterFn: func(t *testing.T, sigRouter *SignerRouterImpl) {
				sigClient := mocks.NewMockSignatureClient(t)
				sigClient.
					On("SignStellarTransaction", ctx, &txnbuild.Transaction{}, hostAccount.Address).
					Return(&txnbuild.Transaction{}, nil).
					Once()
				sigRouter.strategies[schema.HostStellarEnv] = sigClient
			},
		},
		{
			name:     fmt.Sprintf("🎉 successfully signs for %s", schema.ChannelAccountStellarDB),
			accounts: []schema.TransactionAccount{channelAccount},
			mockSignerRouterFn: func(t *testing.T, sigRouter *SignerRouterImpl) {
				sigClient := mocks.NewMockSignatureClient(t)
				sigClient.
					On("SignStellarTransaction", ctx, &txnbuild.Transaction{}, channelAccount.Address).
					Return(&txnbuild.Transaction{}, nil).
					Once()
				sigRouter.strategies[schema.ChannelAccountStellarDB] = sigClient
			},
		},
		{
			name:     fmt.Sprintf("🎉 successfully signs for %s", schema.DistributionAccountStellarEnv),
			accounts: []schema.TransactionAccount{distributionEnvAccount},
			mockSignerRouterFn: func(t *testing.T, sigRouter *SignerRouterImpl) {
				sigClient := mocks.NewMockSignatureClient(t)
				sigClient.
					On("SignStellarTransaction", ctx, &txnbuild.Transaction{}, distributionEnvAccount.Address).
					Return(&txnbuild.Transaction{}, nil).
					Once()
				sigRouter.strategies[schema.DistributionAccountStellarEnv] = sigClient
			},
		},
		{
			name:     fmt.Sprintf("🎉 successfully signs for %s", schema.DistributionAccountStellarDBVault),
			accounts: []schema.TransactionAccount{distributionDBVaultAccount},
			mockSignerRouterFn: func(t *testing.T, sigRouter *SignerRouterImpl) {
				sigClient := mocks.NewMockSignatureClient(t)
				sigClient.
					On("SignStellarTransaction", ctx, &txnbuild.Transaction{}, distributionDBVaultAccount.Address).
					Return(&txnbuild.Transaction{}, nil).
					Once()
				sigRouter.strategies[schema.DistributionAccountStellarDBVault] = sigClient
			},
		},
		{
			name:     fmt.Sprintf("🎉 successfully signs for multiple signers [%s, %s, %s]", schema.HostStellarEnv, schema.ChannelAccountStellarDB, schema.DistributionAccountStellarDBVault),
			accounts: []schema.TransactionAccount{hostAccount, channelAccount, distributionDBVaultAccount},
			mockSignerRouterFn: func(t *testing.T, sigRouter *SignerRouterImpl) {
				hostSigClient := mocks.NewMockSignatureClient(t)
				hostSigClient.
					On("SignStellarTransaction", ctx, &txnbuild.Transaction{}, hostAccount.Address).
					Return(&txnbuild.Transaction{}, nil).
					Once()
				sigRouter.strategies[schema.HostStellarEnv] = hostSigClient

				chAccSigClient := mocks.NewMockSignatureClient(t)
				chAccSigClient.
					On("SignStellarTransaction", ctx, &txnbuild.Transaction{}, channelAccount.Address).
					Return(&txnbuild.Transaction{}, nil).
					Once()
				sigRouter.strategies[schema.ChannelAccountStellarDB] = chAccSigClient

				distAccDBVaultSigClient := mocks.NewMockSignatureClient(t)
				distAccDBVaultSigClient.
					On("SignStellarTransaction", ctx, &txnbuild.Transaction{}, distributionDBVaultAccount.Address).
					Return(&txnbuild.Transaction{}, nil).
					Once()
				sigRouter.strategies[schema.DistributionAccountStellarDBVault] = distAccDBVaultSigClient
			},
		},
		{
			name:     "returns an error when multoiple signers are used but one of them fails",
			accounts: []schema.TransactionAccount{hostAccount, channelAccount},
			mockSignerRouterFn: func(t *testing.T, sigRouter *SignerRouterImpl) {
				chAccSigClient := mocks.NewMockSignatureClient(t)
				chAccSigClient.
					On("SignStellarTransaction", ctx, &txnbuild.Transaction{}, channelAccount.Address).
					Return(&txnbuild.Transaction{}, nil). // <---- SUCCESS
					Once()
				sigRouter.strategies[schema.ChannelAccountStellarDB] = chAccSigClient

				hostSigClient := mocks.NewMockSignatureClient(t)
				hostSigClient.
					On("SignStellarTransaction", ctx, &txnbuild.Transaction{}, hostAccount.Address).
					Return(nil, errors.New("this one fails")). // <---- FAILS
					Once()
				sigRouter.strategies[schema.HostStellarEnv] = hostSigClient
			},
			wantErrContains: fmt.Sprintf("signing stellar transaction for strategy=%s", schema.HostStellarEnv),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			sigRouterImpl := &SignerRouterImpl{
				strategies: map[schema.AccountType]SignatureClient{},
			}
			if tc.mockSignerRouterFn != nil {
				tc.mockSignerRouterFn(t, sigRouterImpl)
			}

			signedStellarTx, err := sigRouterImpl.SignStellarTransaction(ctx, &txnbuild.Transaction{}, tc.accounts...)
			if tc.wantErrContains != "" {
				assert.ErrorContains(t, err, tc.wantErrContains)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, signedStellarTx)
			}
		})
	}
}

func Test_SignerRouterImpl_SignFeeBumpStellarTransaction(t *testing.T) {
	dbt := dbtest.OpenWithoutMigrations(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	hostAccount := schema.NewDefaultHostAccount(keypair.MustRandom().Address())
	channelAccount := schema.NewDefaultChannelAccount(keypair.MustRandom().Address())
	distributionDBVaultAccount := schema.NewDefaultStellarTransactionAccount(keypair.MustRandom().Address())
	distributionEnvAccount := schema.TransactionAccount{
		Type:    schema.DistributionAccountStellarEnv,
		Address: keypair.MustRandom().Address(),
		Status:  schema.AccountStatusActive,
	}
	ctx := context.Background()

	testCases := []struct {
		name               string
		accounts           []schema.TransactionAccount
		mockSignerRouterFn func(t *testing.T, sigRouter *SignerRouterImpl)
		wantErrContains    string
	}{
		{
			name:            "returns an error if zero accounts are provided",
			accounts:        []schema.TransactionAccount{},
			wantErrContains: "no accounts provided to sign the transaction",
		},
		{
			name: "returns an error if an INVALID accountType is provided",
			accounts: []schema.TransactionAccount{{
				Address: keypair.MustRandom().Address(),
				Type:    schema.AccountType("INVALID"),
			}},
			wantErrContains: "routing signer",
		},
		{
			name:     fmt.Sprintf("returns an error if the SigClient fails (%s)", schema.HostStellarEnv),
			accounts: []schema.TransactionAccount{hostAccount},
			mockSignerRouterFn: func(t *testing.T, sigRouter *SignerRouterImpl) {
				sigClient := mocks.NewMockSignatureClient(t)
				sigClient.
					On("SignFeeBumpStellarTransaction", ctx, &txnbuild.FeeBumpTransaction{}, hostAccount.Address).
					Return(nil, fmt.Errorf("some error occurred")).
					Once()
				sigRouter.strategies[schema.HostStellarEnv] = sigClient
			},
			wantErrContains: fmt.Sprintf("signing stellar fee bump transaction for strategy=%s", schema.HostStellarEnv),
		},
		{
			name:     fmt.Sprintf("🎉 successfully signs for %s", schema.HostStellarEnv),
			accounts: []schema.TransactionAccount{hostAccount},
			mockSignerRouterFn: func(t *testing.T, sigRouter *SignerRouterImpl) {
				sigClient := mocks.NewMockSignatureClient(t)
				sigClient.
					On("SignFeeBumpStellarTransaction", ctx, &txnbuild.FeeBumpTransaction{}, hostAccount.Address).
					Return(&txnbuild.FeeBumpTransaction{}, nil).
					Once()
				sigRouter.strategies[schema.HostStellarEnv] = sigClient
			},
		},
		{
			name:     fmt.Sprintf("🎉 successfully signs for %s", schema.ChannelAccountStellarDB),
			accounts: []schema.TransactionAccount{channelAccount},
			mockSignerRouterFn: func(t *testing.T, sigRouter *SignerRouterImpl) {
				sigClient := mocks.NewMockSignatureClient(t)
				sigClient.
					On("SignFeeBumpStellarTransaction", ctx, &txnbuild.FeeBumpTransaction{}, channelAccount.Address).
					Return(&txnbuild.FeeBumpTransaction{}, nil).
					Once()
				sigRouter.strategies[schema.ChannelAccountStellarDB] = sigClient
			},
		},
		{
			name:     fmt.Sprintf("🎉 successfully signs for %s", schema.DistributionAccountStellarEnv),
			accounts: []schema.TransactionAccount{distributionEnvAccount},
			mockSignerRouterFn: func(t *testing.T, sigRouter *SignerRouterImpl) {
				sigClient := mocks.NewMockSignatureClient(t)
				sigClient.
					On("SignFeeBumpStellarTransaction", ctx, &txnbuild.FeeBumpTransaction{}, distributionEnvAccount.Address).
					Return(&txnbuild.FeeBumpTransaction{}, nil).
					Once()
				sigRouter.strategies[schema.DistributionAccountStellarEnv] = sigClient
			},
		},
		{
			name:     fmt.Sprintf("🎉 successfully signs for %s", schema.DistributionAccountStellarDBVault),
			accounts: []schema.TransactionAccount{distributionDBVaultAccount},
			mockSignerRouterFn: func(t *testing.T, sigRouter *SignerRouterImpl) {
				sigClient := mocks.NewMockSignatureClient(t)
				sigClient.
					On("SignFeeBumpStellarTransaction", ctx, &txnbuild.FeeBumpTransaction{}, distributionDBVaultAccount.Address).
					Return(&txnbuild.FeeBumpTransaction{}, nil).
					Once()
				sigRouter.strategies[schema.DistributionAccountStellarDBVault] = sigClient
			},
		},
		{
			name:     fmt.Sprintf("🎉 successfully signs for multiple signers [%s, %s, %s]", schema.HostStellarEnv, schema.ChannelAccountStellarDB, schema.DistributionAccountStellarDBVault),
			accounts: []schema.TransactionAccount{hostAccount, channelAccount, distributionDBVaultAccount},
			mockSignerRouterFn: func(t *testing.T, sigRouter *SignerRouterImpl) {
				hostSigClient := mocks.NewMockSignatureClient(t)
				hostSigClient.
					On("SignFeeBumpStellarTransaction", ctx, &txnbuild.FeeBumpTransaction{}, hostAccount.Address).
					Return(&txnbuild.FeeBumpTransaction{}, nil).
					Once()
				sigRouter.strategies[schema.HostStellarEnv] = hostSigClient

				chAccSigClient := mocks.NewMockSignatureClient(t)
				chAccSigClient.
					On("SignFeeBumpStellarTransaction", ctx, &txnbuild.FeeBumpTransaction{}, channelAccount.Address).
					Return(&txnbuild.FeeBumpTransaction{}, nil).
					Once()
				sigRouter.strategies[schema.ChannelAccountStellarDB] = chAccSigClient

				distAccDBVaultSigClient := mocks.NewMockSignatureClient(t)
				distAccDBVaultSigClient.
					On("SignFeeBumpStellarTransaction", ctx, &txnbuild.FeeBumpTransaction{}, distributionDBVaultAccount.Address).
					Return(&txnbuild.FeeBumpTransaction{}, nil).
					Once()
				sigRouter.strategies[schema.DistributionAccountStellarDBVault] = distAccDBVaultSigClient
			},
		},
		{
			name:     "returns an error when multoiple signers are used but one of them fails",
			accounts: []schema.TransactionAccount{hostAccount, channelAccount},
			mockSignerRouterFn: func(t *testing.T, sigRouter *SignerRouterImpl) {
				chAccSigClient := mocks.NewMockSignatureClient(t)
				chAccSigClient.
					On("SignFeeBumpStellarTransaction", ctx, &txnbuild.FeeBumpTransaction{}, channelAccount.Address).
					Return(&txnbuild.FeeBumpTransaction{}, nil). // <---- SUCCESS
					Once()
				sigRouter.strategies[schema.ChannelAccountStellarDB] = chAccSigClient

				hostSigClient := mocks.NewMockSignatureClient(t)
				hostSigClient.
					On("SignFeeBumpStellarTransaction", ctx, &txnbuild.FeeBumpTransaction{}, hostAccount.Address).
					Return(nil, errors.New("this one fails")). // <---- FAILS
					Once()
				sigRouter.strategies[schema.HostStellarEnv] = hostSigClient
			},
			wantErrContains: fmt.Sprintf("signing stellar fee bump transaction for strategy=%s", schema.HostStellarEnv),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			sigRouterImpl := &SignerRouterImpl{
				strategies: map[schema.AccountType]SignatureClient{},
			}
			if tc.mockSignerRouterFn != nil {
				tc.mockSignerRouterFn(t, sigRouterImpl)
			}

			signedStellarTx, err := sigRouterImpl.SignFeeBumpStellarTransaction(ctx, &txnbuild.FeeBumpTransaction{}, tc.accounts...)
			if tc.wantErrContains != "" {
				assert.ErrorContains(t, err, tc.wantErrContains)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, signedStellarTx)
			}
		})
	}
}

func Test_SignerRouterImpl_BatchInsert(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()

	// Create valid SignatureRouterOptions
	networkPassphrase := network.TestNetworkPassphrase
	chAccEncryptionPassphrase := keypair.MustRandom().Seed()
	distAccEncryptionPassphrase := keypair.MustRandom().Seed()
	distributionKP := keypair.MustRandom()
	hostKP := keypair.MustRandom()
	mLedgerNumberTracker := preconditionsMocks.NewMockLedgerNumberTracker(t)
	mLedgerNumberTracker.On("GetLedgerNumber").Return(1, nil)
	validOptions := SignatureRouterOptions{
		NetworkPassphrase:           networkPassphrase,
		DBConnectionPool:            dbConnectionPool,
		ChAccEncryptionPassphrase:   chAccEncryptionPassphrase,
		LedgerNumberTracker:         mLedgerNumberTracker,
		DistAccEncryptionPassphrase: distAccEncryptionPassphrase,
		DistributionPrivateKey:      distributionKP.Seed(),
		HostPrivateKey:              hostKP.Seed(),
		Encrypter:                   &utils.DefaultPrivateKeyEncrypter{},
	}

	testCases := []struct {
		name            string
		numAccounts     int
		accountType     schema.AccountType
		wantErrContains string
		// mockSigRouterFn is a function that returns a SignerRouter with mocked SignatureClients. If nil, a SignerRouter (with real signer clients) is created.
		mockSigRouterFn    func(t *testing.T) SignerRouter
		wantResponseLength int
	}{
		{
			name:            "error when the number requestes is smaller than one",
			numAccounts:     0,
			wantErrContains: "number of accounts to insert must be greater than 0",
		},
		{
			name:            "error when an invalid account type is passed",
			numAccounts:     1,
			accountType:     "INVALID",
			wantErrContains: "routing signer",
		},
		{
			name: "error when a signer fails to insert",
			mockSigRouterFn: func(t *testing.T) SignerRouter {
				sigClient := mocks.NewMockSignatureClient(t)
				sigClient.
					On("BatchInsert", ctx, 1).
					Return(nil, errors.New("sig client could not insert account")).
					Once()

				return &SignerRouterImpl{
					strategies: map[schema.AccountType]SignatureClient{
						schema.DistributionAccountStellarDBVault: sigClient,
					},
				}
			},
			numAccounts:     1,
			accountType:     schema.DistributionAccountStellarDBVault,
			wantErrContains: fmt.Sprintf("batch inserting accounts for strategy=%s", schema.DistributionAccountStellarDBVault),
		},
		{
			name:               fmt.Sprintf("🎉 successfully inserts with accountType=%s", schema.HostStellarEnv),
			numAccounts:        2,
			accountType:        schema.HostStellarEnv,
			wantErrContains:    ErrUnsupportedCommand.Error(),
			wantResponseLength: 2,
		},
		{
			name:               fmt.Sprintf("🎉 successfully inserts with accountType=%s", schema.ChannelAccountStellarDB),
			numAccounts:        3,
			accountType:        schema.ChannelAccountStellarDB,
			wantResponseLength: 3,
		},
		{
			name:               fmt.Sprintf("🎉 successfully inserts with accountType=%s", schema.DistributionAccountStellarEnv),
			numAccounts:        4,
			accountType:        schema.DistributionAccountStellarEnv,
			wantErrContains:    ErrUnsupportedCommand.Error(),
			wantResponseLength: 4,
		},
		{
			name:               fmt.Sprintf("🎉 successfully inserts with accountType=%s", schema.DistributionAccountStellarDBVault),
			numAccounts:        5,
			accountType:        schema.DistributionAccountStellarDBVault,
			wantResponseLength: 5,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var sigRouter SignerRouter
			var err error
			if tc.mockSigRouterFn != nil {
				sigRouter = tc.mockSigRouterFn(t)
			} else {
				sigRouter, err = NewSignerRouter(validOptions)
				require.NoError(t, err)
			}

			txAccounts, err := sigRouter.BatchInsert(ctx, tc.accountType, tc.numAccounts)
			if tc.wantErrContains != "" {
				assert.ErrorContains(t, err, tc.wantErrContains)
			} else {
				require.NoError(t, err)
			}
			assert.Len(t, txAccounts, tc.wantResponseLength)
		})
	}
}

func Test_SignerRouterImpl_Delete(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()

	// Create valid SignatureRouterOptions
	networkPassphrase := network.TestNetworkPassphrase
	chAccEncryptionPassphrase := keypair.MustRandom().Seed()
	distAccEncryptionPassphrase := keypair.MustRandom().Seed()
	distributionKP := keypair.MustRandom()
	hostKP := keypair.MustRandom()
	mLedgerNumberTracker := preconditionsMocks.NewMockLedgerNumberTracker(t)
	mLedgerNumberTracker.On("GetLedgerNumber").Return(1, nil)
	validOptions := SignatureRouterOptions{
		NetworkPassphrase:           networkPassphrase,
		DBConnectionPool:            dbConnectionPool,
		ChAccEncryptionPassphrase:   chAccEncryptionPassphrase,
		LedgerNumberTracker:         mLedgerNumberTracker,
		DistAccEncryptionPassphrase: distAccEncryptionPassphrase,
		DistributionPrivateKey:      distributionKP.Seed(),
		HostPrivateKey:              hostKP.Seed(),
		Encrypter:                   &utils.DefaultPrivateKeyEncrypter{},
	}
	sigRouter, err := NewSignerRouter(validOptions)
	require.NoError(t, err)

	// Create accounts:
	hostAccounts, err := sigRouter.BatchInsert(ctx, schema.HostStellarEnv, 1)
	require.ErrorIs(t, err, ErrUnsupportedCommand)
	require.Len(t, hostAccounts, 1)
	hostAccount := hostAccounts[0]

	chAccounts, err := sigRouter.BatchInsert(ctx, schema.ChannelAccountStellarDB, 1)
	require.NoError(t, err)
	require.Len(t, chAccounts, 1)
	chAccount := chAccounts[0]
	chAccModel := store.NewChannelAccountModel(dbConnectionPool)
	numChAccounts, err := chAccModel.Count(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, numChAccounts)

	distEnvAccounts, err := sigRouter.BatchInsert(ctx, schema.DistributionAccountStellarEnv, 1)
	require.ErrorIs(t, err, ErrUnsupportedCommand)
	require.Len(t, distEnvAccounts, 1)
	distEnvAccount := distEnvAccounts[0]

	distDBVaultAccounts, err := sigRouter.BatchInsert(ctx, schema.DistributionAccountStellarDBVault, 1)
	require.NoError(t, err)
	require.Len(t, distDBVaultAccounts, 1)
	distDBVaultAccount := distDBVaultAccounts[0]
	query := "SELECT COUNT(*) FROM vault"
	var numDBVaultAccounts int
	err = dbConnectionPool.GetContext(ctx, &numDBVaultAccounts, query)
	require.NoError(t, err)
	require.Equal(t, 1, numDBVaultAccounts)

	testCases := []struct {
		name            string
		accountToDelete schema.TransactionAccount
		wantErrContains string
		// mockSigRouterFn is a function that returns a SignerRouter with mocked SignatureClients. If nil, a SignerRouter (with real signer clients) is created.
		mockSigRouterFn         func(t *testing.T) SignerRouter
		assertAccountDeletionFn func(t *testing.T)
	}{
		{
			name: "error when an invalid account type is passed",
			accountToDelete: schema.TransactionAccount{
				Address: keypair.MustRandom().Address(),
				Type:    "INVALID",
			},
			wantErrContains: "routing signer",
		},
		{
			name: "error when a signer fails to delete",
			mockSigRouterFn: func(t *testing.T) SignerRouter {
				sigClient := mocks.NewMockSignatureClient(t)
				sigClient.
					On("Delete", ctx, distDBVaultAccount.Address).
					Return(errors.New("sig client could not delete account")).
					Once()

				return &SignerRouterImpl{
					strategies: map[schema.AccountType]SignatureClient{
						schema.DistributionAccountStellarDBVault: sigClient,
					},
				}
			},
			accountToDelete: distDBVaultAccount,
			wantErrContains: fmt.Sprintf("deleting account=%v for strategy=%s", distDBVaultAccount, schema.DistributionAccountStellarDBVault),
		},
		{
			name:            fmt.Sprintf("🎉 successfully deletes account with accpountType=%s", schema.HostStellarEnv),
			accountToDelete: hostAccount,
			wantErrContains: ErrUnsupportedCommand.Error(),
		},
		{
			name:            fmt.Sprintf("🎉 successfully deletes account with accpountType=%s", schema.ChannelAccountStellarDB),
			accountToDelete: chAccount,
			assertAccountDeletionFn: func(t *testing.T) {
				numChAccounts, err = chAccModel.Count(ctx)
				assert.NoError(t, err)
				assert.Equal(t, 0, numChAccounts)
			},
		},
		{
			name:            fmt.Sprintf("🎉 successfully deletes account with accpountType=%s", schema.DistributionAccountStellarEnv),
			accountToDelete: distEnvAccount,
			wantErrContains: ErrUnsupportedCommand.Error(),
		},
		{
			name:            fmt.Sprintf("🎉 successfully deletes account with accpountType=%s", schema.DistributionAccountStellarDBVault),
			accountToDelete: distDBVaultAccount,
			assertAccountDeletionFn: func(t *testing.T) {
				query := "SELECT COUNT(*) FROM vault"
				var numDBVaultAccounts int
				err = dbConnectionPool.GetContext(ctx, &numDBVaultAccounts, query)
				require.NoError(t, err)
				require.Equal(t, 0, numDBVaultAccounts)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			sRouter := sigRouter
			if tc.mockSigRouterFn != nil {
				sRouter = tc.mockSigRouterFn(t)
			}

			err := sRouter.Delete(ctx, tc.accountToDelete)
			if tc.wantErrContains != "" {
				assert.ErrorContains(t, err, tc.wantErrContains)
			} else {
				require.NoError(t, err)
			}

			if tc.assertAccountDeletionFn != nil {
				tc.assertAccountDeletionFn(t)
			}
		})
	}
}

func Test_SignerRouterImpl_NetworkPassphrase(t *testing.T) {
	var sigRouterImpl SignerRouter = &SignerRouterImpl{
		networkPassphrase: network.TestNetworkPassphrase,
	}
	require.Equal(t, network.TestNetworkPassphrase, sigRouterImpl.NetworkPassphrase())

	sigRouterImpl = &SignerRouterImpl{
		networkPassphrase: network.PublicNetworkPassphrase,
	}
	require.Equal(t, network.PublicNetworkPassphrase, sigRouterImpl.NetworkPassphrase())

	sigRouterImpl = &SignerRouterImpl{
		networkPassphrase: network.FutureNetworkPassphrase,
	}
	require.Equal(t, network.FutureNetworkPassphrase, sigRouterImpl.NetworkPassphrase())
}

func Test_SignerRouterImpl_SupportedAccountTypes(t *testing.T) {
	dbt := dbtest.OpenWithoutMigrations(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	// Create valid SignatureRouterOptions
	networkPassphrase := network.TestNetworkPassphrase
	chAccEncryptionPassphrase := keypair.MustRandom().Seed()
	distAccEncryptionPassphrase := keypair.MustRandom().Seed()
	distributionKP := keypair.MustRandom()
	hostKP := keypair.MustRandom()
	mLedgerNumberTracker := preconditionsMocks.NewMockLedgerNumberTracker(t)
	validOptions := SignatureRouterOptions{
		NetworkPassphrase:           networkPassphrase,
		DBConnectionPool:            dbConnectionPool,
		ChAccEncryptionPassphrase:   chAccEncryptionPassphrase,
		LedgerNumberTracker:         mLedgerNumberTracker,
		DistAccEncryptionPassphrase: distAccEncryptionPassphrase,
		DistributionPrivateKey:      distributionKP.Seed(),
		HostPrivateKey:              hostKP.Seed(),
		Encrypter:                   &utils.DefaultPrivateKeyEncrypter{},
	}

	testCases := []struct {
		name          string
		inputTypes    []schema.AccountType
		wantSupported []schema.AccountType
	}{
		{
			name:          "returns all supported account types when no input types are provided",
			wantSupported: []schema.AccountType{schema.HostStellarEnv, schema.ChannelAccountStellarDB, schema.DistributionAccountStellarEnv, schema.DistributionAccountStellarDBVault},
		},
		{
			name:          "returns all supported account types when all input types are provided",
			inputTypes:    []schema.AccountType{schema.HostStellarEnv, schema.ChannelAccountStellarDB, schema.DistributionAccountStellarEnv, schema.DistributionAccountStellarDBVault},
			wantSupported: []schema.AccountType{schema.HostStellarEnv, schema.ChannelAccountStellarDB, schema.DistributionAccountStellarEnv, schema.DistributionAccountStellarDBVault},
		},
		{
			name:          "returns only supported account types when some input types are provided",
			inputTypes:    []schema.AccountType{schema.HostStellarEnv, schema.ChannelAccountStellarDB},
			wantSupported: []schema.AccountType{schema.HostStellarEnv, schema.ChannelAccountStellarDB},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			sigRouter, err := NewSignerRouter(validOptions, tc.inputTypes...)
			require.NoError(t, err)

			sigRouterImpl, ok := sigRouter.(*SignerRouterImpl)
			require.True(t, ok)

			assert.ElementsMatch(t, tc.wantSupported, sigRouterImpl.SupportedAccountTypes())
		})
	}
}
