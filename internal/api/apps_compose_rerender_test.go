package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

const rerenderBody = `services:
  n8n:
    image: n8n:latest
    environment:
      WEBHOOK_URL: ${SERVICE_URL_N8N:-http://localhost:5678}
      HOST: ${SERVICE_FQDN_N8N:-localhost}
      CONFIG: ${SERVICE_URL_N8N}
      KEEP: manual
`

func rerenderMember(domains []string, env map[string]string) store.DesiredService {
	return store.DesiredService{Name: "app-n8n", AppID: "app", Port: 80, Domains: domains, Env: env}
}

func patchFor(t *testing.T, rev string, m store.DesiredService, before []string) envPatch {
	t.Helper()
	got, err := rerenderMemberEnv(store.ComposeRevision{AppID: "app", Compose: rev}, "app", []store.DesiredService{m}, m.Name, before)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) == 0 {
		return envPatch{}
	}
	return got[0]
}

func TestRerenderMemberEnv_AssignChangeRemove(t *testing.T) {
	// CONFIG has no default, so it stays unresolved with no domain and is never written.
	env := func(url, host string) map[string]string {
		return map[string]string{"WEBHOOK_URL": url, "HOST": host, "KEEP": "edited"}
	}
	p := patchFor(t, rerenderBody, rerenderMember([]string{"n8n.example.com"}, env("http://localhost:5678", "localhost")), nil)
	if p.Set["WEBHOOK_URL"] != "https://n8n.example.com" || p.Set["HOST"] != "n8n.example.com" || len(p.Set) != 2 {
		t.Errorf("assigned set = %+v", p.Set)
	}

	p = patchFor(t, rerenderBody, rerenderMember([]string{"b.example.com"}, env("https://a.example.com", "a.example.com")), []string{"a.example.com"})
	if p.Set["WEBHOOK_URL"] != "https://b.example.com" || p.Expect["WEBHOOK_URL"] != "https://a.example.com" {
		t.Errorf("changed patch = %+v", p)
	}

	p = patchFor(t, rerenderBody, rerenderMember(nil, env("https://a.example.com", "a.example.com")), []string{"a.example.com"})
	if p.Set["WEBHOOK_URL"] != "http://localhost:5678" || p.Set["HOST"] != "localhost" {
		t.Errorf("removed set = %+v", p.Set)
	}

	p = patchFor(t, rerenderBody, rerenderMember(nil, env("http://localhost:5678", "localhost")), nil)
	if len(p.Set) != 0 {
		t.Errorf("unchanged env produced %+v", p.Set)
	}
}

func TestRerenderMemberEnv_HandEditedValueIsKept(t *testing.T) {
	m := rerenderMember([]string{"b.example.com"}, map[string]string{"WEBHOOK_URL": "https://a.example.com/custom", "HOST": "a.example.com"})
	p := patchFor(t, rerenderBody, m, []string{"a.example.com"})
	if _, ok := p.Set["WEBHOOK_URL"]; ok || len(p.Kept) != 1 || p.Kept[0] != "WEBHOOK_URL" {
		t.Errorf("hand-edited key must be kept, got %+v", p)
	}
	if p.Set["HOST"] != "b.example.com" {
		t.Errorf("untouched key must still move, got %+v", p.Set)
	}
}

func TestRerenderMemberEnv_UnresolvedKeyKeepsOldValue(t *testing.T) {
	body := "services:\n  n8n:\n    image: n8n\n    environment:\n      API: https://${SERVICE_URL_N8N}/api\n"
	m := rerenderMember(nil, map[string]string{"API": "https://https://a.example.com/api"})
	p := patchFor(t, body, m, []string{"a.example.com"})
	if len(p.Set) != 0 {
		t.Errorf("unresolved key must not be written, got %+v", p.Set)
	}
}

const rerenderMixedBody = `services:
  n8n:
    image: n8n:latest
    environment:
      N8N_ENCRYPTION_KEY: ${SERVICE_PASSWORD_64_N8N}
      DB_DSN: u:${SERVICE_PASSWORD_DB}@${SERVICE_FQDN_N8N:-db}
      HOST: ${SERVICE_FQDN_N8N:-localhost}
x-levelrail-domains:
  n8n: old.example.com
`

