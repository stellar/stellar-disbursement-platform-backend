package dependencyinjection

import (
	"context"
	"fmt"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/crashtracker"
)

const CrashTrackerInstanceName = "crash_tracker_instance"

// buildCrashTrackerInstanceName sets up a instance name for the crash tracker type
// to either be created and stored, also retrived later, so we can have a instance
// for each type at the same time.
func buildCrashTrackerInstanceName(crashTrackerType crashtracker.CrashTrackerType) string {
	return fmt.Sprintf("%s-%s", CrashTrackerInstanceName, string(crashTrackerType))
}

// NewCrashTracker creates a new crash tracker instance, or retrives a instance that
// was already created before.
func NewCrashTracker(ctx context.Context, opts crashtracker.CrashTrackerOptions) (crashtracker.CrashTrackerClient, error) {
	instanceName := buildCrashTrackerInstanceName(opts.CrashTrackerType)

	// Already initialized
	if instance, ok := dependenciesStoreMap[instanceName]; ok {
		if crashTrackerInstance, ok := instance.(crashtracker.CrashTrackerClient); ok {
			return crashTrackerInstance, nil
		}
		return nil, fmt.Errorf("error trying to cast crash tracker instance")
	}

	// Setup crash tracker instance
	newCrashTracker, err := crashtracker.GetClient(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("error creating a new crash tracker instance: %w", err)
	}

	setInstance(instanceName, newCrashTracker)

	return newCrashTracker, nil
}
