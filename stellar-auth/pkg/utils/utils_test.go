package utils

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_StringWithCharsetLenght(t *testing.T) {
	charset := "asdfghjklzxcvbnm"
	tokenLength := 4

	token, err := StringWithCharset(tokenLength, charset)
	require.NoError(t, err)
	token2, err := StringWithCharset(tokenLength, charset)
	require.NoError(t, err)
	assert.Len(t, token, tokenLength)
	assert.NotEqual(t, token, token2)
}

func Test_ValidateEmail(t *testing.T) {
	testCases := []struct {
		email   string
		wantErr error
	}{
		{"", fmt.Errorf("email cannot be empty")},
		{"notvalidemail", fmt.Errorf(`the provided email "notvalidemail" is not valid`)},
		{"valid@test.com", nil},
		{"valid+email@test.com", nil},
	}

	for _, tc := range testCases {
		t.Run(tc.email, func(t *testing.T) {
			gotError := ValidateEmail(tc.email)
			assert.Equalf(t, tc.wantErr, gotError, "ValidateEmail(%q) should be %v, but got %v", tc.email, tc.wantErr, gotError)
		})
	}
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
