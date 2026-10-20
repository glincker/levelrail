package search

import (
	"strings"
)

// Query is a parsed search string: scoped filters plus free text.
type Query struct {
	Text []string
	App  string
	Env  string
	Tag  string
	Node string
	Kind Kind
}

var scopeKeys = map[string]struct{}{"app": {}, "env": {}, "tag": {}, "node": {}, "kind": {}}

// ParseQuery splits `app:foo env:prod kind:db restart api` into scopes and
// free text. An unknown `key:value` stays free text, so URLs and image names
// remain searchable.
func ParseQuery(q string) Query {
	var out Query
	for _, tok := range strings.Fields(q) {
		key, val, ok := strings.Cut(tok, ":")
		key = strings.ToLower(key)
		if _, scoped := scopeKeys[key]; !ok || !scoped || val == "" {
			out.Text = append(out.Text, tok)
			continue
		}
		switch key {
		case "app":
			out.App = val
		case "env":
			out.Env = val
		case "tag":
			out.Tag = val
		case "node":
			out.Node = val
		case "kind":
			if k, ok := ParseKind(val); ok {
				out.Kind = k
			} else {
				out.Text = append(out.Text, tok)
			}
		}
	}
	return out
}

// Empty reports whether the query has neither text nor a scope.
func (q Query) Empty() bool {
	return len(q.Text) == 0 && q.App == "" && q.Env == "" && q.Tag == "" && q.Node == "" && q.Kind == ""
}
