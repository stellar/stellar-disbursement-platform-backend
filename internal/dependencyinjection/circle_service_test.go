package dependencyinjection

import (
	"testing"

	"github.com/stellar/go/keypair"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/circle"
	circleMocks "github.com/stellar/stellar-disbursement-platform-backend/internal/circle/mocks"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
)

func Test_NewCircleService(t *testing.T) {
	distAccountPrivateKey := keypair.MustRandom().Seed()
	networkType := utils.TestnetNetworkType
	mCircleClientConfigModel := circleMocks.NewMockClientConfigModel(t)

	t.Run("should create and return the same instance on the second call", func(t *testing.T) {
		defer ClearInstancesTestHelper(t)

		gotDependency, err := NewCircleService(circle.NewClient,
			mCircleClientConfigModel,
			networkType,
			distAccountPrivateKey)
		require.NoError(t, err)

		gotDependencyDuplicate, err := NewCircleService(circle.NewClient,
			mCircleClientConfigModel,
			networkType,
			distAccountPrivateKey)
		require.NoError(t, err)

		assert.Equal(t, &gotDependency, &gotDependencyDuplicate)
	})

	t.Run("should return an error if there's an invalid instance pre-stored", func(t *testing.T) {
		ClearInstancesTestHelper(t)

		instanceName := CircleServiceInstanceName
		SetInstance(instanceName, false)

		gotDependency, err := NewCircleService(circle.NewClient,
			mCircleClientConfigModel,
			networkType,
			distAccountPrivateKey)
		assert.Empty(t, gotDependency)
		assert.EqualError(t, err, "trying to cast an existing circle service instance")
	})
}
