// Package cliconsole turns one typed console line into a vetted argv for the
// platform CLI and runs it as a child process. It is never a shell.
package cliconsole

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"unicode"
)

// Limits on one console line.
const (
	MaxLineBytes = 4096
	MaxArgs      = 64
)

// RejectError explains why a line was refused. Its message is safe to show the operator.
type RejectError struct{ Reason string }

func (e *RejectError) Error() string { return e.Reason }

func reject(format string, args ...any) error {
	return &RejectError{Reason: fmt.Sprintf(format, args...)}
}

// IsReject reports whether err is a policy refusal rather than a runtime failure.
func IsReject(err error) bool {
	var r *RejectError
	return errors.As(err, &r)
}

var envAssignment = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*=`)

// ParseLine splits line into words with POSIX-style quoting and no expansion.
// Shell syntax is refused rather than passed through, so nothing typed here
// can look like it chains, redirects or substitutes.
func ParseLine(line string) ([]string, error) {
	if len(line) > MaxLineBytes {
		return nil, reject("line is longer than %d bytes", MaxLineBytes)
	}
	for _, r := range line {
		if r == '\n' || r == '\r' || r == 0 || (unicode.IsControl(r) && r != '\t') {
			return nil, reject("control characters and newlines are not allowed")
		}
	}
	if strings.ContainsRune(line, '`') || strings.Contains(line, "$(") || strings.Contains(line, "${") {
		return nil, reject("command substitution is not supported: this console runs CLI commands, not a shell")
	}

	var (
		words   []string
		cur     strings.Builder
		inWord  bool
		quote   rune
		escaped bool
	)
	flush := func() {
		if inWord {
			words = append(words, cur.String())
			cur.Reset()
			inWord = false
		}
	}
	for _, r := range line {
		switch {
		case escaped:
			cur.WriteRune(r)
			escaped = false
		case quote == '\'':
			if r == '\'' {
				quote = 0
			} else {
				cur.WriteRune(r)
			}
		case quote == '"':
			switch r {
			case '"':
				quote = 0
			case '\\':
				escaped = true
			default:
				cur.WriteRune(r)
			}
		case r == '\\':
			escaped, inWord = true, true
		case r == '\'' || r == '"':
			quote, inWord = r, true
		case r == ' ' || r == '\t':
			flush()
		case strings.ContainsRune("|;&<>(){}", r):
			return nil, reject("%q is shell syntax; pipes, redirects and chaining are not supported", string(r))
		default:
			cur.WriteRune(r)
			inWord = true
		}
	}
	if quote != 0 || escaped {
		return nil, reject("unterminated quote or trailing backslash")
	}
	flush()
	if len(words) == 0 {
		return nil, reject("empty command")
	}
	if len(words) > MaxArgs {
		return nil, reject("too many arguments (max %d)", MaxArgs)
	}
	if envAssignment.MatchString(words[0]) {
		return nil, reject("environment assignments are not supported")
	}
	return words, nil
}
