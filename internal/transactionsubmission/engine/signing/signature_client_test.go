package signing

import (
	"fmt"
	"testing"

	"github.com/stellar/go/keypair"
	"github.com/stellar/go/network"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/db/dbtest"
	preconditionsMocks "github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine/preconditions/mocks"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/store"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/utils"
	"github.com/stellar/stellar-disbursement-platform-backend/pkg/schema"
)

func Test_SignatureClientType_AccountType(t *testing.T) {
	testCases := []struct {
		distSigClientType           DistributionSignatureClientType
		wantErrContains             string
		wantDistributionAccountType schema.AccountType
	}{
		{
			distSigClientType: DistributionSignatureClientType("INVALID"),
			wantErrContains:   `invalid distribution account type "INVALID"`,
		},
		{
			distSigClientType:           DistributionAccountEnvSignatureClientType,
			wantDistributionAccountType: schema.DistributionAccountStellarEnv,
		},
		{
			distSigClientType:           DistributionAccountDBSignatureClientType,
			wantDistributionAccountType: schema.DistributionAccountStellarDBVault,
		},
	}

	for _, tc := range testCases {
		t.Run(string(tc.distSigClientType), func(t *testing.T) {
			distAccType, err := tc.distSigClientType.AccountType()
			if tc.wantErrContains != "" {
				require.ErrorContains(t, err, tc.wantErrContains)
				assert.Empty(t, distAccType)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tc.wantDistributionAccountType, distAccType)
			}
		})
	}
}

func Test_ParseDistributionSignatureClientType(t *testing.T) {
	testCases := []struct {
		sigServiceTypeStr         string
		expectedDistSigClientType DistributionSignatureClientType
		wantErr                   error
	}{
		{wantErr: fmt.Errorf(`invalid distribution signature client type ""`)},
		{sigServiceTypeStr: "INVALID", wantErr: fmt.Errorf(`invalid distribution signature client type "INVALID"`)},
		{sigServiceTypeStr: "DISTRIBUTION_ACCOUNT_DB", expectedDistSigClientType: DistributionAccountDBSignatureClientType},
		{sigServiceTypeStr: "DISTRIBUTION_ACCOUNT_ENV", expectedDistSigClientType: DistributionAccountEnvSignatureClientType},
	}

	for _, tc := range testCases {
		t.Run("signatureServiceTypeType: "+tc.sigServiceTypeStr, func(t *testing.T) {
			distSigClientType, err := ParseDistributionSignatureClientType(tc.sigServiceTypeStr)
			assert.Equal(t, tc.expectedDistSigClientType, distSigClientType)
			assert.Equal(t, tc.wantErr, err)
		})
	}
}

func Test_NewSignatureClient(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, outerErr := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, outerErr)
	defer dbConnectionPool.Close()

	encryptionPassphrase := keypair.MustRandom().Seed()
	mLedgerNumberTracker := preconditionsMocks.NewMockLedgerNumberTracker(t)
	distributionKP := keypair.MustRandom()

	testCases := []struct {
		name         string
		accType      schema.AccountType
		opts         SignatureClientOptions
		wantResult   SignatureClient
		wantErrorMsg string
	}{
		{
			name:         "invalid signature client type",
			accType:      schema.AccountType("INVALID"),
			opts:         SignatureClientOptions{},
			wantErrorMsg: "cannot find a Stellar signature client for accountType=INVALID",
		},
		{
			name:    "🎉 successfully instantiate a ChannelAccountDB instance",
			accType: schema.ChannelAccountStellarDB,
			opts: SignatureClientOptions{
				NetworkPassphrase:         network.TestNetworkPassphrase,
				DBConnectionPool:          dbConnectionPool,
				ChAccEncryptionPassphrase: encryptionPassphrase,
				LedgerNumberTracker:       mLedgerNumberTracker,
			},
			wantResult: &ChannelAccountDBSignatureClient{
				chAccModel:           store.NewChannelAccountModel(dbConnectionPool),
				dbConnectionPool:     dbConnectionPool,
				encrypter:            &utils.DefaultPrivateKeyEncrypter{},
				encryptionPassphrase: encryptionPassphrase,
				ledgerNumberTracker:  mLedgerNumberTracker,
				networkPassphrase:    network.TestNetworkPassphrase,
			},
		},
		{
			name:    "🎉 successfully instantiate a DistributionAccountDB",
			accType: schema.DistributionAccountStellarDBVault,
			opts: SignatureClientOptions{
				NetworkPassphrase:           network.TestNetworkPassphrase,
				DBConnectionPool:            dbConnectionPool,
				DistAccEncryptionPassphrase: encryptionPassphrase,
				Encrypter:                   &utils.PrivateKeyEncrypterMock{},
			},
			wantResult: &DistributionAccountDBSignatureClient{
				dbVault:              store.NewDBVaultModel(dbConnectionPool),
				encrypter:            &utils.PrivateKeyEncrypterMock{},
				encryptionPassphrase: encryptionPassphrase,
				networkPassphrase:    network.TestNetworkPassphrase,
			},
		},
		{
			name:    "🎉 successfully instantiate a Distribution Account ENV instance",
			accType: schema.DistributionAccountStellarEnv,
			opts: SignatureClientOptions{
				NetworkPassphrase:      network.TestNetworkPassphrase,
				DistributionPrivateKey: distributionKP.Seed(),
			},
			wantResult: &DistributionAccountEnvSignatureClient{
				networkPassphrase:   network.TestNetworkPassphrase,
				distributionAccount: distributionKP.Address(),
				distributionKP:      distributionKP,
			},
		},
		{
			name:    "🎉 successfully instantiate a Distribution Account ENV instance (HOST)",
			accType: schema.HostStellarEnv,
			opts: SignatureClientOptions{
				NetworkPassphrase:      network.TestNetworkPassphrase,
				DistributionPrivateKey: distributionKP.Seed(),
			},
			wantResult: &DistributionAccountEnvSignatureClient{
				networkPassphrase:   network.TestNetworkPassphrase,
				distributionAccount: distributionKP.Address(),
				distributionKP:      distributionKP,
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			sigService, err := NewSignatureClient(tc.accType, tc.opts)
			if tc.wantErrorMsg != "" {
				assert.EqualError(t, err, tc.wantErrorMsg)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tc.wantResult, sigService)
			}
		})
	}
}
