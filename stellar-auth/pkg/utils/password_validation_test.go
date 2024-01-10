package utils

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_ValidatePassword(t *testing.T) {
	pwValidator := PasswordValidator{
		commonPasswordsList: map[string]bool{"password!1234": true},
	}

	const (
		commonPasswordErrMsg     = "password is determined to be too common"
		defaultErrMsg            = "password validation failed for the following reason(s)"
		invalidLengthErrMsg      = "length: password length must be between 12 and 36 characters"
		invalidCharsErrMsg       = "invalid character: password cannot contain any invalid characters ("
		missingLowercaseErrMsg   = "lowercase: password must contain at least one lowercase letter"
		missingUppercaseErrMsg   = "uppercase: password must contain at least one uppercase letter"
		missingDigitErrMsg       = "digit: password must contain at least one numberical digit"
		missingSpecialCharErrMsg = "special character: password must contain at least one special character"
	)

	allErrMessages := []string{
		commonPasswordErrMsg,
		defaultErrMsg,
		invalidLengthErrMsg,
		invalidCharsErrMsg,
		missingLowercaseErrMsg,
		missingUppercaseErrMsg,
		missingDigitErrMsg,
		missingSpecialCharErrMsg,
	}

	testCases := []struct {
		name        string
		input       string
		errContains []string
	}{
		{
			name:  "All criteria is missing: [length, invalid character, lowercase, uppercase, digit, special character]",
			input: "Δ Д",
			errContains: []string{
				defaultErrMsg,
				invalidLengthErrMsg,
				"invalid character: password cannot contain any invalid characters ('Δ', ' ', 'Д')",
				missingLowercaseErrMsg,
				missingUppercaseErrMsg,
				missingDigitErrMsg,
				missingSpecialCharErrMsg,
			},
		},
		{
			name:  "criteria missing: [length, invalid character, lowercase, uppercase, digit]",
			input: "!Δ Д",
			errContains: []string{
				defaultErrMsg,
				invalidLengthErrMsg,
				"invalid character: password cannot contain any invalid characters ('Δ', ' ', 'Д')",
				missingLowercaseErrMsg,
				missingUppercaseErrMsg,
				missingDigitErrMsg,
			},
		},
		{
			name:  "criteria missing: [length, invalid character, lowercase, uppercase]",
			input: "!1Δ Д",
			errContains: []string{
				defaultErrMsg,
				invalidLengthErrMsg,
				"invalid character: password cannot contain any invalid characters ('Δ', ' ', 'Д')",
				missingLowercaseErrMsg,
				missingUppercaseErrMsg,
			},
		},
		{
			name:  "criteria missing: [length, invalid character, lowercase]",
			input: "!1AΔ Д",
			errContains: []string{
				defaultErrMsg,
				invalidLengthErrMsg,
				"invalid character: password cannot contain any invalid characters ('Δ', ' ', 'Д')",
				missingLowercaseErrMsg,
			},
		},
		{
			name:  "criteria missing: [length, invalid character]",
			input: "!1AzΔ Д",
			errContains: []string{
				defaultErrMsg,
				invalidLengthErrMsg,
				"invalid character: password cannot contain any invalid characters ('Δ', ' ', 'Д')",
			},
		},
		{
			name:        "only one criteria is missing: [length]",
			input:       "!1Az",
			errContains: []string{defaultErrMsg, invalidLengthErrMsg},
		},
		{
			name:        "only one criteria is missing: [invalid character]",
			input:       "Д!1Az?2By.3Cx",
			errContains: []string{defaultErrMsg, "invalid character: password cannot contain any invalid characters ('Д')"},
		},
		{
			name:        "only one criteria is missing: [lowercase]",
			input:       "!1AZ?2BY.3CX",
			errContains: []string{defaultErrMsg, missingLowercaseErrMsg},
		},
		{
			name:        "only one criteria is missing: [uppercase]",
			input:       "!1az?2by.3cx",
			errContains: []string{defaultErrMsg, missingUppercaseErrMsg},
		},
		{
			name:        "only one criteria is missing: [digit]",
			input:       "!aAz?bBy.cCx",
			errContains: []string{defaultErrMsg, missingDigitErrMsg},
		},
		{
			name:        "only one criteria is missing: [special character]",
			input:       "11Az22By33Cx",
			errContains: []string{defaultErrMsg, missingSpecialCharErrMsg},
		},
		{
			name:        "only one criteria is missing: [common password]",
			input:       "pAssWord!1234",
			errContains: []string{defaultErrMsg, commonPasswordErrMsg},
		},
		{
			name:        "🎉 All criteria was met!",
			input:       "!1Az?2By.3Cx",
			errContains: nil,
		},
		{
			name:        "🎉 All criteria was met (inverted order)!",
			input:       "xC3.yB2?zA1!",
			errContains: nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := pwValidator.ValidatePassword(tc.input)
			if tc.errContains == nil {
				require.NoError(t, err)
			} else {
				var e *ValidatePasswordError
				require.ErrorAs(t, err, &e)

				expectedErrMap := map[string]struct{}{}
				var assertionsCount int

				// STEP 1: assert the presence of errors that are expected to be returned
				for _, ec := range tc.errContains {
					assert.ErrorContainsf(t, err, ec, "expected error message %q not found", ec)
					assertionsCount++

					// STEP 2: build the map of errors that are expected to be returned
					if strings.HasPrefix(ec, invalidCharsErrMsg) {
						expectedErrMap[invalidCharsErrMsg] = struct{}{}
					} else {
						expectedErrMap[ec] = struct{}{}
					}
				}

				// STEP 3: assert that the response does not contain any other error messages
				for _, em := range allErrMessages {
					if _, ok := expectedErrMap[em]; !ok {
						assert.NotContainsf(t, err.Error(), em, "found unexpected error message: %q", em)
						assertionsCount++
					}
				}

				// STEP 4: Assert that the amount of assertions was the same as the amount of possible errors, asserting that all errors were tested.
				require.Len(t, allErrMessages, assertionsCount)
			}
		})
	}
}
