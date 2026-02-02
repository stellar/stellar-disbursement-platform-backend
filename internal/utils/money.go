package utils

import "github.com/shopspring/decimal"

// FormatDecimal formats d to 2 decimal places.
func FormatDecimal(d decimal.Decimal) string {
	return d.StringFixed(2)
}

// FormatAmountTo2Decimals parses a string amount and returns it formatted to max 2 decimal places.
func FormatAmountTo2Decimals(s string) string {
	if s == "" {
		return "0.00"
	}
	d, err := decimal.NewFromString(s)
	if err != nil {
		return s
	}
	return d.StringFixed(2)
}
