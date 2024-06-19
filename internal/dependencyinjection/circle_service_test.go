package dependencyinjection

import (
	"context"
	"testing"

	"github.com/stellar/go/keypair"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/circle"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
)

func Test_NewCircleService(t *testing.T) {
	ctx := context.Background()
	opts := circle.ServiceOptions{
		ClientFactory:        circle.NewClient,
		ClientConfigModel:    &circle.ClientConfigModel{},
		NetworkType:          utils.TestnetNetworkType,
		EncryptionPassphrase: keypair.MustRandom().Seed(),
	}

	t.Run("should create and return the same instance on the second call", func(t *testing.T) {
		defer ClearInstancesTestHelper(t)

		gotDependency, err := NewCircleService(ctx, opts)
		require.NoError(t, err)

		gotDependencyDuplicate, err := NewCircleService(ctx, opts)
		require.NoError(t, err)

		assert.Equal(t, &gotDependency, &gotDependencyDuplicate)
	})

	t.Run("should return an error if there's an invalid instance pre-stored", func(t *testing.T) {
		ClearInstancesTestHelper(t)

		instanceName := CircleServiceInstanceName
		SetInstance(instanceName, false)

		gotDependency, err := NewCircleService(ctx, opts)
		assert.Empty(t, gotDependency)
		assert.EqualError(t, err, "trying to cast an existing circle service instance")
	})
}
