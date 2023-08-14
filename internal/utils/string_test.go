package utils

import (
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_RandomString(t *testing.T) {
	randomString1, err := RandomString(10)
	require.NoError(t, err)
	require.Len(t, randomString1, 10)
	randomString2, err := RandomString(10)
	require.NoError(t, err)
	require.Len(t, randomString2, 10)
	require.NotEqual(t, randomString1, randomString2)

	randomString3, err := RandomString(5)
	require.NoError(t, err)
	require.Len(t, randomString3, 5)

	randomString4, err := RandomString(6, NumberBytes)
	require.NoError(t, err)
	require.Len(t, randomString4, 6)
	onlyNumbers := regexp.MustCompile(`\d`).MatchString(randomString4)
	assert.True(t, onlyNumbers)
}

func Test_TruncateString(t *testing.T) {
	testCases := []struct {
		name             string
		rawString        string
		borderSizeToKeep int
		wantTruncated    string
	}{
		{
			name:             "string is shorter than borderSizeToKeep",
			rawString:        "abc",
			borderSizeToKeep: 4,
			wantTruncated:    "abc",
		},
		{
			name:             "string is longer than borderSizeToKeep",
			rawString:        "abcdefg",
			borderSizeToKeep: 3,
			wantTruncated:    "abc...efg",
		},
		{
			name:             "string is same length as borderSizeToKeep",
			rawString:        "abcdef",
			borderSizeToKeep: 3,
			wantTruncated:    "abcdef",
		},
		{
			name:             "string is empty",
			rawString:        "",
			borderSizeToKeep: 3,
			wantTruncated:    "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			gotTruncated := TruncateString(tc.rawString, tc.borderSizeToKeep)
			assert.Equal(t, tc.wantTruncated, gotTruncated, "Expected Truncate(%q, %d) to be %q, but got %q", tc.rawString, tc.borderSizeToKeep, tc.wantTruncated, gotTruncated)
		})
	}
}
