package signing

import (
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
