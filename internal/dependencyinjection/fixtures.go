package dependencyinjection

import "testing"

func ClearInstancesTestHelper(t *testing.T) {
	t.Helper()

	// Range over the map and delete each entry.
	dependenciesStore.Range(func(key, value interface{}) bool {
		dependenciesStore.Delete(key)
		return true // Continue iteration
	})
}
