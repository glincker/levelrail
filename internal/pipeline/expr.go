package pipeline

import (
	"fmt"
	"strings"
	"unicode"
)

// Scope is what expressions and ${{ }} references resolve against.
// Vars holds dotted paths such as "ref", "matrix.go", "needs.build.result".
type Scope struct {
	Vars      map[string]string
	Success   bool
	Failure   bool
	Cancelled bool
}

// Interpolate replaces every ${{ path }} in s. A missing "secrets." path is
// an error so a typo never silently runs a step without its credential;
// any other missing path resolves to the empty string.
func Interpolate(s string, sc Scope) (string, error) {
	var firstErr error
	out := exprRe.ReplaceAllStringFunc(s, func(m string) string {
		inner := strings.TrimSpace(exprRe.FindStringSubmatch(m)[1])
		v, ok := sc.Vars[inner]
		if !ok && strings.HasPrefix(inner, "secrets.") && firstErr == nil {
			firstErr = fmt.Errorf("secret %q is not set", strings.TrimPrefix(inner, "secrets."))
		}
		return v
	})
	return out, firstErr
}

// EvalCondition evaluates an `if:` expression. An empty expression is true
// only when the scope is in a success state.
func EvalCondition(expr string, sc Scope) (bool, error) {
	expr = strings.TrimSpace(expr)
	if inner := exprRe.FindStringSubmatch(expr); inner != nil && inner[0] == expr {
		expr = inner[1]
	}
	if expr == "" {
		return sc.Success, nil
	}
	p := &exprParser{toks: tokenize(expr), sc: sc}
	v, err := p.parseOr()
	if err != nil {
		return false, fmt.Errorf("condition %q: %w", expr, err)
	}
	if p.pos < len(p.toks) {
		return false, fmt.Errorf("condition %q: unexpected %q", expr, p.toks[p.pos].text)
	}
	return truthy(v), nil
}

// ValidateCondition checks that expr parses, without evaluating it.
func ValidateCondition(expr string) error {
	_, err := EvalCondition(expr, Scope{Vars: map[string]string{}})
	return err
}

func truthy(v string) bool { return v != "" && v != "false" && v != "0" }

func boolStr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

type token struct {
	kind string // str, id, op, lparen, rparen, comma
	text string
}

func tokenize(s string) []token {
	var out []token
	rs := []rune(s)
	for i := 0; i < len(rs); {
		r := rs[i]
		switch {
		case unicode.IsSpace(r):
			i++
		case r == '\'' || r == '"':
			j := i + 1
			for j < len(rs) && rs[j] != r {
				j++
			}
			out = append(out, token{"str", string(rs[i+1 : min(j, len(rs))])})
			i = j + 1
		case r == '(':
			out = append(out, token{"lparen", "("})
			i++
		case r == ')':
			out = append(out, token{"rparen", ")"})
			i++
		case r == ',':
			out = append(out, token{"comma", ","})
			i++
		case strings.ContainsRune("=!&|", r):
			if i+1 < len(rs) && (string(rs[i:i+2]) == "==" || string(rs[i:i+2]) == "!=" || string(rs[i:i+2]) == "&&" || string(rs[i:i+2]) == "||") {
				out = append(out, token{"op", string(rs[i : i+2])})
				i += 2
			} else {
				out = append(out, token{"op", string(r)})
				i++
			}
		default:
			j := i
			for j < len(rs) && (unicode.IsLetter(rs[j]) || unicode.IsDigit(rs[j]) || strings.ContainsRune("_.-*/", rs[j])) {
				j++
			}
			if j == i {
				j = i + 1
			}
			out = append(out, token{"id", string(rs[i:j])})
			i = j
		}
	}
	return out
}

type exprParser struct {
	toks []token
	pos  int
	sc   Scope
}

func (p *exprParser) peek() (token, bool) {
	if p.pos < len(p.toks) {
		return p.toks[p.pos], true
	}
	return token{}, false
}

