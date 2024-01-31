package dependencyinjection

import (
	"context"
	"testing"

	"github.com/stellar/go/clients/horizonclient"
	"github.com/stellar/stellar-disbursement-platform-backend/db/dbtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_dependencyinjection_NewLedgerNumberTracker(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	ctx := context.Background()
	t.Run("should create and return the same instance on the second call", func(t *testing.T) {
		ClearInstancesTestHelper(t)

		horizonClient := &horizonclient.MockClient{}
		gotDependency, err := NewLedgerNumberTracker(ctx, horizonClient)
		require.NoError(t, err)

		gotDependencyDuplicate, err := NewLedgerNumberTracker(ctx, horizonClient)
		require.NoError(t, err)

		assert.Equal(t, &gotDependency, &gotDependencyDuplicate)
	})

	t.Run("should return an error if there's an invalid instance pre-stored", func(t *testing.T) {
		ClearInstancesTestHelper(t)

		SetInstance(LedgerNumberTrackerInstanceName, false)

		gotDependency, err := NewLedgerNumberTracker(ctx, &horizonclient.MockClient{})
		assert.Nil(t, gotDependency)
		assert.EqualError(t, err, "trying to cast an existing ledger number tracker instance")
	})
}
