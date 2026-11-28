package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/glincker/levelrail/internal/store"
)

func TestResourceMentionsEnvironment(t *testing.T) {
	for r, want := range map[string]bool{
		"environment:env1": true, "environment-kind:production": true, "environment-kind:*": true,
		"environment*": true, "environment-kind*": true, "env*": true, "e*": true, "environment-k*": true,
		"*": false, "app:*": false, "app:web": false, "database:prod": false, "x*": false,
	} {
		if got := resourceMentionsEnvironment(r); got != want {
			t.Errorf("resourceMentionsEnvironment(%q) = %v, want %v", r, got, want)
		}
	}
}

// Policies naming only an environment kind must still resolve the app's
// environment, for an unrestricted caller too.
func TestEnvironmentKindPolicyAppliesToDeployRoute(t *testing.T) {
	denyKind := func(res string) string { return stmtDoc("Deny", `"deploy"`, `"`+res+`"`) }
	cases := []struct {
		name string
		docs []string
		kind string
		deny bool
	}{
		{"ai-operator-nonprod denies production", []string{templateDoc(t, "ai-operator-nonprod")}, store.EnvironmentKindProduction, true},
		{"ai-operator-nonprod allows dev", []string{templateDoc(t, "ai-operator-nonprod")}, store.EnvironmentKindDev, false},
		{"deployer-nonprod denies production", []string{templateDoc(t, "deployer-nonprod")}, store.EnvironmentKindProduction, true},
		{"deployer-nonprod allows dev", []string{templateDoc(t, "deployer-nonprod")}, store.EnvironmentKindDev, false},
		{"deny kind production", []string{denyKind("environment-kind:production")}, store.EnvironmentKindProduction, true},
		{"deny kind production leaves dev", []string{denyKind("environment-kind:production")}, store.EnvironmentKindDev, false},
		{"deny kind wildcard", []string{denyKind("environment-kind:*")}, store.EnvironmentKindDev, true},
		{"deny environment wildcard", []string{denyKind("environment:*")}, store.EnvironmentKindDev, true},
	}
	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rt, db, _ := newPipelineRouter(t)
			bootstrapTestAdmin(t, db)
			ctx := context.Background()
			must := func(err error) {
				t.Helper()
				if err != nil {
					t.Fatal(err)
				}
			}
			must(db.SaveProject(ctx, store.Project{ID: "proj1", Name: "proj1", CreatedAt: "2026-10-01T00:00:00Z"}))
			must(db.SaveEnvironment(ctx, store.Environment{ID: "env1", ProjectID: "proj1", Name: "e", Kind: tc.kind, Scope: store.EnvironmentScopeProject, CreatedAt: "2026-10-01T00:00:00Z"}))
			must(db.SaveDesiredService(ctx, store.DesiredService{Name: "web", Image: "web:1", Port: 80}))
			must(db.SetServiceEnvironment(ctx, "web", "env1"))
			id := fmt.Sprintf("envkind-%d", i)
			plain := seedMatrixToken(t, db, id, []string{AbilityRead, AbilityWrite, AbilityDeploy})
			for _, d := range tc.docs {
				attachDoc(t, db, d, store.PrincipalTypeToken, id)
			}
			code := matrixDo(rt, http.MethodDelete, "/api/v1/apps/web/autoscale", bearer(plain))
			if denied := code == http.StatusForbidden; denied != tc.deny {
				t.Fatalf("status %d, denied=%v want %v", code, denied, tc.deny)
			}
		})
	}
}

func attachDoc(t *testing.T, db *store.DB, doc, ptype, pid string) {
	t.Helper()
	ctx := context.Background()
	pidv, err := store.NewPolicyID()
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	if err := db.SavePolicy(ctx, store.Policy{ID: pidv, Name: pidv, Document: doc, CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	aid, err := store.NewPolicyAttachmentID()
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AttachPolicy(ctx, aid, pidv, ptype, pid); err != nil {
		t.Fatal(err)
	}
}

func templateDoc(t *testing.T, id string) string {
	t.Helper()
	tpl, ok := findPolicyTemplate(id)
	if !ok {
		t.Fatalf("template %q missing", id)
	}
	raw, err := json.Marshal(tpl.build(map[string]string{templateParamDatabase: "prod", templateParamProject: "proj1"}))
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func stmtDoc(effect string, actions string, resources string) string {
	return `{"Statement":[{"Effect":"` + effect + `","Action":[` + actions + `],"Resource":[` + resources + `]}]}`
}
