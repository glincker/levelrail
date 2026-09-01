package api

import (
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestParseDocument_Valid(t *testing.T) {
	doc, err := ParseDocument(`{"Statement":[{"Effect":"Allow","Action":["read"],"Resource":["app:web"]}]}`)
	if err != nil {
		t.Fatalf("ParseDocument() error = %v", err)
	}
	if len(doc.Statement) != 1 || doc.Statement[0].Effect != EffectAllow {
		t.Errorf("got %+v", doc)
	}
}

func TestParseDocument_Errors(t *testing.T) {
	tests := []struct {
		name string
		doc  string
	}{
		{"invalid json", `not json`},
		{"no statements", `{"Statement":[]}`},
		{"bad effect", `{"Statement":[{"Effect":"Maybe","Action":["read"],"Resource":["*"]}]}`},
		{"no action", `{"Statement":[{"Effect":"Allow","Action":[],"Resource":["*"]}]}`},
		{"unknown action", `{"Statement":[{"Effect":"Allow","Action":["fly"],"Resource":["*"]}]}`},
		{"no resource", `{"Statement":[{"Effect":"Allow","Action":["read"],"Resource":[]}]}`},
		{"blank resource", `{"Statement":[{"Effect":"Allow","Action":["read"],"Resource":[" "]}]}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := ParseDocument(tt.doc); err == nil {
				t.Errorf("ParseDocument(%q) error = nil, want an error", tt.doc)
			}
		})
	}
}

func TestParseDocument_WildcardActionAndResourceAllowed(t *testing.T) {
	if _, err := ParseDocument(`{"Statement":[{"Effect":"Deny","Action":["*"],"Resource":["*"]}]}`); err != nil {
		t.Errorf("ParseDocument() error = %v, want wildcard action/resource accepted", err)
	}
}

func TestMatchesPattern(t *testing.T) {
	tests := []struct {
		pattern, value string
		want           bool
	}{
		{"*", "anything", true},
		{"app:web", "app:web", true},
		{"app:web", "app:api", false},
		{"app:*", "app:web", true},
		{"app:*", "database:main", false},
		{"database:*", "database:main", true},
	}
	for _, tt := range tests {
		if got := matchesPattern(tt.pattern, tt.value); got != tt.want {
			t.Errorf("matchesPattern(%q, %q) = %v, want %v", tt.pattern, tt.value, got, tt.want)
		}
	}
}

func allowPolicy(name string, action, resource string) store.Policy {
	return store.Policy{
		Name:     name,
		Document: `{"Statement":[{"Effect":"Allow","Action":["` + action + `"],"Resource":["` + resource + `"]}]}`,
	}
}

func denyPolicy(name string, action, resource string) store.Policy {
	return store.Policy{
		Name:     name,
		Document: `{"Statement":[{"Effect":"Deny","Action":["` + action + `"],"Resource":["` + resource + `"]}]}`,
	}
}

func TestAuthorizeResource_NoPolicies_FallsBackToBaseAbilities(t *testing.T) {
	if !authorizeResource([]string{AbilityRead}, nil, AbilityRead, "app:web") {
		t.Error("want allowed: base ability grants it and there are no policies to interfere")
	}
	if authorizeResource([]string{AbilityRead}, nil, AbilityWrite, "app:web") {
		t.Error("want denied: base abilities don't grant write and there are no policies")
	}
}

func TestAuthorizeResource_ExplicitDenyOverridesBaseAbility(t *testing.T) {
	policies := []store.Policy{denyPolicy("deny-write-prod", AbilityWrite, "app:prod")}
	if authorizeResource([]string{AbilityRoot}, policies, AbilityWrite, "app:prod") {
		t.Error("want denied: an explicit Deny must override even a root-derived base ability")
	}
	if !authorizeResource([]string{AbilityRoot}, policies, AbilityWrite, "app:staging") {
		t.Error("want allowed: the Deny only scopes to app:prod, other resources are unaffected")
	}
}

func TestAuthorizeResource_ExplicitAllowGrantsBeyondBaseAbilities(t *testing.T) {
	policies := []store.Policy{allowPolicy("scoped-write", AbilityWrite, "app:web")}
	if !authorizeResource([]string{AbilityRead}, policies, AbilityWrite, "app:web") {
		t.Error("want allowed: the policy grants write on this resource even though base abilities only have read")
	}
	if authorizeResource([]string{AbilityRead}, policies, AbilityWrite, "app:other") {
		t.Error("want denied: the Allow only scopes to app:web")
	}
}

func TestAuthorizeResource_MalformedPolicyDocumentIgnored(t *testing.T) {
	policies := []store.Policy{{Name: "broken", Document: `not json`}}
	if authorizeResource([]string{AbilityRead}, policies, AbilityWrite, "app:web") {
		t.Error("want denied: a malformed document should be inert, not accidentally granting")
	}
}

func TestAuthorizeResource_WildcardResourceInPolicy(t *testing.T) {
	policies := []store.Policy{allowPolicy("all-apps-read", AbilityRead, "app:*")}
	if !authorizeResource(nil, policies, AbilityRead, "app:anything") {
		t.Error("want allowed: app:* should match any app: resource")
	}
	if authorizeResource(nil, policies, AbilityRead, "database:main") {
		t.Error("want denied: app:* must not match a database: resource")
	}
}
