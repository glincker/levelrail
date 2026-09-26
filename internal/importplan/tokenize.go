package importplan

import (
	"errors"
	"strings"
)

// ErrUnterminatedQuote is returned by Tokenize for an unbalanced quote.
var ErrUnterminatedQuote = errors.New("importplan: unterminated quote")

// Tokenize splits a shell-style command line into words. It handles single
// and double quotes, backslash escapes and backslash-newline continuations.
// Nothing is expanded or executed: $VAR and $(...) stay literal text.
func Tokenize(s string) ([]string, error) {
	var (
		out     []string
		cur     strings.Builder
		inWord  bool
		inSing  bool
		inDoub  bool
		runes   = []rune(s)
		flushFn = func() {
			if inWord {
				out = append(out, cur.String())
				cur.Reset()
				inWord = false
			}
		}
	)
	for i := 0; i < len(runes); i++ {
		c := runes[i]
		switch {
		case inSing:
			if c == '\'' {
				inSing = false
			} else {
				cur.WriteRune(c)
			}
		case inDoub:
			switch {
			case c == '"':
				inDoub = false
			case c == '\\' && i+1 < len(runes) && strings.ContainsRune("\"\\$`", runes[i+1]):
				i++
				cur.WriteRune(runes[i])
			case c == '\\' && i+1 < len(runes) && runes[i+1] == '\n':
				i++
			default:
				cur.WriteRune(c)
			}
		case c == '\\':
			if i+1 >= len(runes) {
				cur.WriteRune(c)
				inWord = true
				continue
			}
			i++
			if runes[i] == '\r' && i+1 < len(runes) && runes[i+1] == '\n' {
				i++
			}
			if runes[i] == '\n' {
				continue
			}
			cur.WriteRune(runes[i])
			inWord = true
		case c == '\'':
			inSing, inWord = true, true
		case c == '"':
			inDoub, inWord = true, true
		case c == ' ' || c == '\t' || c == '\n' || c == '\r':
			flushFn()
		case c == '#' && !inWord:
			for i < len(runes) && runes[i] != '\n' {
				i++
			}
		default:
			cur.WriteRune(c)
			inWord = true
		}
	}
	if inSing || inDoub {
		return nil, ErrUnterminatedQuote
	}
	flushFn()
	return out, nil
}
