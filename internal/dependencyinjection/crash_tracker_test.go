package dependencyinjection

import (
	"context"
	"testing"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/crashtracker"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_dependencyinjection_buildCrashTrackerInstanceName(t *testing.T) {
	testCrashTrackerType := crashtracker.CrashTrackerTypeSentry
	result := buildCrashTrackerInstanceName(testCrashTrackerType)
	assert.Equal(t, "crash_tracker_instance-SENTRY", result)
}

func Test_dependencyinjection_NewCrashTracker(t *testing.T) {
	ctx := context.Background()
	t.Run("should create and return the same instrance on the second call", func(t *testing.T) {
		ClearInstancesTestHelper(t)

		testSentryOptions := crashtracker.CrashTrackerOptions{
			CrashTrackerType: crashtracker.CrashTrackerTypeSentry,
		}

		gotClient, err := NewCrashTracker(ctx, testSentryOptions)
		require.NoError(t, err)

		gotClientDuplicate, err := NewCrashTracker(ctx, testSentryOptions)
		require.NoError(t, err)

		assert.Equal(t, &gotClient, &gotClientDuplicate)
	})

	t.Run("should return an error on a invalid option", func(t *testing.T) {
		ClearInstancesTestHelper(t)

		testInvalidOptions := crashtracker.CrashTrackerOptions{}

		gotClient, err := NewCrashTracker(ctx, testInvalidOptions)
		assert.Nil(t, gotClient)
		assert.EqualError(t, err, `error creating a new crash tracker instance: unknown crash tracker type: ""`)
	})

	t.Run("should return an error on a invalid instance", func(t *testing.T) {
		ClearInstancesTestHelper(t)

		testSentryOptions := crashtracker.CrashTrackerOptions{
			CrashTrackerType: crashtracker.CrashTrackerTypeSentry,
		}

		setInstance(buildCrashTrackerInstanceName(testSentryOptions.CrashTrackerType), false)

		gotClient, err := NewCrashTracker(ctx, testSentryOptions)
		assert.Nil(t, gotClient)
		assert.EqualError(t, err, "error trying to cast crash tracker instance")
	})
}
