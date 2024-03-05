package dependencyinjection

import (
	"context"
	"testing"

	"github.com/stellar/go/keypair"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/db/dbtest"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine/signing"
)

func Test_dependencyinjection_NewDistributionAccountResolver(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	hostDistAccPublicKey := keypair.MustRandom().Address()

	ctx := context.Background()
	t.Run("should create and return the same instance on the second call", func(t *testing.T) {
		defer ClearInstancesTestHelper(t)

		opts := signing.DistributionAccountResolverOptions{
			AdminDBConnectionPool:            dbConnectionPool,
			HostDistributionAccountPublicKey: hostDistAccPublicKey,
		}

		gotDependency, err := NewDistributionAccountResolver(ctx, opts)
		require.NoError(t, err)

		gotDependencyDuplicate, err := NewDistributionAccountResolver(ctx, opts)
		require.NoError(t, err)

		assert.Equal(t, &gotDependency, &gotDependencyDuplicate)
	})

	t.Run("should return an error if the provided options are invalid", func(t *testing.T) {
		defer ClearInstancesTestHelper(t)

		opts := signing.DistributionAccountResolverOptions{}
		gotDependency, err := NewDistributionAccountResolver(ctx, opts)
		assert.Empty(t, gotDependency)
		assert.ErrorContains(t, err, "creating a new distribution account resolver instance")
		assert.ErrorContains(t, err, "validating config in NewDistributionAccountResolver")
		assert.ErrorContains(t, err, "AdminDBConnectionPool cannot be nil")
	})

	t.Run("should return an error if there's an invalid instance pre-stored", func(t *testing.T) {
		ClearInstancesTestHelper(t)

		instanceName := DistributionAccountResolverInstanceName
		SetInstance(instanceName, false)

		opts := signing.DistributionAccountResolverOptions{
			AdminDBConnectionPool:            dbConnectionPool,
			HostDistributionAccountPublicKey: hostDistAccPublicKey,
		}
		gotDependency, err := NewDistributionAccountResolver(ctx, opts)
		assert.Empty(t, gotDependency)
		assert.EqualError(t, err, "trying to cast an existing distribution account resolver instance")
	})
}
