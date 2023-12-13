package dependencyinjection

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_NewKafkaProducer(t *testing.T) {
	testingCases := []struct {
		name            string
		brokers         []string
		wantErrContains string
	}{
		{
			name:            "return an error if brokers is empty",
			brokers:         []string{},
			wantErrContains: "brokers cannot be empty",
		},
		{
			name:            "🎉 successfully creates a new instance if none exist before",
			brokers:         []string{"kafka:9092"},
			wantErrContains: "",
		},
	}
	ctx := context.Background()

	for _, tc := range testingCases {
		t.Run(tc.name, func(t *testing.T) {
			defer ClearInstancesTestHelper(t)

			gotResult, err := NewKafkaProducer(ctx, tc.brokers)
			if tc.wantErrContains != "" {
				require.ErrorContains(t, err, tc.wantErrContains)
				require.Nil(t, gotResult)
			} else {
				require.NoError(t, err)
				require.NotNil(t, gotResult)
			}
		})
	}
}

func Test_NewKafkaProducer_existingInstanceIsReturned(t *testing.T) {
	ctx := context.Background()
	brokers := []string{"kafka:9092"}

	defer ClearInstancesTestHelper(t)

	// STEP 1: assert that the instance is nil
	_, ok := dependenciesStoreMap[kafkaProducerInstanceName]
	require.False(t, ok)

	// STEP 2: create a new instance
	kafkaProducer1, err := NewKafkaProducer(ctx, brokers)
	require.NoError(t, err)
	require.NotNil(t, kafkaProducer1)

	// STEP 3: assert that the instance is not nil
	storedKafkaProducer, ok := dependenciesStoreMap[kafkaProducerInstanceName]
	require.True(t, ok)
	require.NotNil(t, storedKafkaProducer)
	require.Same(t, kafkaProducer1, storedKafkaProducer)

	// STEP 4: create a new instance
	kafkaProducer2, err := NewKafkaProducer(ctx, brokers)
	require.NoError(t, err)
	require.NotNil(t, kafkaProducer2)

	// STEP 5: assert that the returned object is the same as the stored one
	require.Same(t, kafkaProducer1, kafkaProducer2)
}
