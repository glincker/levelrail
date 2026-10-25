package deploylink

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

type caseSet struct {
	Valid   []string `json:"valid"`
	Invalid []string `json:"invalid"`
}

func loadCases(t *testing.T) map[string]caseSet {
	t.Helper()
	b, err := os.ReadFile("testdata/cases.json")
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]caseSet
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	return m
}

func TestSharedCases(t *testing.T) {
	validators := map[string]func(string) error{
		"repo":     ValidateRepo,
		"branch":   ValidateBranch,
		"name":     ValidateName,
		"template": ValidateTemplate,
		"port":     func(s string) error { _, err := ParsePort(s); return err },
	}
	cases := loadCases(t)
	for field, fn := range validators {
		set := cases[field]
		if len(set.Valid) == 0 || len(set.Invalid) == 0 {
			t.Fatalf("%s: cases missing", field)
		}
		for _, v := range set.Valid {
			if err := fn(v); err != nil {
				t.Errorf("%s %q should be valid: %v", field, v, err)
			}
		}
		for _, v := range set.Invalid {
			if err := fn(v); err == nil {
				t.Errorf("%s %q should be invalid", field, v)
			}
		}
	}
}

func TestParamsValidate(t *testing.T) {
	tests := []struct {
		name string
		p    Params
		bad  bool
	}{
		{"repo only", Params{Repo: "https://github.com/you/app"}, false},
		{"template only", Params{Template: "postgres", Name: "db", Port: 5432}, false},
		{"neither", Params{Name: "x"}, true},
		{"both", Params{Repo: "https://github.com/you/app", Template: "postgres"}, true},
		{"branch without repo", Params{Template: "postgres", Branch: "main"}, true},
		{"bad port", Params{Repo: "https://github.com/you/app", Port: 70000}, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.p.Validate(); (err != nil) != tc.bad {
				t.Fatalf("err = %v, want bad=%v", err, tc.bad)
			}
		})
	}
}

func TestURLAndSnippets(t *testing.T) {
	p := Params{Repo: "https://github.com/you/app.git", Branch: "main", Name: "app", Port: 3000}
	link, err := p.URL("https://lr.example.com/")
	if err != nil {
		t.Fatal(err)
	}
	want := "https://lr.example.com/deploy?repo=https%3A%2F%2Fgithub.com%2Fyou%2Fapp.git&branch=main&name=app&port=3000"
	if link != want {
		t.Fatalf("link = %q, want %q", link, want)
	}
	if _, err := p.URL("https://u:p@lr.example.com"); err == nil {
		t.Fatal("instance with credentials accepted")
	}
	if _, err := p.URL("javascript:alert(1)"); err == nil {
		t.Fatal("non-http instance accepted")
	}
	md := Markdown("Deploy to Levelrail", link, "https://x/b.svg")
	if !strings.HasPrefix(md, "[![Deploy to Levelrail](https://x/b.svg)](https://lr.example.com/deploy?") {
		t.Fatalf("markdown = %q", md)
	}
	if h := HTML("Deploy", link, "https://x/b.svg"); !strings.Contains(h, "&amp;branch=main") {
		t.Fatalf("html not escaped: %q", h)
	}
}
