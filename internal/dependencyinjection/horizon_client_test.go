package dependencyinjection

import (
	"context"
	"testing"

	"github.com/stellar/stellar-disbursement-platform-backend/db/dbtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_dependencyinjection_NewHorizonClient(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	ctx := context.Background()
	t.Run("should create and return the same instance on the second call", func(t *testing.T) {
		ClearInstancesTestHelper(t)

		horizonURL := "https://horizon-testnet.stellar.org"
		gotDependency, err := NewHorizonClient(ctx, horizonURL)
		require.NoError(t, err)

		gotDependencyDuplicate, err := NewHorizonClient(ctx, horizonURL)
		require.NoError(t, err)

		assert.Equal(t, &gotDependency, &gotDependencyDuplicate)
	})

	t.Run("should return an error if there's an invalid instance pre-stored", func(t *testing.T) {
		ClearInstancesTestHelper(t)

		SetInstance(horizonClientInstanceName, false)

		horizonURL := "https://horizon-testnet.stellar.org"
		gotDependency, err := NewHorizonClient(ctx, horizonURL)
		assert.Nil(t, gotDependency)
		assert.EqualError(t, err, "trying to cast an existing horizon client instance")
	})
}
