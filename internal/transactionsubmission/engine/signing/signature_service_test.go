package signing

import (
	"fmt"
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
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/pkg/tenant"
)

func Test_SignatureService_Validate(t *testing.T) {
	mSignerClient := mocks.NewMockSignatureClient(t)
	mSignerClient.On("NetworkPassphrase").Return(network.TestNetworkPassphrase)
	mSignerClientPubnet := mocks.NewMockSignatureClient(t)
	mSignerClientPubnet.On("NetworkPassphrase").Return(network.PublicNetworkPassphrase)

	testCases := []struct {
		name            string
		sigService      SignatureService
		wantErrContains string
	}{
		{
			name:            "ChAccountSigner cannot be nil",
			sigService:      SignatureService{},
			wantErrContains: "channel account signer cannot be nil",
		},
		{
			name: "DistAccountSigner cannot be nil",
			sigService: SignatureService{
				ChAccountSigner: mSignerClient,
			},
			wantErrContains: "distribution account signer cannot be nil",
		},
		{
			name: "HostAccountSigner cannot be nil",
			sigService: SignatureService{
				ChAccountSigner:   mSignerClient,
				DistAccountSigner: mSignerClient,
			},
			wantErrContains: "host account signer cannot be nil",
		},
		{
			name: "Network passphrases needs to be consistent",
			sigService: SignatureService{
				ChAccountSigner:   mSignerClient,
				DistAccountSigner: mSignerClient,
				HostAccountSigner: mSignerClientPubnet,
			},
			wantErrContains: "network passphrase of all signers should be the same",
		},
		{
			name: "DistributionAccountResolver cannot be nil",
			sigService: SignatureService{
				ChAccountSigner:   mSignerClient,
				DistAccountSigner: mSignerClient,
				HostAccountSigner: mSignerClient,
			},
			wantErrContains: "distribution account resolver cannot be nil",
		},
		{
			name: "🎉 successfully validates object",
			sigService: SignatureService{
				ChAccountSigner:             mSignerClient,
				DistAccountSigner:           mSignerClient,
				HostAccountSigner:           mSignerClient,
				DistributionAccountResolver: mocks.NewMockDistributionAccountResolver(t),
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.sigService.Validate()
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
	wantDistAccountDBSigner := &DistributionAccountDBSignatureClient{
		networkPassphrase:    network.TestNetworkPassphrase,
		encryptionPassphrase: distAccEncryptionPassphrase,
		dbVault:              store.NewDBVaultModel(dbConnectionPool),
		encrypter:            &utils.DefaultPrivateKeyEncrypter{},
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
			name:            "returns an error if the distribution signer type is invalid",
			opts:            SignatureServiceOptions{DistributionSignerType: SignatureClientType("invalid")},
			wantErrContains: `invalid distribution signer type "invalid"`,
		},
		{
			name: "returns an error if the options are invalid for the channel account signer",
			opts: SignatureServiceOptions{
				DistributionSignerType: DistributionAccountEnvSignatureClientType,
			},
			wantErrContains: "creating a new channel account signature client:",
		},
		{
			name: "returns an error if the options are invalid for the distribution account signer (DISTRIBUTION_ACCOUNT_ENV)",
			opts: SignatureServiceOptions{
				DistributionSignerType:    DistributionAccountEnvSignatureClientType,
				NetworkPassphrase:         network.TestNetworkPassphrase,
				DBConnectionPool:          dbConnectionPool,
				ChAccEncryptionPassphrase: chAccEncryptionPassphrase,
				LedgerNumberTracker:       mLedgerNumberTracker,
			},
			wantErrContains: fmt.Sprintf("creating a new distribution account signature client with type %v", DistributionAccountEnvSignatureClientType),
		},
		{
			name: "🎉 successfully instantiate new signature service (DISTRIBUTION_ACCOUNT_ENV)",
			opts: SignatureServiceOptions{
				DistributionSignerType:    DistributionAccountEnvSignatureClientType,
				NetworkPassphrase:         network.TestNetworkPassphrase,
				DBConnectionPool:          dbConnectionPool,
				ChAccEncryptionPassphrase: chAccEncryptionPassphrase,
				LedgerNumberTracker:       mLedgerNumberTracker,

				DistributionPrivateKey: distributionKP.Seed(),
			},

			wantSigService: SignatureService{
				ChAccountSigner:             wantChAccountSigner,
				DistAccountSigner:           wantDistAccountEnvSigner,
				HostAccountSigner:           wantDistAccountEnvSigner,
				DistributionAccountResolver: wantDistAccountResolver,
				networkPassphrase:           network.TestNetworkPassphrase,
			},
		},
		{
			name: "🎉 successfully instantiate new signature service (DISTRIBUTION_ACCOUNT_DB)",
			opts: SignatureServiceOptions{
				DistributionSignerType:    DistributionAccountDBSignatureClientType,
				NetworkPassphrase:         network.TestNetworkPassphrase,
				DBConnectionPool:          dbConnectionPool,
				ChAccEncryptionPassphrase: chAccEncryptionPassphrase,
				LedgerNumberTracker:       mLedgerNumberTracker,

				DistAccEncryptionPassphrase: distAccEncryptionPassphrase,

				DistributionPrivateKey: distributionKP.Seed(),
			},

			wantSigService: SignatureService{
				ChAccountSigner:             wantChAccountSigner,
				DistAccountSigner:           wantDistAccountDBSigner,
				HostAccountSigner:           wantDistAccountEnvSigner,
				DistributionAccountResolver: wantDistAccountResolver,
				networkPassphrase:           network.TestNetworkPassphrase,
			},
		},
		{
			name: "🎉 successfully instantiate new signature (DISTRIBUTION_ACCOUNT_ENV) with the provided DistributionAccountResolver",
			opts: SignatureServiceOptions{
				DistributionSignerType:    DistributionAccountEnvSignatureClientType,
				NetworkPassphrase:         network.TestNetworkPassphrase,
				DBConnectionPool:          dbConnectionPool,
				ChAccEncryptionPassphrase: chAccEncryptionPassphrase,
				LedgerNumberTracker:       mLedgerNumberTracker,

				DistributionPrivateKey:      distributionKP.Seed(),
				DistributionAccountResolver: mocks.NewMockDistributionAccountResolver(t),
			},

			wantSigService: SignatureService{
				ChAccountSigner:             wantChAccountSigner,
				DistAccountSigner:           wantDistAccountEnvSigner,
				HostAccountSigner:           wantDistAccountEnvSigner,
				DistributionAccountResolver: mocks.NewMockDistributionAccountResolver(t),
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
