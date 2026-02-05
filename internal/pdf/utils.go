package pdf

import "strings"

// breakHeaderWords inserts newlines between words
func breakHeaderWords(s string) string {
	return strings.ReplaceAll(s, " ", "\n")
}
