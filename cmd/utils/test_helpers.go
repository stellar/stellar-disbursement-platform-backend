package utils

import (
	"os"
	"strings"
	"testing"
)

// clearTestEnvironment removes all envs from the test environment. It's useful
// to make tests independent from the localhost environment variables.
func ClearTestEnvironment(t *testing.T) {
	// remove all envs from tghe test environment
	for _, env := range os.Environ() {
		key := env[:strings.Index(env, "=")]
		t.Setenv(key, "")
	}
}
