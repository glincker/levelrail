package mailtemplates

import (
	"fmt"
	"html"
	"regexp"
	"sort"
	"strings"
)

var tokenRe = regexp.MustCompile(`\{\{\s*([a-z_]*)\s*\}\}`)

var openBraceRe = regexp.MustCompile(`\{\{`)

// ValidateTokens reports every token in text that is malformed or not in
// allowed. Saving a template with any of these is refused.
func ValidateTokens(text string, allowed []string) error {
	ok := make(map[string]bool, len(allowed))
	for _, a := range allowed {
		ok[a] = true
	}
	var bad []string
	seen := map[string]bool{}
	matches := tokenRe.FindAllStringSubmatch(text, -1)
	for _, m := range matches {
		if !ok[m[1]] && !seen[m[1]] {
			seen[m[1]] = true
			bad = append(bad, "{{"+m[1]+"}}")
		}
	}
	if len(openBraceRe.FindAllString(text, -1)) != len(matches) {
		bad = append(bad, "a malformed {{ }} placeholder")
	}
	if len(bad) > 0 {
		sort.Strings(bad)
		return fmt.Errorf("unknown or malformed tokens: %s", strings.Join(bad, ", "))
	}
	return nil
}

// substitute replaces each {{token}} with its value, HTML-escaped when
// escape is set. A token with no value becomes empty text.
func substitute(text string, values map[string]string, escape bool) string {
	return tokenRe.ReplaceAllStringFunc(text, func(m string) string {
		v := values[tokenRe.FindStringSubmatch(m)[1]]
		if escape {
			return html.EscapeString(v)
		}
		return v
	})
}

// SubstituteText fills tokens for a plain-text field such as a subject.
// Line breaks in values are flattened so a value cannot extend a header.
func SubstituteText(text string, values map[string]string) string {
	flat := make(map[string]string, len(values))
	for k, v := range values {
		flat[k] = strings.NewReplacer("\r", " ", "\n", " ").Replace(v)
	}
	return substitute(text, flat, false)
}
