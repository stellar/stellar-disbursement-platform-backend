package shared

import "strings"

// BreakHeaderWords inserts newlines between words
func BreakHeaderWords(s string) string {
	return strings.ReplaceAll(s, " ", "\n")
}
