package utils

import (
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFormatDecimal(t *testing.T) {
	tests := []struct {
		name     string
		input    decimal.Decimal
		expected string
	}{
		{
			name:     "zero",
			input:    decimal.Zero,
			expected: "0.00",
		},
		{
			name:     "small decimal",
			input:    decimal.NewFromFloat(123.45),
			expected: "123.45",
		},
		{
			name:     "large number with commas",
			input:    decimal.NewFromFloat(19950190.79),
			expected: "19,950,190.79",
		},
		{
			name:     "truncates to 2 decimals",
			input:    decimal.NewFromFloat(123.456789),
			expected: "123.45",
		},
		{
			name:     "negative number",
			input:    decimal.NewFromFloat(-1234.56),
			expected: "-1,234.56",
		},
		{
			name:     "very small decimal",
			input:    decimal.NewFromFloat(0.001),
			expected: "0.00",
		},
		{
			name:     "million with decimals",
			input:    decimal.NewFromFloat(1000000.99),
			expected: "1,000,000.99",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FormatDecimal(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestFormatAmountTo2Decimals(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "empty string returns 0.00",
			input:    "",
			expected: "0.00",
		},
		{
			name:     "valid decimal string",
			input:    "123.45",
			expected: "123.45",
		},
		{
			name:     "large number with commas",
			input:    "19950190.79",
			expected: "19,950,190.79",
		},
		{
			name:     "truncates to 2 decimals",
			input:    "123.456789",
			expected: "123.45",
		},
		{
			name:     "negative number",
			input:    "-1234.56",
			expected: "-1,234.56",
		},
		{
			name:     "invalid string returns as-is",
			input:    "not-a-number",
			expected: "not-a-number",
		},
		{
			name:     "very small decimal",
			input:    "0.001",
			expected: "0.00",
		},
		{
			name:     "million with decimals",
			input:    "1000000.99",
			expected: "1,000,000.99",
		},
		{
			name:     "integer",
			input:    "1000",
			expected: "1,000.00",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FormatAmountTo2Decimals(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestFormatWithCommas(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "small number",
			input:    "123.45",
			expected: "123.45",
		},
		{
			name:     "thousand",
			input:    "1000.00",
			expected: "1,000.00",
		},
		{
			name:     "million",
			input:    "19950190.79",
			expected: "19,950,190.79",
		},
		{
			name:     "billion",
			input:    "1234567890.12",
			expected: "1,234,567,890.12",
		},
		{
			name:     "integer without decimal",
			input:    "1000",
			expected: "1,000",
		},
		{
			name:     "single digit",
			input:    "5",
			expected: "5",
		},
		{
			name:     "negative number",
			input:    "-1234.56",
			expected: "-1,234.56",
		},
		{
			name:     "no decimal part",
			input:    "1234",
			expected: "1,234",
		},
		{
			name:     "negative 3-digit integer (no comma expected)",
			input:    "-123",
			expected: "-123",
		},
		{
			name:     "negative 6-digit integer",
			input:    "-123456",
			expected: "-123,456",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatWithCommas(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestFormatDecimalEdgeCases(t *testing.T) {
	// Test that FormatDecimal properly truncates (not rounds)
	value := decimal.NewFromFloat(123.999)
	result := FormatDecimal(value)
	require.Equal(t, "123.99", result, "should truncate, not round")

	// Test zero
	result = FormatDecimal(decimal.Zero)
	require.Equal(t, "0.00", result)
}
