package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_ExtractTenantNameFromHostName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
		err      error
	}{
		{
			name:     "invalid hostname without subdomain",
			input:    "invalid",
			expected: "",
			err:      ErrTenantNameNotFound,
		},
		{
			name:     "empty hostname",
			input:    "",
			expected: "",
			err:      ErrTenantNameNotFound,
		},
		{
			name:     "valid hostname with subdomain",
			input:    "aidorg.sdp.com",
			expected: "aidorg",
			err:      nil,
		},
		{
			name:     "valid hostname with multiple subdomains",
			input:    "subdomain.aidorg.sdp.com",
			expected: "subdomain",
			err:      nil,
		},
		{
			name:     "valid hostname with hyphenated subdomain",
			input:    "sub-domain.aidorg.sdp.com",
			expected: "sub-domain",
			err:      nil,
		},
		{
			name:     "valid hostname with port",
			input:    "aidorg.sdp.com:8000",
			expected: "aidorg",
			err:      nil,
		},
		{
			name:     "IPv4 address",
			input:    "192.168.1.1",
			expected: "",
			err:      ErrHostnameIsIPAddress,
		},
		{
			name:     "IPv4 address with port",
			input:    "192.168.1.1:8000",
			expected: "",
			err:      ErrHostnameIsIPAddress,
		},
		{
			name:     "IPv4 localhost",
			input:    "127.0.0.1",
			expected: "",
			err:      ErrHostnameIsIPAddress,
		},
		{
			name:     "IPv6 address loopback",
			input:    "::1",
			expected: "",
			err:      ErrHostnameIsIPAddress,
		},
		{
			name:     "IPv6 address full form",
			input:    "2001:0db8:85a3:0000:0000:8a2e:0370:7334",
			expected: "",
			err:      ErrHostnameIsIPAddress,
		},
		{
			name:     "IPv6 address compressed",
			input:    "2001:db8::1",
			expected: "",
			err:      ErrHostnameIsIPAddress,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			actualOutput, actualError := ExtractTenantNameFromHostName(test.input)
			assert.Equal(t, test.expected, actualOutput)
			if test.err != nil {
				assert.ErrorIs(t, actualError, test.err)
			} else {
				assert.NoError(t, actualError)
			}
		})
	}
}
