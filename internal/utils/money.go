package utils

import (
	"strings"

	"github.com/shopspring/decimal"
)

// FormatDecimal formats d to 2 decimal places with truncation (not rounding) and comma separators.
func FormatDecimal(d decimal.Decimal) string {
	// Truncate to 2 decimal places: multiply by 100, truncate, divide by 100
	truncated := d.Mul(decimal.NewFromInt(100)).Truncate(0).Div(decimal.NewFromInt(100))
	return formatWithCommas(truncated.StringFixed(2))
}

// FormatAmountTo2Decimals parses a string amount and returns it formatted to max 2 decimal places with truncation (not rounding) and comma separators.
func FormatAmountTo2Decimals(s string) string {
	if s == "" {
		return "0.00"
	}
	d, err := decimal.NewFromString(s)
	if err != nil {
		return s
	}
	// Truncate to 2 decimal places: multiply by 100, truncate, divide by 100
	truncated := d.Mul(decimal.NewFromInt(100)).Truncate(0).Div(decimal.NewFromInt(100))
	return formatWithCommas(truncated.StringFixed(2))
}

// formatWithCommas adds comma separators to a number string (e.g., "19950190.79" -> "19,950,190.79").
func formatWithCommas(s string) string {
	// Split into integer and decimal parts
	parts := strings.Split(s, ".")
	integerPart := parts[0]
	decimalPart := ""
	if len(parts) > 1 {
		decimalPart = "." + parts[1]
	}

	// Add commas every 3 digits from right to left
	var result strings.Builder
	length := len(integerPart)
	for i, digit := range integerPart {
		if i > 0 && (length-i)%3 == 0 {
			result.WriteString(",")
		}
		result.WriteRune(digit)
	}

	return result.String() + decimalPart
}
