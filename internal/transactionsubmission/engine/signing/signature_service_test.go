package signing

import (
	"testing"

	"github.com/stellar/go/keypair"
	"github.com/stellar/go/network"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/db/dbtest"
	preconditionsMocks "github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine/preconditions/mocks"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine/signing/mocks"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/store"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/utils"
	"github.com/stellar/stellar-disbursement-platform-backend/pkg/schema"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/pkg/tenant"
)

func Test_SignatureService_Validate(t *testing.T) {
	testCases := []struct {
		name            string
		getSigServiceFn func(mSignerRouter *mocks.MockSignerRouter, mDistAccResolver *mocks.MockDistributionAccountResolver) SignatureService
		wantErrContains string
	}{
		{
			name: "signerRouter cannot be nil",
			getSigServiceFn: func(_ *mocks.MockSignerRouter, _ *mocks.MockDistributionAccountResolver) SignatureService {
				return SignatureService{}
			},
			wantErrContains: "signer router cannot be nil",
		},
		{
			name: "signerRouter cannot be empty",
			getSigServiceFn: func(mSignerRouter *mocks.MockSignerRouter, _ *mocks.MockDistributionAccountResolver) SignatureService {
				mSignerRouter.
					On("SupportedAccountTypes").
					Return([]schema.AccountType{}).
					Once()
				return SignatureService{
					SignerRouter: mSignerRouter,
				}
			},
			wantErrContains: "signer router must support at least one account type",
		},
		{
			name: "DistributionAccountResolver cannot be nil",
			getSigServiceFn: func(mSignerRouter *mocks.MockSignerRouter, _ *mocks.MockDistributionAccountResolver) SignatureService {
				mSignerRouter.
					On("SupportedAccountTypes").
					Return([]schema.AccountType{schema.DistributionAccountStellarEnv}).
					Once()
				return SignatureService{
					SignerRouter: mSignerRouter,
				}
			},
			wantErrContains: "distribution account resolver cannot be nil",
		},
		{
			name: "🎉 successfully validates object",
			getSigServiceFn: func(mSignerRouter *mocks.MockSignerRouter, mDistAccResolver *mocks.MockDistributionAccountResolver) SignatureService {
				mSignerRouter.
					On("SupportedAccountTypes").
					Return([]schema.AccountType{schema.DistributionAccountStellarEnv}).
					Once()
				return SignatureService{
					SignerRouter:                mSignerRouter,
					DistributionAccountResolver: mDistAccResolver,
				}
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// prepareMocks
			mSignerRouter := mocks.NewMockSignerRouter(t)
			mDistAccResolver := mocks.NewMockDistributionAccountResolver(t)

			sigService := tc.getSigServiceFn(mSignerRouter, mDistAccResolver)

			err := sigService.Validate()
			if tc.wantErrContains == "" {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, tc.wantErrContains)
			}
		})
	}
}

func Test_NewSignatureService(t *testing.T) {
	dbt := dbtest.OpenWithoutMigrations(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	chAccEncryptionPassphrase := keypair.MustRandom().Seed()
	distAccEncryptionPassphrase := keypair.MustRandom().Seed()
	mLedgerNumberTracker := preconditionsMocks.NewMockLedgerNumberTracker(t)
	distributionKP := keypair.MustRandom()

	wantChAccountSigner := &ChannelAccountDBSignatureClient{
		networkPassphrase:    network.TestNetworkPassphrase,
		dbConnectionPool:     dbConnectionPool,
		encryptionPassphrase: chAccEncryptionPassphrase,
		ledgerNumberTracker:  mLedgerNumberTracker,
		chAccModel:           store.NewChannelAccountModel(dbConnectionPool),
		encrypter:            &utils.DefaultPrivateKeyEncrypter{},
	}
	wantDistAccountEnvSigner := &DistributionAccountEnvSignatureClient{
		networkPassphrase:   network.TestNetworkPassphrase,
		distributionAccount: distributionKP.Address(),
		distributionKP:      distributionKP,
	}
	wantHostAccountEnvSigner := wantDistAccountEnvSigner
	wantDistAccountDBSigner := &DistributionAccountDBSignatureClient{
		networkPassphrase:    network.TestNetworkPassphrase,
		encryptionPassphrase: distAccEncryptionPassphrase,
		dbVault:              store.NewDBVaultModel(dbConnectionPool),
		encrypter:            &utils.DefaultPrivateKeyEncrypter{},
	}
	wantSigRouterStrategies := map[schema.AccountType]SignatureClient{
		schema.HostStellarEnv:                    wantHostAccountEnvSigner,
		schema.ChannelAccountStellarDB:           wantChAccountSigner,
		schema.DistributionAccountStellarEnv:     wantDistAccountEnvSigner,
		schema.DistributionAccountStellarDBVault: wantDistAccountDBSigner,
	}

	wantDistAccountResolver := &DistributionAccountResolverImpl{
		tenantManager:                 tenant.NewManager(tenant.WithDatabase(dbConnectionPool)),
		hostDistributionAccountPubKey: distributionKP.Address(),
	}

	testCases := []struct {
		name            string
		opts            SignatureServiceOptions
		wantErrContains string
		wantSigService  SignatureService
	}{
		{
			name: "returns an error if the distribution account resolver is nil",
			opts: SignatureServiceOptions{
				DistributionSignerType: DistributionAccountEnvSignatureClientType,
			},
			wantErrContains: "distribution account resolver cannot be nil",
		},
		{
			name: "returns an error if the options are invalid for the NewSignerRouter method",
			opts: SignatureServiceOptions{
				DistributionSignerType:      DistributionAccountEnvSignatureClientType,
				DistributionAccountResolver: wantDistAccountResolver,
			},
			wantErrContains: "creating a new signer router",
		},
		{
			name: "🎉 successfully instantiate new signature service",
			opts: SignatureServiceOptions{
				DistributionSignerType:      DistributionAccountDBSignatureClientType,
				NetworkPassphrase:           network.TestNetworkPassphrase,
				DBConnectionPool:            dbConnectionPool,
				ChAccEncryptionPassphrase:   chAccEncryptionPassphrase,
				LedgerNumberTracker:         mLedgerNumberTracker,
				DistAccEncryptionPassphrase: distAccEncryptionPassphrase,
				DistributionPrivateKey:      distributionKP.Seed(),
				DistributionAccountResolver: wantDistAccountResolver,
			},

			wantSigService: SignatureService{
				SignerRouter: &SignerRouterImpl{
					strategies:        wantSigRouterStrategies,
					networkPassphrase: network.TestNetworkPassphrase,
				},
				DistributionAccountResolver: wantDistAccountResolver,
				networkPassphrase:           network.TestNetworkPassphrase,
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			sigService, err := NewSignatureService(tc.opts)
			if tc.wantErrContains != "" {
				require.ErrorContains(t, err, tc.wantErrContains)
				require.Empty(t, sigService)
			} else {
				require.NoError(t, err)
				require.Equal(t, tc.wantSigService, sigService)
			}
		})
	}
}