func (p *exprParser) eatOp(op string) bool {
	if t, ok := p.peek(); ok && t.kind == "op" && t.text == op {
		p.pos++
		return true
	}
	return false
}

func (p *exprParser) parseOr() (string, error) {
	l, err := p.parseAnd()
	if err != nil {
		return "", err
	}
	for p.eatOp("||") {
		r, err := p.parseAnd()
		if err != nil {
			return "", err
		}
		l = boolStr(truthy(l) || truthy(r))
	}
	return l, nil
}

func (p *exprParser) parseAnd() (string, error) {
	l, err := p.parseEq()
	if err != nil {
		return "", err
	}
	for p.eatOp("&&") {
		r, err := p.parseEq()
		if err != nil {
			return "", err
		}
		l = boolStr(truthy(l) && truthy(r))
	}
	return l, nil
}

func (p *exprParser) parseEq() (string, error) {
	l, err := p.parseUnary()
	if err != nil {
		return "", err
	}
	for {
		switch {
		case p.eatOp("=="):
			r, err := p.parseUnary()
			if err != nil {
				return "", err
			}
			l = boolStr(l == r)
		case p.eatOp("!="):
			r, err := p.parseUnary()
			if err != nil {
				return "", err
			}
			l = boolStr(l != r)
		default:
			return l, nil
		}
	}
}

func (p *exprParser) parseUnary() (string, error) {
	if p.eatOp("!") {
		v, err := p.parseUnary()
		if err != nil {
			return "", err
		}
		return boolStr(!truthy(v)), nil
	}
	return p.parsePrimary()
}

func (p *exprParser) parsePrimary() (string, error) {
	t, ok := p.peek()
	if !ok {
		return "", fmt.Errorf("unexpected end of expression")
	}
	p.pos++
	switch t.kind {
	case "str":
		return t.text, nil
	case "lparen":
		v, err := p.parseOr()
		if err != nil {
			return "", err
		}
		if n, ok := p.peek(); !ok || n.kind != "rparen" {
			return "", fmt.Errorf("missing closing parenthesis")
		}
		p.pos++
		return v, nil
	case "id":
		if n, ok := p.peek(); ok && n.kind == "lparen" {
			p.pos++
			return p.call(t.text)
		}
		switch t.text {
		case "true", "false":
			return t.text, nil
		}
		return p.sc.Vars[t.text], nil
	}
	return "", fmt.Errorf("unexpected %q", t.text)
}

func (p *exprParser) call(name string) (string, error) {
	var args []string
	for {
		if t, ok := p.peek(); ok && t.kind == "rparen" {
			p.pos++
			break
		}
		v, err := p.parseOr()
		if err != nil {
			return "", err
		}
		args = append(args, v)
		if t, ok := p.peek(); ok && t.kind == "comma" {
			p.pos++
			continue
		}
		if t, ok := p.peek(); !ok || t.kind != "rparen" {
			return "", fmt.Errorf("missing closing parenthesis in %s()", name)
		}
	}
	need := func(n int) error {
		if len(args) != n {
			return fmt.Errorf("%s() takes %d argument(s)", name, n)
		}
		return nil
	}
	switch name {
	case "success":
		return boolStr(p.sc.Success), need(0)
	case "failure":
		return boolStr(p.sc.Failure), need(0)
	case "cancelled":
		return boolStr(p.sc.Cancelled), need(0)
	case "always":
		return "true", need(0)
	case "contains":
		if err := need(2); err != nil {
			return "", err
		}
		return boolStr(strings.Contains(args[0], args[1])), nil
	case "startsWith":
		if err := need(2); err != nil {
			return "", err
		}
		return boolStr(strings.HasPrefix(args[0], args[1])), nil
	case "endsWith":
		if err := need(2); err != nil {
			return "", err
		}
		return boolStr(strings.HasSuffix(args[0], args[1])), nil
	}
	return "", fmt.Errorf("unknown function %s()", name)
}
