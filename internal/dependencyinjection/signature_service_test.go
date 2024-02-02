package dependencyinjection

import (
	"context"
	"testing"

	"github.com/stellar/go/keypair"
	"github.com/stellar/go/network"
	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/db/dbtest"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine"
	preconditionsMocks "github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine/preconditions/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

		opts := engine.SignatureServiceOptions{
			NetworkPassphrase:      network.TestNetworkPassphrase,
			DBConnectionPool:       dbConnectionPool,
			DistributionPrivateKey: distributionPrivateKey,
			EncryptionPassphrase:   encryptionPassphrase,
			Type:                   engine.SignatureServiceTypeDefault,
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

		opts := engine.SignatureServiceOptions{}
		gotDependency, err := NewSignatureService(ctx, opts)
		assert.Nil(t, gotDependency)
		assert.EqualError(t, err, "creating a new signature service instance: invalid signature service type: ")
	})

	t.Run("should return an error on a invalid option", func(t *testing.T) {
		ClearInstancesTestHelper(t)

		opts := engine.SignatureServiceOptions{Type: engine.SignatureServiceTypeDefault}
		gotDependency, err := NewSignatureService(ctx, opts)
		assert.Nil(t, gotDependency)
		assert.EqualError(t, err, "creating a new signature service instance: validating options: network passphrase cannot be empty")
	})

	t.Run("should return an error if there's an invalid instance pre-stored", func(t *testing.T) {
		ClearInstancesTestHelper(t)

		SetInstance(SignatureServiceInstanceName, false)

		opts := engine.SignatureServiceOptions{
			NetworkPassphrase:      network.TestNetworkPassphrase,
			DBConnectionPool:       dbConnectionPool,
			DistributionPrivateKey: distributionPrivateKey,
			EncryptionPassphrase:   encryptionPassphrase,
			Type:                   engine.SignatureServiceTypeDefault,
		}
		gotDependency, err := NewSignatureService(ctx, opts)
		assert.Nil(t, gotDependency)
		assert.EqualError(t, err, "trying to cast an existing signature service instance")
	})
}
