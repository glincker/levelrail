package connhub

import (
	"errors"
	"strings"
	"testing"
)

func TestBindingRuleStringsAreOneLineAndBounded(t *testing.T) {
	lim := RuleLimits{MaxBytes: 4096, MaxValueLen: 32}
	for name, tc := range map[string]struct {
		role, raw string
		ok        bool
	}{
		"plain branch":          {"source", `{"branch":"release/v1"}`, true},
		"plain paths":           {"source", `{"paths":["web/**","api/*.go"]}`, true},
		"trimmed branch":        {"source", `{"branch":" main "}`, true},
		"newline in branch":     {"source", `{"branch":"main\nmore"}`, false},
		"carriage return":       {"source", `{"branch":"main\rx"}`, false},
		"tab in path":           {"source", `{"paths":["web\t/**"]}`, false},
		"nul in path":           {"source", `{"paths":["web\u0000"]}`, false},
		"line separator":        {"source", `{"branch":"a b"}`, false},
		"long branch":           {"source", `{"branch":"` + strings.Repeat("b", 33) + `"}`, false},
		"long path":             {"source", `{"paths":["` + strings.Repeat("p", 33) + `"]}`, false},
		"branch at the limit":   {"source", `{"branch":"` + strings.Repeat("b", 32) + `"}`, true},
		"registry host newline": {"registry_pull", `{"host":"a.example\n"}`, true},
		"registry host inner":   {"registry_pull", `{"host":"a\nexample"}`, false},
	} {
		_, err := ValidateRules(tc.role, []byte(tc.raw), lim)
		if tc.ok && err != nil {
			t.Errorf("%s: unexpected error %v", name, err)
		}
		if !tc.ok && !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: err = %v, want ErrInvalid", name, err)
		}
	}
	// A zero value length falls back to the default, never to no limit.
	if _, err := ValidateRules("source", []byte(`{"branch":"`+strings.Repeat("b", DefaultRuleValueMaxLen+1)+`"}`), RuleLimits{}); !errors.Is(err, ErrInvalid) {
		t.Errorf("default limit: err = %v, want ErrInvalid", err)
	}
}

func TestCoveredBindingsMatchesResolve(t *testing.T) {
	conns := map[string]Connection{
		"registry:a": {Ref: "registry:a", ScopeType: "app", ScopeID: "web"},
		"registry:b": {Ref: "registry:b", ScopeType: "app", ScopeID: "other"},
		"registry:c": {Ref: "registry:c", ScopeType: "app"},
	}
	rows := []Binding{
		{Ref: "registry:a", Role: "registry_pull", Origin: "binding"},
		{Ref: "registry:b", Role: "notify", Origin: "binding"},
		{Ref: "registry:c", Role: "dns", Origin: "binding"},
		{Ref: "registry:gone", Role: "secrets", Origin: "binding"},
		{Ref: "git:gc_1", Role: "source", Origin: "legacy"},
	}
	target := Target{Type: "app", ID: "web", TeamID: "org_default"}
	got := CoveredBindings(rows, conns, target)
	if len(got) != 2 || got[0].Ref != "registry:a" || got[1].Origin != "legacy" {
		t.Fatalf("covered = %+v", got)
	}
	if res := Resolve(rows, nil, conns, target); len(res) != 2 {
		t.Fatalf("resolve kept %d rows, want the same 2", len(res))
	}
}
