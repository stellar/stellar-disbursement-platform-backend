package cmd

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_noArgsAndHelpHaveSameResultAndDoDontPanic(t *testing.T) {
	cmdArgsTestCases := [][]string{
		{"--help"},
		{},
	}

	for i, cmdArgs := range cmdArgsTestCases {
		// setup
		rootCmd := SetupCLI("x.y.z", "1234567890abcdef")
		rootCmd.SetArgs(cmdArgs)
		var out bytes.Buffer
		rootCmd.SetOut(&out)

		// test
		err := rootCmd.Execute()
		assert.NoErrorf(t, err, "test case %d returned an error", i)

		// assert printed text
		assert.Containsf(t, out.String(), "Use \"stellar-disbursement-platform [command] --help\" for more information about a command.", "test case %d did not print help message as expected", i)
	}
}
