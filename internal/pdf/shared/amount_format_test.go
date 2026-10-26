package shared

import (
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
)

func TestFormatAmountTo2Decimals(t *testing.T) {
	testCases := map[string]string{
		"":              "0.00",
		"not-a-number":  "not-a-number",
		"0":             "0.00",
		"7":             "7.00",
		"0.0000001":     "0.00",
		"123.999":       "123.99", // truncated, not rounded
		"19950190.79":   "19,950,190.79",
		"1000000.5":     "1,000,000.50",
		"-123":          "-123.00", // the sign is not counted when placing commas
		"-1234567.891":  "-1,234,567.89",
		"1234567890.12": "1,234,567,890.12",
	}
	for input, expected := range testCases {
		assert.Equal(t, expected, FormatAmountTo2Decimals(input), "input %q", input)
	}
}

func TestFormatDecimal(t *testing.T) {
	assert.Equal(t, "123.99", FormatDecimal(decimal.NewFromFloat(123.999)))
	assert.Equal(t, "-1,234.50", FormatDecimal(decimal.NewFromFloat(-1234.5)))
}
