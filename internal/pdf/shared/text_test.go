package shared

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPrintableText(t *testing.T) {
	testCases := map[string]struct{ in, want string }{
		"text within U+FFFF unchanged": {"Stellar Aid María 支付", "Stellar Aid María 支付"},
		"beyond U+FFFF dropped":        {"Aid 🚀 a𠀋b", "Aid  ab"},
		"control characters dropped":   {"a\x00b\x01c\x7fd\x1be", "abcde"},
		"tab and newline dropped":      {"line one\n\tline two", "line oneline two"},
		"empty":                        {"", ""},
	}
	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.want, PrintableText(tc.in))
		})
	}
}