func TestRerenderMemberEnv_SecretsSurviveAndCurrentDomainWins(t *testing.T) {
	m := rerenderMember([]string{"new.example.com"}, map[string]string{"N8N_ENCRYPTION_KEY": "rotated", "DB_DSN": "u:kept@old", "HOST": "old.example.com"})
	p := patchFor(t, rerenderMixedBody, m, []string{"old.example.com"})
	if p.Set["HOST"] != "new.example.com" || len(p.Set) != 1 {
		t.Errorf("set = %+v, want only HOST on the current domain", p.Set)
	}

	m.Domains = nil
	p = patchFor(t, rerenderMixedBody, m, []string{"old.example.com"})
	if p.Set["HOST"] != "localhost" {
		t.Errorf("removed domain set = %+v", p.Set)
	}
}

func TestRerenderMemberEnv_LegacyFQDNOnURLKeyRendersURL(t *testing.T) {
	rev := "services:\n  n8n:\n    image: n8n\n    environment:\n      WEBHOOK_URL: ${SERVICE_FQDN_N8N:-http://localhost:5678}\n      N8N_HOST: ${SERVICE_FQDN_N8N:-localhost}\n"
	m := rerenderMember([]string{"n8n.example.com"}, map[string]string{"WEBHOOK_URL": "https://n8n.example.com", "N8N_HOST": "n8n.example.com"})
	if p := patchFor(t, rev, m, []string{"n8n.example.com"}); len(p.Set) != 0 {
		t.Fatalf("legacy URL key must keep its URL form: %+v", p)
	}
}

// domainEditFixture seeds a two-service-free compose app with one n8n member.
func domainEditFixture(t *testing.T) (*Router, *store.DB, *http.Cookie) {
	t.Helper()
	rt, db, cookie := newTimelineRouter(t)
	ctx := context.Background()
	if err := db.SaveApp(ctx, store.App{ID: "app", Name: "app", CreatedAt: "2026-01-01T00:00:00Z", UpdatedAt: "2026-01-01T00:00:00Z"}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.AddComposeRevision(ctx, store.ComposeRevision{AppID: "app", Compose: rerenderBody}); err != nil {
		t.Fatal(err)
	}
	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "app-n8n", AppID: "app", Image: "n8n:latest", Port: 80, Env: map[string]string{
		"WEBHOOK_URL": "http://localhost:5678", "HOST": "localhost", "KEEP": "manual",
	}}); err != nil {
		t.Fatal(err)
	}
	return rt, db, cookie
}

func TestEditDomains_RerendersEnvAndAudits(t *testing.T) {
	rt, db, cookie := domainEditFixture(t)
	var res editDomainsResponse
	if code := tlJSON(t, rt, cookie, http.MethodPatch, "/api/v1/apps/app-n8n/domains", `{"add":["n8n.example.com"]}`, &res); code != http.StatusOK {
		t.Fatalf("patch = %d", code)
	}
	if len(res.EnvSkipped)+res.EnvSkippedHidden != 0 {
		t.Errorf("nothing should be skipped for a deploy caller: %+v", res)
	}
	svc, _ := db.GetDesiredService(context.Background(), "app-n8n")
	if svc.Env["WEBHOOK_URL"] != "https://n8n.example.com" || svc.Env["HOST"] != "n8n.example.com" {
		t.Fatalf("env = %+v", svc.Env)
	}
	rows, err := db.ListAuditEntries(context.Background(), 50, nil, store.AuditEntryFilter{})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, e := range rows {
		if e.Method == "DOMAIN_ENV_RERENDER" && strings.Contains(e.Path, "app-n8n") && strings.Contains(e.Path, "WEBHOOK_URL") && !strings.Contains(e.Path, "example.com") {
			found = true
		}
	}
	if !found {
		t.Error("no audit row for the env rewrite")
	}
}

func TestEditDomains_WriteOnlyCallerSkipsRestartedMember(t *testing.T) {
	rt, db, _ := domainEditFixture(t)
	tok := seedMatrixToken(t, db, "writer", []string{AbilityWrite})
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/apps/app-n8n/domains", strings.NewReader(`{"add":["n8n.example.com"]}`))
	bearer(tok)(req)
	rec := serve(rt, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("patch = %d %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"env_skipped`) {
		t.Errorf("response must report the skipped member: %s", rec.Body.String())
	}
	svc, _ := db.GetDesiredService(context.Background(), "app-n8n")
	if svc.Env["WEBHOOK_URL"] != "http://localhost:5678" || len(svc.Domains) != 1 {
		t.Fatalf("env must stay, domain must land: %+v", svc)
	}
}
