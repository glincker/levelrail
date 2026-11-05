package iac

import (
	"strings"
	"testing"
)

func TestPlanValuesTruncatedAndHidden(t *testing.T) {
	t.Setenv("APP_IAC_PLAN_VALUE_MAX", "10")
	long := strings.Repeat("x", 40)
	desired := map[string]any{"note": long + "b", "apiToken": "new", "secrets": map[string]any{"db": "n"}, "env": map[string]any{"A": "1"}}
	live := map[string]any{"note": long + "a", "apiToken": "old", "secrets": map[string]any{"db": "o"}, "env": map[string]any{"A": "0"}}
	changes, _ := diffFields(desired, live, nil, false)
	got := map[string]FieldChange{}
	for _, c := range changes {
		got[c.Path] = c
	}
	if n := got["note"]; n.Old != strings.Repeat("x", 10)+"..." || n.New != n.Old {
		t.Errorf("note not truncated: %+v", n)
	}
	for _, p := range []string{"apiToken", "secrets.db", "env.A"} {
		if c := got[p]; c.Old != hiddenValue || c.New != hiddenValue {
			t.Errorf("%s not hidden: %+v", p, c)
		}
	}
}
