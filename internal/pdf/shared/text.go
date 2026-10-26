package shared

import (
	"strings"
	"unicode"
)

// PrintableText drops what the embedded fonts cannot draw: characters beyond U+FFFF (the UTF-8 font
// tables stop there) and control characters.
func PrintableText(s string) string {
	return strings.Map(func(r rune) rune {
		if r > 0xFFFF || unicode.IsControl(r) {
			return -1
		}
		return r
	}, s)
}
