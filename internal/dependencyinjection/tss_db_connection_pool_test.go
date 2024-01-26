package dependencyinjection

import (
	"context"
	"testing"

	"github.com/stellar/stellar-disbursement-platform-backend/db/dbtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_dependencyinjection_NewTSSDBConnectionPool(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	ctx := context.Background()
	t.Run("should create and return the same instance on the second call", func(t *testing.T) {
		ClearInstancesTestHelper(t)

		opts := TSSDBConnectionPoolOptions{DatabaseURL: dbt.DSN}

		gotDependency, err := NewTSSDBConnectionPool(ctx, opts)
		require.NoError(t, err)
		defer gotDependency.Close()

		gotDependencyDuplicate, err := NewTSSDBConnectionPool(ctx, opts)
		require.NoError(t, err)

		assert.Equal(t, &gotDependency, &gotDependencyDuplicate)
	})

	t.Run("should return an error on a invalid option", func(t *testing.T) {
		ClearInstancesTestHelper(t)

		opts := TSSDBConnectionPoolOptions{}
		gotDependency, err := NewTSSDBConnectionPool(ctx, opts)
		assert.Nil(t, gotDependency)
		assert.ErrorContains(t, err, "opening TSS DB connection pool: error pinging app DB connection pool")
	})

	t.Run("should return an error if there's an invalid instance pre-stored", func(t *testing.T) {
		ClearInstancesTestHelper(t)

		setInstance(tssDBConnectionPoolInstanceName, false)

		opts := TSSDBConnectionPoolOptions{DatabaseURL: dbt.DSN}
		gotDependency, err := NewTSSDBConnectionPool(ctx, opts)
		assert.Nil(t, gotDependency)
		assert.EqualError(t, err, "trying to cast TSS DBConnectionPool client for depencency injection")
	})
}
