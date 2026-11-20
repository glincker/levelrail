package platformimport

import (
	"fmt"
	"strconv"
	"strings"
)

// parseRailwayTOML reads the small TOML subset a Railway config file uses:
// tables, dotted keys, strings, numbers, booleans and arrays of scalars.
// Anything else is an error so nothing is silently misread.
func parseRailwayTOML(data string) (map[string]any, error) {
	root := map[string]any{}
	cur := root
	lines := strings.Split(strings.ReplaceAll(data, "\r\n", "\n"), "\n")
	for i := 0; i < len(lines); i++ {
		line := strings.TrimSpace(stripTOMLComment(lines[i]))
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "[[") {
			return nil, fmt.Errorf("line %d: arrays of tables are not supported", i+1)
		}
		if strings.HasPrefix(line, "[") {
			if !strings.HasSuffix(line, "]") {
				return nil, fmt.Errorf("line %d: malformed table header", i+1)
			}
			t, err := tomlTable(root, splitTOMLKey(line[1:len(line)-1]))
			if err != nil {
				return nil, fmt.Errorf("line %d: %w", i+1, err)
			}
			cur = t
			continue
		}
		eq := strings.Index(line, "=")
		if eq < 0 {
			return nil, fmt.Errorf("line %d: expected key = value", i+1)
		}
		key, raw := strings.TrimSpace(line[:eq]), strings.TrimSpace(line[eq+1:])
		for strings.HasPrefix(raw, "[") && strings.Count(raw, "[") > strings.Count(raw, "]") && i+1 < len(lines) {
			i++
			raw += " " + strings.TrimSpace(stripTOMLComment(lines[i]))
		}
		val, err := parseTOMLValue(raw)
		if err != nil {
			return nil, fmt.Errorf("line %d: %w", i+1, err)
		}
		path := splitTOMLKey(key)
		t, err := tomlTable(cur, path[:len(path)-1])
		if err != nil {
			return nil, fmt.Errorf("line %d: %w", i+1, err)
		}
		t[path[len(path)-1]] = val
	}
	return root, nil
}

func tomlTable(from map[string]any, path []string) (map[string]any, error) {
	t := from
	for _, p := range path {
		next, ok := t[p]
		if !ok {
			m := map[string]any{}
			t[p] = m
			t = m
			continue
		}
		m, ok := next.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("%q is both a value and a table", p)
		}
		t = m
	}
	return t, nil
}

func splitTOMLKey(k string) []string {
	var out []string
	var b strings.Builder
	quote := byte(0)
	for i := 0; i < len(k); i++ {
		c := k[i]
		switch {
		case quote != 0 && c == quote:
			quote = 0
		case quote == 0 && (c == '"' || c == '\''):
			quote = c
		case quote == 0 && c == '.':
			out = append(out, strings.TrimSpace(b.String()))
			b.Reset()
		default:
			b.WriteByte(c)
		}
	}
	return append(out, strings.TrimSpace(b.String()))
}

func stripTOMLComment(s string) string {
	quote := byte(0)
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case quote != 0:
			if c == '\\' && quote == '"' {
				i++
			} else if c == quote {
				quote = 0
			}
		case c == '"' || c == '\'':
			quote = c
		case c == '#':
			return s[:i]
		}
	}
	return s
}

func parseTOMLValue(raw string) (any, error) {
	switch {
	case raw == "":
		return nil, fmt.Errorf("missing value")
	case strings.HasPrefix(raw, `"""`) || strings.HasPrefix(raw, "'''"):
		return nil, fmt.Errorf("multi-line strings are not supported")
	case raw[0] == '"':
		s, err := strconv.Unquote(raw)
		if err != nil {
			return nil, fmt.Errorf("bad string %s", raw)
		}
		return s, nil
	case raw[0] == '\'':
		if len(raw) < 2 || raw[len(raw)-1] != '\'' {
			return nil, fmt.Errorf("bad string %s", raw)
		}
		return raw[1 : len(raw)-1], nil
	case raw == "true":
		return true, nil
	case raw == "false":
		return false, nil
	case raw[0] == '{':
		return nil, fmt.Errorf("inline tables are not supported, use a [table] header")
	case raw[0] == '[':
		return parseTOMLArray(raw)
	}
	if n, err := strconv.Atoi(strings.ReplaceAll(raw, "_", "")); err == nil {
		return float64(n), nil
	}
	if f, err := strconv.ParseFloat(raw, 64); err == nil {
		return f, nil
	}
	return nil, fmt.Errorf("unsupported value %s", raw)
}

func parseTOMLArray(raw string) (any, error) {
	if !strings.HasSuffix(raw, "]") {
		return nil, fmt.Errorf("unterminated array")
	}
	body := strings.TrimSpace(raw[1 : len(raw)-1])
	out := []any{}
	var b strings.Builder
	quote := byte(0)
	flush := func() error {
		if s := strings.TrimSpace(b.String()); s != "" {
			v, err := parseTOMLValue(s)
			if err != nil {
				return err
			}
			out = append(out, v)
		}
		b.Reset()
		return nil
	}
	for i := 0; i < len(body); i++ {
		c := body[i]
		switch {
		case quote != 0:
			b.WriteByte(c)
			if c == '\\' && quote == '"' && i+1 < len(body) {
				i++
				b.WriteByte(body[i])
			} else if c == quote {
				quote = 0
			}
		case c == '"' || c == '\'':
			quote = c
			b.WriteByte(c)
		case c == ',':
			if err := flush(); err != nil {
				return nil, err
			}
		default:
			b.WriteByte(c)
		}
	}
	if err := flush(); err != nil {
		return nil, err
	}
	return out, nil
}
