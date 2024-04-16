package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_ExtractTenantNameFromHostName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
		err      error
	}{
		{"invalid", "", ErrTenantNameNotFound},
		{"", "", ErrTenantNameNotFound},
		{"aidorg.sdp.com", "aidorg", nil},
		{"subdomain.aidorg.sdp.com", "subdomain", nil},
		{"sub-domain.aidorg.sdp.com", "sub-domain", nil},
		{"aidorg.sdp.com:8000", "aidorg", nil},
	}

	for _, test := range tests {
		actualOutput, actualError := ExtractTenantNameFromHostName(test.input)
		assert.Equal(t, test.expected, actualOutput)
		assert.Equal(t, test.err, actualError)
	}
}
