package telemetry

import (
	"errors"
	"fmt"
	"strings"
)

// ErrInvalidLogSearch marks a log search expression that cannot be run.
var ErrInvalidLogSearch = errors.New("telemetry: invalid log search")

const (
	maxLogSearchLen   = 512
	maxLogSearchTerms = 24
)

const (
	tokTerm  = "term"
	tokOp    = "op"
	tokOpen  = "("
	tokClose = ")"
	tokStart = ""
)

// QuoteLogPhrase turns text into a search expression that matches it as one literal phrase.
func QuoteLogPhrase(text string) string {
	return `"` + strings.ReplaceAll(text, `"`, `""`) + `"`
}

// ValidateLogSearch reports whether query is a runnable search expression.
func ValidateLogSearch(query string) error {
	_, err := compileLogSearch(query)
	return err
}

type searchToken struct {
	kind   string
	value  string
	prefix bool
}

func isSearchSpace(r rune) bool { return r == ' ' || r == '\t' || r == '\n' }

func lexQuoted(rs []rune, i int) (string, int, error) {
	var b strings.Builder
	for i < len(rs) {
		if rs[i] == '"' {
			if i+1 < len(rs) && rs[i+1] == '"' {
				b.WriteRune('"')
				i += 2
				continue
			}
			return b.String(), i + 1, nil
		}
		b.WriteRune(rs[i])
		i++
	}
	return "", i, fmt.Errorf("%w: unbalanced quote", ErrInvalidLogSearch)
}

func lexWord(word string) ([]searchToken, error) {
	switch word {
	case "AND", "OR", "NOT":
		return []searchToken{{kind: tokOp, value: word}}, nil
	}
	var toks []searchToken
	if strings.HasPrefix(word, "-") && len(word) > 1 {
		toks = append(toks, searchToken{kind: tokOp, value: "NOT"})
		word = word[1:]
	}
	prefix := strings.HasSuffix(word, "*")
	word = strings.TrimRight(word, "*")
	if word == "" {
		return nil, fmt.Errorf("%w: put * after a word, for example err*", ErrInvalidLogSearch)
	}
	return append(toks, searchToken{kind: tokTerm, value: word, prefix: prefix}), nil
}

func lexLogSearch(query string) ([]searchToken, error) {
	var toks []searchToken
	rs := []rune(query)
	for i := 0; i < len(rs); {
		c := rs[i]
		switch {
		case isSearchSpace(c):
			i++
		case c == '(' || c == ')':
			toks = append(toks, searchToken{kind: string(c)})
			i++
		case c == '"':
			phrase, next, err := lexQuoted(rs, i+1)
			if err != nil {
				return nil, err
			}
			t := searchToken{kind: tokTerm, value: phrase}
			i = next
			if i < len(rs) && rs[i] == '*' {
				t.prefix = true
				i++
			}
			toks = append(toks, t)
		default:
			start := i
			for i < len(rs) && !isSearchSpace(rs[i]) && rs[i] != '(' && rs[i] != ')' && rs[i] != '"' {
				i++
			}
			ws, err := lexWord(string(rs[start:i]))
			if err != nil {
				return nil, err
			}
			toks = append(toks, ws...)
		}
	}
	return toks, nil
}

func searchErr(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalidLogSearch, fmt.Sprintf(format, args...))
}

// compileLogSearch converts the operator syntax (quoted phrases, word*,
// AND, OR, NOT, -word, parentheses) into an FTS5 MATCH expression. Every
// term is quoted so characters FTS5 treats specially cannot cause a syntax error.
func compileLogSearch(query string) (string, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return "", nil
	}
	if len(query) > maxLogSearchLen {
		return "", searchErr("longer than %d characters", maxLogSearchLen)
	}
	toks, err := lexLogSearch(query)
	if err != nil {
		return "", err
	}
	var (
		out   []string
		depth int
		terms int
		last  = tokStart
	)
	hasOperand := func() bool { return last == tokTerm || last == tokClose }
	for _, t := range toks {
		switch t.kind {
		case tokTerm:
			terms++
			if terms > maxLogSearchTerms {
				return "", searchErr("more than %d terms", maxLogSearchTerms)
			}
			if hasOperand() {
				out = append(out, "AND")
			}
			s := QuoteLogPhrase(t.value)
			if t.prefix {
				s += "*"
			}
			out = append(out, s)
		case tokOp:
			if t.value == "NOT" {
				if !hasOperand() {
					return "", searchErr("NOT needs a term before it, for example error NOT timeout")
				}
			} else if !hasOperand() {
				return "", searchErr("%s needs a term before it", t.value)
			}
			out = append(out, t.value)
		case tokOpen:
			if hasOperand() {
				out = append(out, "AND")
			}
			depth++
			out = append(out, "(")
		case tokClose:
			depth--
			if depth < 0 || !hasOperand() {
				return "", searchErr("unbalanced parentheses")
			}
			out = append(out, ")")
		}
		last = t.kind
	}
	switch {
	case depth != 0:
		return "", searchErr("unbalanced parentheses")
	case last == tokOp:
		return "", searchErr("search ends with an operator")
	case terms == 0:
		return "", searchErr("no search term")
	}
	return strings.Join(out, " "), nil
}
