package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_GenerateSalt(t *testing.T) {
	testCases := []struct {
		name          string
		contact       string
		contactType   string
		expectedError string
	}{
		{
			name:        "successful generation with email",
			contact:     "test@example.com",
			contactType: "EMAIL",
		},
		{
			name:        "successful generation with phone number",
			contact:     "+1-555-123-4567",
			contactType: "PHONE_NUMBER",
		},
		{
			name:          "error with empty contact",
			contact:       "",
			contactType:   "EMAIL",
			expectedError: "receiver contact cannot be empty",
		},
		{
			name:          "error with empty contact type",
			contact:       "test@example.com",
			contactType:   "",
			expectedError: "contact type cannot be empty",
		},
		{
			name:          "error with invalid contact type",
			contact:       "test@example.com",
			contactType:   "INVALID",
			expectedError: "contact type must be 'EMAIL' or 'PHONE_NUMBER', got: INVALID",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			salt, err := GenerateSalt(tc.contact, tc.contactType)

			if tc.expectedError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.expectedError)
			} else {
				require.NoError(t, err)
				assert.NotEmpty(t, salt)
			}
		})
	}
}

func Test_GenerateSalt_HexLength(t *testing.T) {
	saltHex, err := GenerateSalt("test@example.com", "EMAIL")
	require.NoError(t, err)
	assert.Len(t, saltHex, 64)
}

func Test_GenerateSalt_Deterministic(t *testing.T) {
	testCases := []struct {
		name        string
		contact1    string
		contact2    string
		type1       string
		type2       string
		shouldMatch bool
	}{
		{
			name:        "same email should produce same salt",
			contact1:    "test@example.com",
			contact2:    "test@example.com",
			type1:       "EMAIL",
			type2:       "EMAIL",
			shouldMatch: true,
		},
		{
			name:        "email case differences should produce same salt",
			contact1:    "Test@Example.Com",
			contact2:    "test@example.com",
			type1:       "EMAIL",
			type2:       "EMAIL",
			shouldMatch: true,
		},
		{
			name:        "phone format differences should produce same salt",
			contact1:    "+1-555-123-4567",
			contact2:    "5551234567",
			type1:       "PHONE_NUMBER",
			type2:       "PHONE_NUMBER",
			shouldMatch: true,
		},
		{
			name:        "different emails should produce different salts",
			contact1:    "test1@example.com",
			contact2:    "test2@example.com",
			type1:       "EMAIL",
			type2:       "EMAIL",
			shouldMatch: false,
		},
		{
			name:        "same contact different types should produce different salts",
			contact1:    "5551234567",
			contact2:    "5551234567",
			type1:       "EMAIL",
			type2:       "PHONE_NUMBER",
			shouldMatch: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			salt1, err := GenerateSalt(tc.contact1, tc.type1)
			require.NoError(t, err)

			salt2, err := GenerateSalt(tc.contact2, tc.type2)
			require.NoError(t, err)

			if tc.shouldMatch {
				assert.Equal(t, salt1, salt2, "expected salts to match")
			} else {
				assert.NotEqual(t, salt1, salt2, "expected salts to be different")
			}
		})
	}
}

func Test_GenerateSalt_Consistency(t *testing.T) {
	originalSaltHex, err := GenerateSalt("test@example.com", "EMAIL")
	require.NoError(t, err)

	saltHex, err := GenerateSalt("test@example.com", "EMAIL")
	require.NoError(t, err)

	assert.Equal(t, originalSaltHex, saltHex)
}

func Test_normalizeContactForSalt(t *testing.T) {
	testCases := []struct {
		name        string
		contact     string
		contactType ContactType
		expected    string
	}{
		{
			name:        "email normalization - lowercase",
			contact:     "Test@Example.Com",
			contactType: ContactTypeEmail,
			expected:    "test@example.com",
		},
		{
			name:        "email normalization - trim spaces",
			contact:     "  test@example.com  ",
			contactType: ContactTypeEmail,
			expected:    "test@example.com",
		},
		{
			name:        "phone normalization - basic format",
			contact:     "+1-555-123-4567",
			contactType: ContactTypePhoneNumber,
			expected:    "5551234567",
		},
		{
			name:        "phone normalization - with parentheses",
			contact:     "(555) 123-4567",
			contactType: ContactTypePhoneNumber,
			expected:    "5551234567",
		},
		{
			name:        "phone normalization - international with country code",
			contact:     "+15551234567",
			contactType: ContactTypePhoneNumber,
			expected:    "5551234567",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := normalizeContactForSalt(tc.contact, tc.contactType)
			assert.Equal(t, tc.expected, result)
		})
	}
}
