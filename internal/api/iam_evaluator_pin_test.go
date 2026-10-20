package api

import (
	"encoding/json"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

func pinPolicy(name, effect string, actions []string, resources ...string) store.Policy {
	doc := Document{Statement: []Statement{{Effect: Effect(effect), Action: actions, Resource: resources}}}
	return store.Policy{Name: name, Document: mustDocJSON(doc)}
}

func mustDocJSON(d Document) string {
	b, err := jsonMarshalDoc(d)
	if err != nil {
		panic(err)
	}
	return b
}

// TestEvaluatorPinned freezes today's authorizeResource results so the IAM
// authoring work can be proven not to move any decision.
func TestEvaluatorPinned(t *testing.T) {
	allowRead := pinPolicy("allow-read", "Allow", []string{AbilityRead}, "app:web")
	allowWriteWeb := pinPolicy("allow-write-web", "Allow", []string{AbilityWrite}, "app:web")
	denyWriteProd := pinPolicy("deny-write-prod", "Deny", []string{AbilityWrite}, "app:prod*")
	denyAll := pinPolicy("deny-all", "Deny", []string{"*"}, "*")
	allowAll := pinPolicy("allow-all", "Allow", []string{"*"}, "*")
	envAllow := pinPolicy("env-allow", "Allow", []string{AbilityRead}, "environment-kind:dev")
	envDeny := pinPolicy("env-deny", "Deny", []string{AbilityDeploy}, "environment:env_1")

	tests := []struct {
		name     string
		base     []string
		policies []store.Policy
		ability  string
		resource string
		env      []string
		want     bool
	}{
		{"no policy, base grants", []string{AbilityRead}, nil, AbilityRead, "app:web", nil, true},
		{"no policy, base lacks", []string{AbilityRead}, nil, AbilityWrite, "app:web", nil, false},
		{"root implies everything", []string{AbilityRoot}, nil, AbilityDeploy, "database:main", nil, true},
		{"allow grants beyond base", nil, []store.Policy{allowWriteWeb}, AbilityWrite, "app:web", nil, true},
		{"allow does not leak to other resource", nil, []store.Policy{allowWriteWeb}, AbilityWrite, "app:api", nil, false},
		{"allow of read does not grant write", nil, []store.Policy{allowRead}, AbilityWrite, "app:web", nil, false},
		{"deny beats root", []string{AbilityRoot}, []store.Policy{denyWriteProd}, AbilityWrite, "app:prod-web", nil, false},
		{"deny prefix does not touch others", []string{AbilityRoot}, []store.Policy{denyWriteProd}, AbilityWrite, "app:staging", nil, true},
		{"deny beats allow in separate policies", nil, []store.Policy{allowAll, denyAll}, AbilityRead, "app:web", nil, false},
		{"deny beats allow in reverse order", nil, []store.Policy{denyAll, allowAll}, AbilityRead, "app:web", nil, false},
		{"allow all grants anything", nil, []store.Policy{allowAll}, AbilityRoot, "app:web", nil, true},
		{"malformed document is inert", []string{AbilityRead}, []store.Policy{{Name: "bad", Document: "{"}}, AbilityRead, "app:web", nil, true},
		{"malformed document grants nothing", nil, []store.Policy{{Name: "bad", Document: "{"}}, AbilityRead, "app:web", nil, false},
		{"environment kind allow via extra resource", nil, []store.Policy{envAllow}, AbilityRead, "app:web", []string{"environment-kind:dev"}, true},
		{"environment kind allow absent without tag", nil, []store.Policy{envAllow}, AbilityRead, "app:web", nil, false},
		{"environment id deny via extra resource", []string{AbilityDeploy}, []store.Policy{envDeny}, AbilityDeploy, "app:web", []string{"environment:env_1"}, false},
		{"environment id deny other env", []string{AbilityDeploy}, []store.Policy{envDeny}, AbilityDeploy, "app:web", []string{"environment:env_2"}, true},
		{"star resource matches only star-pattern policies", nil, []store.Policy{allowRead}, AbilityRead, "*", nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := authorizeResource(tt.base, tt.policies, tt.ability, tt.resource, tt.env...); got != tt.want {
				t.Errorf("authorizeResource = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestEvaluatePoliciesMatchedFlag(t *testing.T) {
	tests := []struct {
		name        string
		policies    []store.Policy
		wantMatched bool
		wantAllowed bool
	}{
		{"nothing mentions the pair", []store.Policy{pinPolicy("a", "Allow", []string{AbilityRead}, "app:other")}, false, false},
		{"allow matched", []store.Policy{pinPolicy("a", "Allow", []string{AbilityWrite}, "app:web")}, true, true},
		{"deny matched", []store.Policy{pinPolicy("a", "Deny", []string{AbilityWrite}, "app:*")}, true, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, a := evaluatePolicies(tt.policies, AbilityWrite, "app:web")
			if m != tt.wantMatched || a != tt.wantAllowed {
				t.Errorf("evaluatePolicies = (%v, %v), want (%v, %v)", m, a, tt.wantMatched, tt.wantAllowed)
			}
		})
	}
}

func jsonMarshalDoc(d Document) (string, error) {
	b, err := json.Marshal(d)
	return string(b), err
}
