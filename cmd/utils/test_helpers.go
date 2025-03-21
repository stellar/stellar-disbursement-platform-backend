package utils

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
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

// AssertFuncExitsWithFatal asserts that a function exits with a fatal error, usually `os.Exit(1)`.
func AssertFuncExitsWithFatal(t *testing.T, fatalFunc func(), stdErrContains ...string) {
	t.Helper()

	if os.Getenv("TEST_FATAL") == "1" {
		// Run the fatal function that will call os.Exit(n)
		fatalFunc()
		return
	}

	cmd := exec.Command(os.Args[0], fmt.Sprintf("-test.run=%s", t.Name()))
	cmd.Env = append(os.Environ(), "TEST_FATAL=1")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	err := cmd.Run()
	if exitError, ok := err.(*exec.ExitError); ok {
		require.False(t, exitError.Success())
		return
	}

	for _, stdErrContain := range stdErrContains {
		require.Contains(t, stderr.String(), stdErrContain)
	}

	t.Fatalf("process ran with err %v, want exit status 1", err)
}
