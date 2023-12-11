package dependencyinjection

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_NewKafkaEventManager(t *testing.T) {
	testingCases := []struct {
		name            string
		brokers         []string
		topics          []string
		consumerGroupID string
		wantErrContains string
	}{
		{
			name:            "return an error if brokers is empty",
			brokers:         []string{},
			topics:          []string{},
			consumerGroupID: "",
			wantErrContains: "brokers cannot be empty",
		},
		{
			name:            "return an error if consumer topics is empty",
			brokers:         []string{"kafka:9092"},
			topics:          []string{},
			consumerGroupID: "",
			wantErrContains: "consumer topics cannot be empty",
		},
		{
			name:            "return an error if consumer group ID is empty",
			brokers:         []string{"kafka:9092"},
			topics:          []string{"my-topic"},
			consumerGroupID: "",
			wantErrContains: "consumer group ID cannot be empty",
		},
		{
			name:            "🎉 successfully creates a new instance if none exist before",
			brokers:         []string{"kafka:9092"},
			topics:          []string{"my-topic"},
			consumerGroupID: "group-id",
			wantErrContains: "",
		},
	}
	ctx := context.Background()

	for _, tc := range testingCases {
		t.Run(tc.name, func(t *testing.T) {
			defer ClearInstancesTestHelper(t)

			gotResult, err := NewKafkaEventManager(ctx, tc.brokers, tc.topics, tc.consumerGroupID)
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

func Test_NewKafkaEventManager_existingInstanceIsReturned(t *testing.T) {
	ctx := context.Background()
	brokers := []string{"kafka:9092"}
	topics := []string{"my-topic"}
	consumerGroupID := "group-id"

	defer ClearInstancesTestHelper(t)

	// STEP 1: assert that the instance is nil
	_, ok := dependenciesStoreMap[kafkaEventManagerInstanceName]
	require.False(t, ok)

	// STEP 2: create a new instance
	kafkaEventManager1, err := NewKafkaEventManager(ctx, brokers, topics, consumerGroupID)
	require.NoError(t, err)
	require.NotNil(t, kafkaEventManager1)

	// STEP 3: assert that the instance is not nil
	storedKafkaEventManager, ok := dependenciesStoreMap[kafkaEventManagerInstanceName]
	require.True(t, ok)
	require.NotNil(t, storedKafkaEventManager)
	require.Same(t, kafkaEventManager1, storedKafkaEventManager)

	// STEP 4: create a new instance
	kafkaEventManager2, err := NewKafkaEventManager(ctx, brokers, topics, consumerGroupID)
	require.NoError(t, err)
	require.NotNil(t, kafkaEventManager2)

	// STEP 5: assert that the returned object is the same as the stored one
	require.Same(t, kafkaEventManager1, kafkaEventManager2)
}
