package compose

import (
	"fmt"
	"strings"
)

// dollarSentinel holds a "$$" escape through the SERVICE_ pass, so the
// literal "$" it stands for is never re-read as the start of a token.
const dollarSentinel = "\x00$\x00"

// unsetVarFormat is parsed back by InterpolateDeclared to name a missing variable.
const unsetVarFormat = "variable %s is not set, left as written"

func unescapeDollar(s string) string {
	if !strings.Contains(s, dollarSentinel) {
		return s
	}
	return strings.ReplaceAll(s, dollarSentinel, "$")
}

// interpolateValue expands ${VAR}, ${VAR:-d}, ${VAR-d}, ${VAR:?msg},
// ${VAR?msg}, ${VAR:+alt}, ${VAR+alt} and the "$$" escape. SERVICE_ tokens
// pass through byte for byte for the magic-var pass. An unset plain ${VAR}
// stays as written (the container shell may own it) and is warned about.
// Messages carry names only, never values.
func interpolateValue(s string, vars map[string]string, warn func(string)) (string, error) {
	if len(s) > maxInterpolatedLen {
		return "", fmt.Errorf("value is longer than the %d byte interpolation limit", maxInterpolatedLen)
	}
	return interpolateDepth(s, vars, warn, 0, "")
}

// Caps keep a hostile compose body from pinning a core or the stack: each
// nesting level rescans its operand, so work grows with depth times length.
const (
	maxInterpolationDepth = 16
	maxInterpolatedLen    = 64 << 10
)

func interpolateDepth(s string, vars map[string]string, warn func(string), depth int, parent string) (string, error) {
	if depth > maxInterpolationDepth {
		return "", fmt.Errorf("variable %s: ${...} nesting is deeper than the limit of %d", parent, maxInterpolationDepth)
	}
	if !strings.Contains(s, "$") {
		return s, nil
	}
	var b strings.Builder
	for i := 0; i < len(s); {
		if s[i] != '$' || i+1 >= len(s) {
			b.WriteByte(s[i])
			i++
			continue
		}
		if s[i+1] == '$' {
			b.WriteString(dollarSentinel)
			i += 2
			continue
		}
		if s[i+1] != '{' {
			b.WriteByte(s[i])
			i++
			continue
		}
		end, ok := closingBrace(s, i+2)
		name, rest := splitVarName(s[i+2 : end])
		if !ok || name == "" || strings.HasPrefix(name, "SERVICE_") {
			cut := strings.IndexByte(s[i:], '}')
			if cut < 0 {
				b.WriteString(s[i:])
				return b.String(), nil
			}
			b.WriteString(s[i : i+cut+1])
			i += cut + 1
			continue
		}
		out, err := expandVar(name, rest, vars, warn, depth)
		if err != nil {
			return "", err
		}
		b.WriteString(out)
		if b.Len() > maxInterpolatedLen {
			return "", fmt.Errorf("variable %s: expanded value is longer than the %d byte limit", name, maxInterpolatedLen)
		}
		i = end + 1
	}
	return b.String(), nil
}

// closingBrace finds the } closing a ${ whose body starts at from, counting nested ${.
func closingBrace(s string, from int) (int, bool) {
	depth := 1
	for j := from; j < len(s); j++ {
		switch {
		case s[j] == '$' && j+1 < len(s) && s[j+1] == '{':
			depth++
			j++
		case s[j] == '}':
			depth--
			if depth == 0 {
				return j, true
			}
		}
	}
	return len(s), false
}

func splitVarName(body string) (name, rest string) {
	n := 0
	for n < len(body) {
		c := body[n]
		if c == '_' || (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (n > 0 && c >= '0' && c <= '9') {
			n++
			continue
		}
		break
	}
	return body[:n], body[n:]
}

func expandVar(name, rest string, vars map[string]string, warn func(string), depth int) (string, error) {
	val, set := vars[name]
	nonEmpty := set && val != ""
	if rest == "" {
		if set {
			return val, nil
		}
		warn(fmt.Sprintf(unsetVarFormat, name))
		return "${" + name + "}", nil
	}
	op, arg := rest[:1], rest[1:]
	colon := false
	if op == ":" && len(arg) > 0 {
		colon, op, arg = true, arg[:1], arg[1:]
	}
	present := set
	if colon {
		present = nonEmpty
	}
	switch op {
	case "-":
		if present {
			return val, nil
		}
		return interpolateDepth(arg, vars, warn, depth+1, name)
	case "+":
		if present {
			return interpolateDepth(arg, vars, warn, depth+1, name)
		}
		return "", nil
	case "?":
		if present {
			return val, nil
		}
		if arg == "" {
			return "", fmt.Errorf("required variable %s is missing a value", name)
		}
		return "", fmt.Errorf("required variable %s is missing a value: %s", name, arg)
	}
	// Unknown operator: not a form we own, so leave it for the container.
	return "${" + name + rest + "}", nil
}
