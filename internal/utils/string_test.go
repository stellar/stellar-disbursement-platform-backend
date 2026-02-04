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

func Test_ContainsAny(t *testing.T) {
	testCases := []struct {
		name       string
		message    string
		substrings []string
		want       bool
	}{
		{
			name:       "message contains one substring",
			message:    "rate limit exceeded",
			substrings: []string{"rate limit", "throttle"},
			want:       true,
		},
		{
			name:       "message contains multiple substrings",
			message:    "too many requests throttled",
			substrings: []string{"too many", "throttle"},
			want:       true,
		},
		{
			name:       "message contains none of the substrings",
			message:    "connection timeout",
			substrings: []string{"rate limit", "throttle"},
			want:       false,
		},
		{
			name:       "empty message",
			message:    "",
			substrings: []string{"rate limit"},
			want:       false,
		},
		{
			name:       "empty substrings",
			message:    "some message",
			substrings: []string{},
			want:       false,
		},
		{
			name:       "substring is empty string (skipped)",
			message:    "some message",
			substrings: []string{"", "nomatch"},
			want:       false,
		},
		{
			name:       "case sensitive match",
			message:    "Rate Limit exceeded",
			substrings: []string{"rate limit"},
			want:       false,
		},
		{
			name:       "case sensitive match works when lowercased",
			message:    "rate limit exceeded",
			substrings: []string{"rate limit"},
			want:       true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := ContainsAny(tc.message, tc.substrings...)
			assert.Equal(t, tc.want, got)
		})
	}
}

func Test_TrimAndLower(t *testing.T) {
	testCases := []struct {
		name     string
		input    string
		expected string
	}{
		{name: "already lowercase no spaces", input: "hello", expected: "hello"},
		{name: "uppercase", input: "HELLO", expected: "hello"},
		{name: "mixed case", input: "HeLLo WoRLD", expected: "hello world"},
		{name: "leading spaces", input: "  hello", expected: "hello"},
		{name: "trailing spaces", input: "hello  ", expected: "hello"},
		{name: "both leading and trailing spaces", input: "  hello  ", expected: "hello"},
		{name: "tabs and newlines", input: "\t\nhello\t\n", expected: "hello"},
		{name: "empty string", input: "", expected: ""},
		{name: "only spaces", input: "   ", expected: ""},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := TrimAndLower(tc.input)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func Test_Humanize(t *testing.T) {
	testCases := []struct {
		name     string
		input    string
		expected string
	}{
		{name: "single word", input: "hello", expected: "hello"},
		{name: "underscores to spaces", input: "hello_world", expected: "hello world"},
		{name: "multiple underscores", input: "hello_world_test", expected: "hello world test"},
		{name: "uppercase to lowercase", input: "HELLO_WORLD", expected: "hello world"},
		{name: "mixed case", input: "Hello_World", expected: "hello world"},
		{name: "no underscores uppercase", input: "HELLO", expected: "hello"},
		{name: "empty string", input: "", expected: ""},
		{name: "consecutive underscores", input: "hello__world", expected: "hello  world"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := Humanize(tc.input)
			assert.Equal(t, tc.expected, result)
		})
	}
}
