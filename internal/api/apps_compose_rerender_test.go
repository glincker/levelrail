package api

import (
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

const rerenderBody = `services:
  n8n:
    image: n8n:latest
    environment:
      WEBHOOK_URL: ${SERVICE_URL_N8N:-http://localhost:5678}
      HOST: ${SERVICE_FQDN_N8N:-localhost}
      KEEP: manual
`

func TestRerenderMemberEnv_DomainAssignedChangedRemoved(t *testing.T) {
	rev := store.ComposeRevision{AppID: "app", Compose: rerenderBody}
	member := func(domains []string, webhook string) store.DesiredService {
		return store.DesiredService{
			Name: "app-n8n", AppID: "app", Domains: domains,
			Env: map[string]string{"WEBHOOK_URL": webhook, "HOST": "localhost", "KEEP": "edited"},
		}
	}

	got, err := rerenderMemberEnv(rev, "app", []store.DesiredService{member([]string{"n8n.example.com"}, "http://localhost:5678")})
	if err != nil || len(got) != 1 {
		t.Fatalf("assigned: got %d updates, err %v", len(got), err)
	}
	if e := got[0].Env; e["WEBHOOK_URL"] != "https://n8n.example.com" || e["HOST"] != "n8n.example.com" || e["KEEP"] != "edited" {
		t.Errorf("assigned env = %+v", e)
	}

	m := member([]string{"b.example.com"}, "https://a.example.com")
	got, _ = rerenderMemberEnv(rev, "app", []store.DesiredService{m})
	if len(got) != 1 || got[0].Env["WEBHOOK_URL"] != "https://b.example.com" {
		t.Errorf("changed env = %+v", got)
	}

	m = member(nil, "https://a.example.com")
	m.Env["HOST"] = "a.example.com"
	got, _ = rerenderMemberEnv(rev, "app", []store.DesiredService{m})
	if len(got) != 1 || got[0].Env["WEBHOOK_URL"] != "http://localhost:5678" || got[0].Env["HOST"] != "localhost" {
		t.Errorf("removed env = %+v", got)
	}

	got, _ = rerenderMemberEnv(rev, "app", []store.DesiredService{member(nil, "http://localhost:5678")})
	if len(got) != 0 {
		t.Errorf("unchanged env produced %d updates, want none", len(got))
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
	rev := store.ComposeRevision{AppID: "app", Compose: rerenderMixedBody}
	m := store.DesiredService{
		Name: "app-n8n", AppID: "app", Domains: []string{"new.example.com"},
		Env: map[string]string{"N8N_ENCRYPTION_KEY": "rotated", "DB_DSN": "u:kept@old/x", "HOST": "old.example.com"},
	}
	got, err := rerenderMemberEnv(rev, "app", []store.DesiredService{m})
	if err != nil || len(got) != 1 {
		t.Fatalf("got %d updates, err %v", len(got), err)
	}
	e := got[0].Env
	if e["HOST"] != "new.example.com" {
		t.Errorf("HOST = %q, want the current domain over the install-time block", e["HOST"])
	}
	if e["N8N_ENCRYPTION_KEY"] != "rotated" || e["DB_DSN"] != "u:kept@old/x" {
		t.Errorf("secret-bearing keys changed: %+v", e)
	}

	m.Domains = nil
	got, _ = rerenderMemberEnv(rev, "app", []store.DesiredService{m})
	if len(got) != 1 || got[0].Env["HOST"] != "localhost" {
		t.Errorf("removed domain env = %+v", got)
	}
}

func TestRerenderMemberEnv_LegacyFQDNOnURLKeyRendersURL(t *testing.T) {
	rev := store.ComposeRevision{AppID: "app", Compose: "services:\n  n8n:\n    image: n8n\n    environment:\n      WEBHOOK_URL: ${SERVICE_FQDN_N8N:-http://localhost:5678}\n      N8N_HOST: ${SERVICE_FQDN_N8N:-localhost}\n"}
	m := store.DesiredService{
		Name: "app-n8n", AppID: "app", Domains: []string{"n8n.example.com"},
		Env: map[string]string{"WEBHOOK_URL": "https://n8n.example.com", "N8N_HOST": "n8n.example.com"},
	}
	got, err := rerenderMemberEnv(rev, "app", []store.DesiredService{m})
	if err != nil || len(got) != 0 {
		t.Fatalf("legacy URL key must keep its URL form: %+v, %v", got, err)
	}
}

func TestBulkPrincipalDeployGate(t *testing.T) {
	writeOnly := bulkPrincipal{abilities: []string{AbilityWrite}}
	if writeOnly.allowed(AbilityDeploy, "app-n8n") {
		t.Error("a write-only caller must not be allowed to restart a member")
	}
	if !(bulkPrincipal{abilities: []string{AbilityDeploy}}).allowed(AbilityDeploy, "app-n8n") {
		t.Error("a deploy caller must be allowed")
	}
}
