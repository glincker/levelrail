package api

import (
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

func policyOf(effect, action string, resources ...string) store.Policy {
	quoted := make([]string, len(resources))
	for i, r := range resources {
		quoted[i] = `"` + r + `"`
	}
	return store.Policy{Document: `{"Statement":[{"Effect":"` + effect + `","Action":["` + action + `"],"Resource":[` + strings.Join(quoted, ",") + `]}]}`}
}

func TestAuthorizeResource_EnvironmentMatrix(t *testing.T) {
	prodRef := &store.EnvironmentRef{ID: "env_production", Kind: "production"}
	devRef := &store.EnvironmentRef{ID: "env_dev", Kind: "dev"}
	tests := []struct {
		name    string
		ability string
		base    []string
		policy  []store.Policy
		ref     *store.EnvironmentRef
		want    bool
	}{
		{"deny kind production blocks base deploy", AbilityDeploy, []string{AbilityDeploy}, []store.Policy{policyOf("Deny", "deploy", "environment-kind:production")}, prodRef, false},
		{"deny kind production leaves dev", AbilityDeploy, []string{AbilityDeploy}, []store.Policy{policyOf("Deny", "deploy", "environment-kind:production")}, devRef, true},
		{"deny kind production does not match untagged", AbilityDeploy, []string{AbilityDeploy}, []store.Policy{policyOf("Deny", "deploy", "environment-kind:production")}, nil, true},
		{"deny environment id", AbilityDeploy, []string{AbilityDeploy}, []store.Policy{policyOf("Deny", "deploy", "environment:env_production")}, prodRef, false},
		{"deny environment wildcard untagged unaffected", AbilityDeploy, []string{AbilityDeploy}, []store.Policy{policyOf("Deny", "deploy", "environment:*")}, nil, true},
		{"deny environment wildcard tagged", AbilityDeploy, []string{AbilityDeploy}, []store.Policy{policyOf("Deny", "deploy", "environment:*")}, devRef, false},
		{"deny kind wildcard", AbilityDeploy, []string{AbilityDeploy}, []store.Policy{policyOf("Deny", "deploy", "environment-kind:*")}, devRef, false},
		{"deny other action leaves deploy", AbilityDeploy, []string{AbilityDeploy}, []store.Policy{policyOf("Deny", "write", "environment-kind:production")}, prodRef, true},
		{"allow kind dev grants without base", AbilityDeploy, nil, []store.Policy{policyOf("Allow", "deploy", "environment-kind:dev")}, devRef, true},
		{"allow kind dev does not grant prod", AbilityDeploy, nil, []store.Policy{policyOf("Allow", "deploy", "environment-kind:dev")}, prodRef, false},
		{"allow kind dev does not grant untagged", AbilityDeploy, nil, []store.Policy{policyOf("Allow", "deploy", "environment-kind:dev")}, nil, false},
		{"allow environment id grants write", AbilityWrite, nil, []store.Policy{policyOf("Allow", "write", "environment:env_dev")}, devRef, true},
		{"allow wrong action", AbilityDeploy, nil, []store.Policy{policyOf("Allow", "read", "environment:env_dev")}, devRef, false},
		{"deny beats allow across resources", AbilityDeploy, nil, []store.Policy{policyOf("Allow", "deploy", "environment-kind:dev"), policyOf("Deny", "deploy", "environment:env_dev")}, devRef, false},
		{"deny on the app beats env allow", AbilityDeploy, nil, []store.Policy{policyOf("Allow", "deploy", "environment-kind:dev"), policyOf("Deny", "deploy", "app:web", "database:main")}, devRef, false},
		{"deny star on production beats root", AbilityDeploy, []string{AbilityRoot}, []store.Policy{policyOf("Deny", "*", "environment-kind:production")}, prodRef, false},
		{"no policies falls back to base", AbilityWrite, []string{AbilityWrite}, nil, prodRef, true},
		{"no policies and no base", AbilityWrite, nil, nil, prodRef, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, resource := range []string{"app:web", "database:main"} {
				got := authorizeResource(tt.base, tt.policy, tt.ability, resource, environmentResources(tt.ref)...)
				if got != tt.want {
					t.Errorf("%s authorize = %v, want %v", resource, got, tt.want)
				}
			}
		})
	}
}

func TestEnvironmentResources(t *testing.T) {
	if got := environmentResources(nil); got != nil {
		t.Errorf("untagged = %v, want nil", got)
	}
	got := environmentResources(&store.EnvironmentRef{ID: "env_uat", Kind: "uat"})
	if len(got) != 2 || got[0] != "environment:env_uat" || got[1] != "environment-kind:uat" {
		t.Errorf("got %v", got)
	}
}

func TestParseDocument_EnvironmentResources(t *testing.T) {
	doc := func(r string) string {
		return `{"Statement":[{"Effect":"Deny","Action":["deploy"],"Resource":["` + r + `"]}]}`
	}
	valid := []string{"environment:*", "environment:env_dev", "environment:env_*", "environment-kind:production", "environment-kind:*", "environment-kind:dev", "environment-kind:pre*", "app:web", "*"}
	for _, r := range valid {
		if _, err := ParseDocument(doc(r)); err != nil {
			t.Errorf("ParseDocument(%q) error = %v, want valid", r, err)
		}
	}
	invalid := []string{"environment:", "environment-kind:", "environment-kind:staging", "environment-kind:prod uction", "environment:a*b", "environment-kind:*x", "environment: ", " "}
	for _, r := range invalid {
		if _, err := ParseDocument(doc(r)); err == nil {
			t.Errorf("ParseDocument(%q) accepted, want error", r)
		}
	}
}
