package dependencyinjection

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/db/dbtest"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine/signing"
)

func Test_dependencyinjection_NewTxSubmitterEngine(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	ctx := context.Background()
	t.Run("should create and return the same instance on the second call", func(t *testing.T) {
		ClearInstancesTestHelper(t)

		sigService, _, _, _, _ := signing.NewMockSignatureService(t)
		istanceName := buildSignatureServiceInstanceName(signing.SignatureClientTypeDistributionAccountEnv)
		SetInstance(istanceName, sigService)

		opts := TxSubmitterEngineOptions{
			MaxBaseFee: 100,
			SignatureServiceOptions: signing.SignatureServiceOptions{
				DistributionSignerType: signing.SignatureClientTypeDistributionAccountEnv,
			},
		}
		gotDependency, err := NewTxSubmitterEngine(ctx, opts)
		require.NoError(t, err)

		gotDependencyDuplicate, err := NewTxSubmitterEngine(ctx, opts)
		require.NoError(t, err)

		assert.Equal(t, &gotDependency, &gotDependencyDuplicate)
	})

	t.Run("should return an error if there's an invalid instance pre-stored", func(t *testing.T) {
		ClearInstancesTestHelper(t)

		SetInstance(TxSubmitterEngineInstanceName, false)

		opts := TxSubmitterEngineOptions{}
		gotDependency, err := NewTxSubmitterEngine(ctx, opts)
		assert.Empty(t, gotDependency)
		assert.EqualError(t, err, "trying to cast an existing submitter engine instance")
	})
}
