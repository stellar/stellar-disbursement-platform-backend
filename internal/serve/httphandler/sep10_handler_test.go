package httphandler

import (
	"testing"

	"github.com/stellar/go/keypair"
	"github.com/stretchr/testify/assert"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/services"
)

func TestValidateChallengeRequest(t *testing.T) {
	t.Parallel()
	kp := keypair.MustRandom()
	handler := SEP10Handler{}

	testCases := []struct {
		name          string
		account       string
		expectError   bool
		errMsg string
	}{
		{
			name:        "valid Ed25519 account",
			account:     kp.Address(),
			expectError: false,
		},
		{
			name:          "empty account",
			account:       "",
			expectError:   true,
			errMsg: "account is required",
		},
		{
			name:          "invalid account format",
			account:       "invalid-account",
			expectError:   true,
			errMsg: "invalid account format",
		},
		{
			name:          "muxed account not supported",
			account:       "MAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAABAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
			expectError:   true,
			errMsg: "invalid account format",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req := services.ChallengeRequest{
				Account: tc.account,
			}

			err := handler.validateChallengeRequest(req)

			if tc.expectError {
				assert.Error(t, err, "expected error but got nil")
				assert.Contains(t, err.Error(), tc.errMsg,
					"expected error to contain %q but got %q", tc.errMsg, err.Error())
			} else {
				assert.NoError(t, err, "expected no error but got %v", err)
			}
		})
	}
}

func TestValidateChallengeRequestWithMemo(t *testing.T) {
	t.Parallel()
	kp := keypair.MustRandom()
	handler := SEP10Handler{}

	testCases := []struct {
		name          string
		account       string
		memo          string
		expectError   bool
		errMsg string
	}{
		{
			name:        "valid account with valid memo",
			account:     kp.Address(),
			memo:        "12345",
			expectError: false,
		},
		{
			name:          "valid account with invalid memo",
			account:       kp.Address(),
			memo:          "invalid-memo",
			expectError:   true,
			errMsg: "invalid memo must be a positive integer",
		},
		{
			name:          "valid account with negative memo",
			account:       kp.Address(),
			memo:          "-123",
			expectError:   true,
			errMsg: "invalid memo must be a positive integer",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req := services.ChallengeRequest{
				Account: tc.account,
				Memo:    tc.memo,
			}

			err := handler.validateChallengeRequest(req)

			if tc.expectError {
				assert.Error(t, err, "expected error but got nil")
				assert.Contains(t, err.Error(), tc.errMsg,
					"expected error to contain %q but got %q", tc.errMsg, err.Error())
			} else {
				assert.NoError(t, err, "expected no error but got %v", err)
			}
		})
	}
}

func TestParseContentType(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "empty content type",
			input:    "",
			expected: "",
		},
		{
			name:     "simple content type",
			input:    "application/json",
			expected: "application/json",
		},
		{
			name:     "content type with charset",
			input:    "application/x-www-form-urlencoded; charset=utf-8",
			expected: "application/x-www-form-urlencoded",
		},
		{
			name:     "content type with multiple parameters",
			input:    "application/json; charset=utf-8; boundary=123",
			expected: "application/json",
		},
		{
			name:     "content type with spaces",
			input:    "  application/json  ;  charset=utf-8  ",
			expected: "application/json",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := parseContentType(tc.input)
			assert.Equal(t, tc.expected, result, "expected %q but got %q", tc.expected, result)
		})
	}
}
