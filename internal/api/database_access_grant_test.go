package api

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/dbaccess"
	"github.com/GLINCKER/levelrail/internal/store"
)

func TestDatabaseTemplates_ScopeToOneDatabase(t *testing.T) {
	cases := []struct {
		template string
		ability  string
		resource string
		want     bool
	}{
		{"database-read-only", AbilityReadSensitive, "database:main", true},
		{"database-read-only", AbilityWrite, "database:main", false},
		{"database-read-only", AbilityReadSensitive, "database:other", false},
		{"database-operator", AbilityWriteSensitive, "database:main", true},
		{"database-operator", AbilityRoot, "database:main", false},
		{"database-owner", AbilityRoot, "database:main", true},
		{"database-owner", AbilityRoot, "database:other", false},
		{"database-owner", AbilityRoot, "app:web", false},
	}
	for _, c := range cases {
		pol := renderedTemplatePolicy(t, c.template, map[string]string{templateParamDatabase: "main"})
		got := authorizeResource(nil, []store.Policy{pol}, c.ability, c.resource)
		if got != c.want {
			t.Errorf("%s %s on %s = %v, want %v", c.template, c.ability, c.resource, got, c.want)
		}
	}
}

func TestDatabaseAccess_GrantPreviewApplyAndWho(t *testing.T) {
	rt, db, _ := newAccessTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()
	const plaintext = "dev-token-plaintext" //nolint:gosec // fake fixture, not a real credential
	if err := db.SaveAPIToken(ctx, store.APIToken{ID: "tok_dev", Name: "dev laptop", TokenHash: hashToken(plaintext), Abilities: []string{AbilityRead}, CreatedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	body := `{"template":"database-operator","principal_type":"token","principal_id":"tok_dev","preview":true}`
	prev := viewerDo(t, rt, cookie, http.MethodPost, "/api/v1/databases/main/access/grants", body)
	if prev.Code != http.StatusOK || !strings.Contains(prev.Body.String(), "database:main") || !strings.Contains(prev.Body.String(), `"applied":false`) {
		t.Fatalf("preview = %d %s", prev.Code, prev.Body.String())
	}
	if pols, _ := db.ListPolicies(ctx); len(pols) != 0 {
		t.Fatalf("preview wrote %d policies", len(pols))
	}
	apply := viewerDo(t, rt, cookie, http.MethodPost, "/api/v1/databases/main/access/grants", strings.Replace(body, `,"preview":true`, "", 1))
	if apply.Code != http.StatusOK || !strings.Contains(apply.Body.String(), `"applied":true`) {
		t.Fatalf("apply = %d %s", apply.Code, apply.Body.String())
	}
	who := viewerDo(t, rt, cookie, http.MethodGet, "/api/v1/databases/main/access/principals", "")
	if who.Code != http.StatusOK || !strings.Contains(who.Body.String(), "dev laptop") || !strings.Contains(who.Body.String(), "policy database-operator-main") {
		t.Fatalf("who = %d %s", who.Code, who.Body.String())
	}
	if strings.Contains(who.Body.String(), plaintext) || strings.Contains(who.Body.String(), hashToken(plaintext)) {
		t.Fatal("principal listing leaked token material")
	}
	entries, _ := db.ListAuditEntries(ctx, 50, nil, store.AuditEntryFilter{})
	var audited bool
	for _, e := range entries {
		audited = audited || e.Method == dbaccess.ActionGrantApply
	}
	if !audited {
		t.Error("grant was not audited")
	}
}
