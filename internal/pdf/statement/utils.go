package statement

import "strings"

func breakHeaderWords(s string) string {
	return strings.ReplaceAll(s, " ", "\n")
}
