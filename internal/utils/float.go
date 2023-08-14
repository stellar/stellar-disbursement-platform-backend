package utils

import "strconv"

// FloatToString converts a float number to a string with 7 decimal places.
func FloatToString(inputNum float64) string {
	return strconv.FormatFloat(inputNum, 'f', 7, 64)
}
