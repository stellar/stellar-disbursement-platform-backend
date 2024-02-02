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
)

func Test_ParseSignatureClientType(t *testing.T) {
	testCases := []struct {
		sigServiceTypeStr     string
		expectedSigClientType SignatureClientType
		wantErr               error
	}{
		{wantErr: fmt.Errorf(`invalid signature client type ""`)},
		{sigServiceTypeStr: "INVALID", wantErr: fmt.Errorf(`invalid signature client type "INVALID"`)},
		{sigServiceTypeStr: "CHANNEL_ACCOUNT_DB", expectedSigClientType: SignatureClientTypeChannelAccountDB},
		{sigServiceTypeStr: "DISTRIBUTION_ACCOUNT_ENV", expectedSigClientType: SignatureClientTypeDistributionAccountEnv},
	}

	for _, tc := range testCases {
		t.Run("signatureServiceTypeType: "+tc.sigServiceTypeStr, func(t *testing.T) {
			sigServiceType, err := ParseSignatureClientType(tc.sigServiceTypeStr)
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
		opts         SignatureClientOptions
		wantResult   SignatureClient
		wantErrorMsg string
	}{
		{
			name:         "invalid signature client type",
			opts:         SignatureClientOptions{Type: SignatureClientType("INVALID")},
			wantErrorMsg: "invalid signature client type: INVALID",
		},
		{
			name: "🎉 successfully instantiate a Channel Account DB instance",
			opts: SignatureClientOptions{
				Type:                 SignatureClientTypeChannelAccountDB,
				NetworkPassphrase:    network.TestNetworkPassphrase,
				DBConnectionPool:     dbConnectionPool,
				EncryptionPassphrase: encryptionPassphrase,
				LedgerNumberTracker:  mLedgerNumberTracker,
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
			name: "🎉 successfully instantiate a Distribution Account ENV instance",
			opts: SignatureClientOptions{
				Type:                   SignatureClientTypeDistributionAccountEnv,
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
			sigService, err := NewSignatureClient(tc.opts)
			if tc.wantErrorMsg != "" {
				assert.EqualError(t, err, tc.wantErrorMsg)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tc.wantResult, sigService)
			}
		})
	}
}
