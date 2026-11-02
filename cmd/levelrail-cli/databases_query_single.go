package main

import "strings"

// hasMultipleStatements reports whether sql holds more than one statement. It skips quoted
// strings, quoted identifiers, comments and dollar-quoted bodies so a semicolon inside them is not a split.
func hasMultipleStatements(sql string) bool {
	seen := false
	for i := 0; i < len(sql); {
		c := sql[i]
		switch {
		case c == '\'' || c == '"' || c == '`':
			i = skipQuoted(sql, i, c)
			seen = true
		case c == '-' && strings.HasPrefix(sql[i:], "--"):
			i = skipTo(sql, i, "\n")
		case c == '/' && strings.HasPrefix(sql[i:], "/*"):
			i = skipTo(sql, i+2, "*/")
		case c == '$':
			if tag := dollarTag(sql[i:]); tag != "" {
				i = skipTo(sql, i+len(tag), tag)
				seen = true
				continue
			}
			i++
			seen = true
		case c == ';':
			if seen && strings.TrimSpace(stripComments(sql[i+1:])) != "" {
				return true
			}
			i++
		case c == ' ' || c == '\t' || c == '\n' || c == '\r':
			i++
		default:
			seen = true
			i++
		}
	}
	return false
}

// skipQuoted returns the index after the quote that closes the one at start, doubled quotes and backslash escapes included.
func skipQuoted(s string, start int, q byte) int {
	for i := start + 1; i < len(s); i++ {
		switch {
		case s[i] == '\\' && q != '"':
			i++
		case s[i] == q:
			if i+1 < len(s) && s[i+1] == q {
				i++
				continue
			}
			return i + 1
		}
	}
	return len(s)
}

// skipTo returns the index after the next end at or after from, or len(s) when it never appears.
func skipTo(s string, from int, end string) int {
	if from >= len(s) {
		return len(s)
	}
	j := strings.Index(s[from:], end)
	if j < 0 {
		return len(s)
	}
	return from + j + len(end)
}

// dollarTag returns a leading $tag$ opener, or "" when s does not start with one.
func dollarTag(s string) string {
	for i := 1; i < len(s); i++ {
		c := s[i]
		if c == '$' {
			return s[:i+1]
		}
		isWord := c == '_' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' && i > 1
		if !isWord {
			return ""
		}
	}
	return ""
}

// stripComments drops comments so a trailing comment after the final semicolon is not a second statement.
func stripComments(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); {
		switch {
		case strings.HasPrefix(s[i:], "--"):
			i = skipTo(s, i, "\n")
		case strings.HasPrefix(s[i:], "/*"):
			i = skipTo(s, i+2, "*/")
		default:
			b.WriteByte(s[i])
			i++
		}
	}
	return b.String()
}
