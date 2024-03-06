package dependencyinjection

import (
	"context"
	"testing"
)

func ClearInstancesTestHelper(t *testing.T) {
	t.Helper()

	// Range over the map and delete each entry.
	for instanceName := range dependenciesStore {
		CleanupInstanceByKey(context.Background(), instanceName)
	}
}
