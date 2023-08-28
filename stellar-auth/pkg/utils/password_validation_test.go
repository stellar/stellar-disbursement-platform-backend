package utils

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_ValidatePassword(t *testing.T) {
	testCases := []struct {
		name        string
		input       string
		errContains []string
	}{
		{
			name:        "returns an error if the input is less than 8 characters",
			input:       "test",
			errContains: []string{"password must have at least 8 characters"},
		},
		{
			name:        "returns an error if the input does not contain all the required characters (NO uppercase letters, symbols)",
			input:       "test1234",
			errContains: []string{"uppercase letter", "symbol"},
		},
		{
			name:        "return an error if the input does not contain all the required characters (NO digits)",
			input:       "test#ABC",
			errContains: []string{"digit"},
		},
		{
			name:        "returns an error if the input does not contain all the required characters (NO digits, symbols)",
			input:       "testTEST",
			errContains: []string{"digit", "symbol"},
		},
		{
			name:        "returns an error if the input does not contain all the required characters (NO lowercase letters, symbols)",
			input:       "TEST123123",
			errContains: []string{"lowercase letter", "symbol"},
		},
		{
			name:        "returns an error if the input does not contain all the required characters (NO lowercase, uppercase letters, symbols)",
			input:       "1010011010",
			errContains: []string{"lowercase letter", "uppercase letter", "symbol"},
		},
		{
			name:        "returns an error if the input contains invalid character(s) but fulfills the minimum character requirement",
			input:       "1Tv(^_^)vT1",
			errContains: []string{"password contains invalid characters"},
		},
		{
			name:  "returns no error if the input is valid (happy path 1)",
			input: "tEsT123#@",
		},
		{
			name:  "returns no error if the input is valid (happy path 2)",
			input: "h3LL0w0rLd$$$",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidatePassword(tc.input)
			if tc.errContains == nil {
				require.NoError(t, err)
			} else {
				for _, ec := range tc.errContains {
					require.ErrorContains(t, err, ec)
				}
			}
		})
	}
}
