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

func Test_SignatureClientType_DistributionAccountType(t *testing.T) {
	testCases := []struct {
		signatureClientType         SignatureClientType
		wantErrContains             string
		wantDistributionAccountType schema.AccountType
	}{
		{
			signatureClientType: ChannelAccountDBSignatureClientType,
			wantErrContains:     fmt.Sprintf("invalid distribution account type %q", ChannelAccountDBSignatureClientType),
		},
		{
			signatureClientType:         DistributionAccountEnvSignatureClientType,
			wantDistributionAccountType: schema.DistributionAccountStellarEnv,
		},
		{
			signatureClientType:         DistributionAccountDBSignatureClientType,
			wantDistributionAccountType: schema.DistributionAccountStellarDBVault,
		},
		{
			signatureClientType: HostAccountEnvSignatureClientType,
			wantErrContains:     fmt.Sprintf("invalid distribution account type %q", HostAccountEnvSignatureClientType),
		},
	}

	for _, tc := range testCases {
		t.Run(string(tc.signatureClientType), func(t *testing.T) {
			distAccType, err := tc.signatureClientType.AccountType()
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

func Test_ParseSignatureClientType(t *testing.T) {
	testCases := []struct {
		sigServiceTypeStr     string
		expectedSigClientType SignatureClientType
		wantErr               error
	}{
		{wantErr: fmt.Errorf(`invalid signature client type ""`)},
		{sigServiceTypeStr: "INVALID", wantErr: fmt.Errorf(`invalid signature client type "INVALID"`)},
		{sigServiceTypeStr: "CHANNEL_ACCOUNT_DB", expectedSigClientType: ChannelAccountDBSignatureClientType},
		{sigServiceTypeStr: "DISTRIBUTION_ACCOUNT_ENV", expectedSigClientType: DistributionAccountEnvSignatureClientType},
		{sigServiceTypeStr: "HOST_ACCOUNT_ENV", expectedSigClientType: HostAccountEnvSignatureClientType},
	}

	for _, tc := range testCases {
		t.Run("signatureServiceTypeType: "+tc.sigServiceTypeStr, func(t *testing.T) {
			sigServiceType, err := ParseSignatureClientType(tc.sigServiceTypeStr)
			assert.Equal(t, tc.expectedSigClientType, sigServiceType)
			assert.Equal(t, tc.wantErr, err)
		})
	}
}

func Test_ParseSignatureClientDistributionType(t *testing.T) {
	testCases := []struct {
		sigServiceTypeStr     string
		expectedSigClientType SignatureClientType
		wantErr               error
	}{
		{wantErr: fmt.Errorf(`invalid signature client distribution type ""`)},
		{sigServiceTypeStr: "INVALID", wantErr: fmt.Errorf(`invalid signature client distribution type "INVALID"`)},
		{sigServiceTypeStr: "CHANNEL_ACCOUNT_DB", wantErr: fmt.Errorf(`invalid signature client distribution type "CHANNEL_ACCOUNT_DB"`)},
		{sigServiceTypeStr: "HOST_ACCOUNT_ENV", wantErr: fmt.Errorf(`invalid signature client distribution type "HOST_ACCOUNT_ENV"`)},
		{sigServiceTypeStr: "DISTRIBUTION_ACCOUNT_ENV", expectedSigClientType: DistributionAccountEnvSignatureClientType},
	}

	for _, tc := range testCases {
		t.Run("signatureServiceTypeType: "+tc.sigServiceTypeStr, func(t *testing.T) {
			sigServiceType, err := ParseSignatureClientDistributionType(tc.sigServiceTypeStr)
			assert.Equal(t, tc.expectedSigClientType, sigServiceType)
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
		sigType      SignatureClientType
		opts         SignatureClientOptions
		wantResult   SignatureClient
		wantErrorMsg string
	}{
		{
			name:         "invalid signature client type",
			sigType:      SignatureClientType("INVALID"),
			opts:         SignatureClientOptions{},
			wantErrorMsg: "invalid signature client type: INVALID",
		},
		{
			name:    "🎉 successfully instantiate a ChannelAccountDB instance",
			sigType: ChannelAccountDBSignatureClientType,
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
			sigType: DistributionAccountDBSignatureClientType,
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
			sigType: DistributionAccountEnvSignatureClientType,
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
			sigService, err := NewSignatureClient(tc.sigType, tc.opts)
			if tc.wantErrorMsg != "" {
				assert.EqualError(t, err, tc.wantErrorMsg)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tc.wantResult, sigService)
			}
		})
	}
}
