package dependencyinjection

import (
	"context"
	"testing"

	"github.com/stellar/go/clients/horizonclient"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/services"
)

func Test_dependencyinjection_NewDistributionAccountService(t *testing.T) {
	ctx := context.Background()
	mockHorizonClient := &horizonclient.MockClient{}
	svcOpts := services.DistributionAccountServiceOptions{
		HorizonClient: mockHorizonClient,
	}

	t.Run("should create and return the same instance on the second call", func(t *testing.T) {
		defer ClearInstancesTestHelper(t)

		gotDependency, err := NewDistributionAccountService(ctx, svcOpts)
		require.NoError(t, err)

		gotDependencyDuplicate, err := NewDistributionAccountService(ctx, svcOpts)
		require.NoError(t, err)

		assert.Equal(t, &gotDependency, &gotDependencyDuplicate)
	})

	t.Run("should return an error if there's an invalid instance pre-stored", func(t *testing.T) {
		ClearInstancesTestHelper(t)

		SetInstance(DistributionAccountServiceInstanceName, false)

		gotDependency, err := NewDistributionAccountService(ctx, svcOpts)
		assert.Empty(t, gotDependency)
		assert.EqualError(t, err, "trying to cast a new distribution account service instance")
	})
}
