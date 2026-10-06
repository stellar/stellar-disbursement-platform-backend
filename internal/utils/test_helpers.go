package utils

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
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
	var exitError *exec.ExitError
	require.ErrorAs(t, err, &exitError)
	require.True(t, errors.As(err, &exitError), "process ran with err %v, want exit status 1", err)
	require.False(t, exitError.Success())

	for _, stdErrContain := range stdErrContains {
		require.Contains(t, stderr.String(), stdErrContain)
	}
}

// CreatePNGHeaderWithDimensions builds a minimal valid PNG (signature + IHDR only) declaring
// w x h. It is a few dozen bytes, so tests can exercise the logo dimension cap without
// allocating a real image the way a decompression bomb would.
func CreatePNGHeaderWithDimensions(t *testing.T, w, h uint32) []byte {
	t.Helper()
	var buf bytes.Buffer
	buf.Write([]byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a})
	ihdr := make([]byte, 13)
	binary.BigEndian.PutUint32(ihdr[0:4], w)
	binary.BigEndian.PutUint32(ihdr[4:8], h)
	ihdr[8] = 8 // bit depth
	ihdr[9] = 2 // color type: truecolor
	var length [4]byte
	binary.BigEndian.PutUint32(length[:], uint32(len(ihdr)))
	buf.Write(length[:])
	buf.WriteString("IHDR")
	buf.Write(ihdr)
	crc := crc32.NewIEEE()
	crc.Write([]byte("IHDR"))
	crc.Write(ihdr)
	var crcBytes [4]byte
	binary.BigEndian.PutUint32(crcBytes[:], crc.Sum32())
	buf.Write(crcBytes[:])
	return buf.Bytes()
}
