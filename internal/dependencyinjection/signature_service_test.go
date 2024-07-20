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
	sigMocks "github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine/signing/mocks"
)

func Test_dependencyinjection_NewSignatureService(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	distributionPrivateKey := keypair.MustRandom().Seed()
	chAccEncryptionPassphrase := keypair.MustRandom().Seed()
	distAccEncryptionPassphrase := keypair.MustRandom().Seed()
	mDistAccResolver := sigMocks.NewMockDistributionAccountResolver(t)

	ctx := context.Background()
	t.Run("should create and return the same instance on the second call", func(t *testing.T) {
		ClearInstancesTestHelper(t)

		opts := signing.SignatureServiceOptions{
			NetworkPassphrase:           network.TestNetworkPassphrase,
			DBConnectionPool:            dbConnectionPool,
			DistributionPrivateKey:      distributionPrivateKey,
			ChAccEncryptionPassphrase:   chAccEncryptionPassphrase,
			LedgerNumberTracker:         preconditionsMocks.NewMockLedgerNumberTracker(t),
			DistAccEncryptionPassphrase: distAccEncryptionPassphrase,
			DistributionAccountResolver: mDistAccResolver,
		}

		gotDependency, err := NewSignatureService(ctx, opts)
		require.NoError(t, err)

		gotDependencyDuplicate, err := NewSignatureService(ctx, opts)
		require.NoError(t, err)

		assert.Equal(t, &gotDependency, &gotDependencyDuplicate)
	})

	t.Run("should return an error on a nil distribution account resolver", func(t *testing.T) {
		ClearInstancesTestHelper(t)

		opts := signing.SignatureServiceOptions{}
		gotDependency, err := NewSignatureService(ctx, opts)
		assert.Empty(t, gotDependency)
		assert.EqualError(t, err, "creating a new signature service instance: distribution account resolver cannot be nil")
	})

	t.Run("should return an error on a invalid option", func(t *testing.T) {
		ClearInstancesTestHelper(t)

		opts := signing.SignatureServiceOptions{
			DistributionAccountResolver: mDistAccResolver,
		}
		gotDependency, err := NewSignatureService(ctx, opts)
		assert.Empty(t, gotDependency)
		assert.ErrorContains(t, err, "creating a new signature service instance:")
		assert.ErrorContains(t, err, ": network passphrase cannot be empty")
	})

	t.Run("should return an error if there's an invalid instance pre-stored", func(t *testing.T) {
		ClearInstancesTestHelper(t)

		instanceName := SignatureServiceInstanceName
		SetInstance(instanceName, false)

		opts := signing.SignatureServiceOptions{
			NetworkPassphrase:           network.TestNetworkPassphrase,
			DBConnectionPool:            dbConnectionPool,
			DistributionPrivateKey:      distributionPrivateKey,
			ChAccEncryptionPassphrase:   chAccEncryptionPassphrase,
			DistAccEncryptionPassphrase: distAccEncryptionPassphrase,
			DistributionAccountResolver: mDistAccResolver,
		}
		gotDependency, err := NewSignatureService(ctx, opts)
		assert.Empty(t, gotDependency)
		assert.EqualError(t, err, "trying to cast an existing signature service instance")
	})
}
