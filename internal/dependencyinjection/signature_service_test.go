package dependencyinjection

import (
	"context"
	"testing"

	"github.com/stellar/go/keypair"
	"github.com/stellar/go/network"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/db/dbtest"
	preconditionsMocks "github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine/preconditions/mocks"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine/signing"
)

func Test_dependencyinjection_NewSignatureService(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	distributionPrivateKey := keypair.MustRandom().Seed()
	encryptionPassphrase := keypair.MustRandom().Seed()

	ctx := context.Background()
	t.Run("should create and return the same instance on the second call", func(t *testing.T) {
		ClearInstancesTestHelper(t)

		opts := SignatureServiceOptions{
			NetworkPassphrase:      network.TestNetworkPassphrase,
			DBConnectionPool:       dbConnectionPool,
			DistributionPrivateKey: distributionPrivateKey,
			EncryptionPassphrase:   encryptionPassphrase,
			DistributionSignerType: signing.SignatureClientTypeDistributionAccountEnv,
			LedgerNumberTracker:    preconditionsMocks.NewMockLedgerNumberTracker(t),
		}

		gotDependency, err := NewSignatureService(ctx, opts)
		require.NoError(t, err)

		gotDependencyDuplicate, err := NewSignatureService(ctx, opts)
		require.NoError(t, err)

		assert.Equal(t, &gotDependency, &gotDependencyDuplicate)
	})

	t.Run("should return an error on an invalid sig service type", func(t *testing.T) {
		ClearInstancesTestHelper(t)

		opts := SignatureServiceOptions{}
		gotDependency, err := NewSignatureService(ctx, opts)
		assert.Empty(t, gotDependency)
		assert.EqualError(t, err, "creating a new signature service instance: invalid distribution signer type \"\"")
	})

	t.Run("should return an error on a invalid option", func(t *testing.T) {
		ClearInstancesTestHelper(t)

		opts := SignatureServiceOptions{DistributionSignerType: signing.SignatureClientTypeDistributionAccountEnv}
		gotDependency, err := NewSignatureService(ctx, opts)
		assert.Empty(t, gotDependency)
		assert.ErrorContains(t, err, "creating a new signature service instance:")
		assert.ErrorContains(t, err, ": network passphrase cannot be empty")
	})

	t.Run("should return an error if there's an invalid instance pre-stored", func(t *testing.T) {
		ClearInstancesTestHelper(t)

		distributionSignerType := signing.SignatureClientTypeDistributionAccountEnv
		instanceName := buildSignatureServiceInstanceName(distributionSignerType)
		SetInstance(instanceName, false)

		opts := SignatureServiceOptions{
			DistributionSignerType: distributionSignerType,
			NetworkPassphrase:      network.TestNetworkPassphrase,
			DBConnectionPool:       dbConnectionPool,
			DistributionPrivateKey: distributionPrivateKey,
			EncryptionPassphrase:   encryptionPassphrase,
		}
		gotDependency, err := NewSignatureService(ctx, opts)
		assert.Empty(t, gotDependency)
		assert.EqualError(t, err, "trying to cast an existing signature service instance")
	})
}
