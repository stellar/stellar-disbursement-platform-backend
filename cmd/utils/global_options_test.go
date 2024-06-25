package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/crashtracker"
)

func Test_globalOptions_PopulateCrashTrackerOptions(t *testing.T) {
	globalOptions := GlobalOptionsType{
		Environment: "test",
		GitCommit:   "1234567890abcdef",
		SentryDSN:   "test-sentry-dsn",
	}

	t.Run("CrashTrackerType is not Sentry", func(t *testing.T) {
		crashTrackerOptions := crashtracker.CrashTrackerOptions{}
		globalOptions.PopulateCrashTrackerOptions(&crashTrackerOptions)

		wantCrashTrackerOptions := crashtracker.CrashTrackerOptions{
			Environment: "test",
			GitCommit:   "1234567890abcdef",
		}
		assert.Equal(t, wantCrashTrackerOptions, crashTrackerOptions)
	})

	t.Run("CrashTrackerType is Sentry", func(t *testing.T) {
		crashTrackerOptions := crashtracker.CrashTrackerOptions{
			CrashTrackerType: crashtracker.CrashTrackerTypeSentry,
		}
		globalOptions.PopulateCrashTrackerOptions(&crashTrackerOptions)

		wantCrashTrackerOptions := crashtracker.CrashTrackerOptions{
			Environment:      "test",
			GitCommit:        "1234567890abcdef",
			SentryDSN:        "test-sentry-dsn",
			CrashTrackerType: crashtracker.CrashTrackerTypeSentry,
		}
		assert.Equal(t, wantCrashTrackerOptions, crashTrackerOptions)
	})
}
