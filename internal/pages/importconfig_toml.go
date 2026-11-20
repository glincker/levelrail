package pages

import (
	"fmt"
	"strconv"
	"strings"
)

// importTomlRules reads [[redirects]] and [[headers]] tables with a small
// line parser, enough for the flat shape these files use.
func importTomlRules(src, text string, res *ImportResult) {
	type table struct {
		kind   string
		keys   map[string]string
		values [][2]string
		line   int
	}
	var tables []*table
	var cur *table
	inValues := false
	for i, raw := range strings.Split(text, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		switch {
		case line == "[[redirects]]" || line == "[[headers]]":
			cur = &table{kind: strings.Trim(line, "[]"), keys: map[string]string{}, line: i + 1}
			tables = append(tables, cur)
			inValues = false
		case line == "[headers.values]":
			inValues = cur != nil && cur.kind == "headers"
		case strings.HasPrefix(line, "["):
			cur = nil
			inValues = false
		case cur != nil:
			k, v, ok := strings.Cut(line, "=")
			if !ok {
				continue
			}
			k = strings.Trim(strings.TrimSpace(k), `"'`)
			v = unquoteToml(strings.TrimSpace(v))
			if inValues {
				cur.values = append(cur.values, [2]string{k, v})
			} else {
				cur.keys[k] = v
			}
		}
	}
	for _, t := range tables {
		where := fmt.Sprintf("%s table at line %d", src, t.line)
		if t.kind == "headers" {
			if t.keys["for"] == "" {
				res.skip(where, "headers table without a for path")
				continue
			}
			for _, h := range t.values {
				res.addHeader(t.keys["for"], h[0], h[1])
			}
			continue
		}
		importTomlRedirect(where, t.keys, res)
	}
}

func importTomlRedirect(where string, keys map[string]string, res *ImportResult) {
	from, to := keys["from"], keys["to"]
	if from == "" || to == "" {
		res.skip(where, "redirect needs from and to")
		return
	}
	for _, k := range []string{"conditions", "query", "headers", "signed"} {
		if _, bad := keys[k]; bad {
			res.skip(where, k+" matchers are not supported")
			return
		}
	}
	status := 301
	if s, ok := keys["status"]; ok {
		n, err := strconv.Atoi(s)
		if err != nil {
			res.skip(where, "status is not a number")
			return
		}
		status = n
	}
	res.addRedirect(from, to, status)
}

func unquoteToml(v string) string {
	if len(v) >= 2 && (v[0] == '"' && v[len(v)-1] == '"' || v[0] == '\'' && v[len(v)-1] == '\'') {
		return v[1 : len(v)-1]
	}
	if i := strings.Index(v, " #"); i >= 0 {
		v = strings.TrimSpace(v[:i])
	}
	return v
}
