package utils

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/require"
)

// AssertFuncExitsWithFatal asserts that a function exits with a fatal error, usually `os.Exit(1)`.
func AssertFuncExitsWithFatal(t *testing.T, fatalFunc func(), stdErrContains ...string) {
	t.Helper()

	if os.Getenv("TEST_SUBPROCESS") == "1" {
		// Run the fatal function that will call os.Exit(n)
		fatalFunc()
		return
	}

	cmd := exec.Command(os.Args[0], fmt.Sprintf("-test.run=%s", t.Name()))
	cmd.Env = append(os.Environ(), "TEST_SUBPROCESS=1")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	err := cmd.Run()
	exitError, ok := err.(*exec.ExitError)
	require.Truef(t, ok, "process ran with err %v, want exit status 1", err)
	require.False(t, exitError.Success())

	for _, stdErrContain := range stdErrContains {
		require.Contains(t, stderr.String(), stdErrContain)
	}
}
