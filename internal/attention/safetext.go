package attention

import (
	"strings"
	"unicode"
)

// MaxErrorText bounds an agent-reported error shown in an item.
const MaxErrorText = 200

const redactedText = "[redacted]"

// SafeErrorText bounds and cleans text an agent reported. Anything that
// looks like key or certificate material is replaced entirely.
func SafeErrorText(s string) string {
	l := strings.ToUpper(s)
	if strings.Contains(l, "-----BEGIN") || strings.Contains(l, "PRIVATE KEY") {
		return redactedText
	}
	s = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, s)
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > MaxErrorText {
		s = string(r[:MaxErrorText]) + "..."
	}
	return s
}
