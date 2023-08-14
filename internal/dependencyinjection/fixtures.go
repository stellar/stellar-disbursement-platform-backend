package dependencyinjection

import "testing"

func ClearInstancesTestHelper(t *testing.T) {
	t.Helper()
	dependenciesStoreMap = make(map[string]interface{})
}
