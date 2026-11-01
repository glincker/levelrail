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

func noRerenderGen(string, string, int) (string, error) { return "", nil }
func noRerenderPersist(string, string, string) error    { return nil }

func TestRerenderMemberEnv_DomainAssignedChangedRemoved(t *testing.T) {
	rev := store.ComposeRevision{AppID: "app", Compose: rerenderBody}
	member := func(domains []string, webhook string) store.DesiredService {
		return store.DesiredService{
			Name: "app-n8n", AppID: "app", Domains: domains,
			Env: map[string]string{"WEBHOOK_URL": webhook, "HOST": "localhost", "KEEP": "edited"},
		}
	}

	got, err := rerenderMemberEnv(rev, "app", []store.DesiredService{member([]string{"n8n.example.com"}, "http://localhost:5678")}, noRerenderGen, noRerenderPersist)
	if err != nil || len(got) != 1 {
		t.Fatalf("assigned: got %d updates, err %v", len(got), err)
	}
	if e := got[0].Env; e["WEBHOOK_URL"] != "https://n8n.example.com" || e["HOST"] != "n8n.example.com" || e["KEEP"] != "edited" {
		t.Errorf("assigned env = %+v", e)
	}

	m := member([]string{"b.example.com"}, "https://a.example.com")
	got, _ = rerenderMemberEnv(rev, "app", []store.DesiredService{m}, noRerenderGen, noRerenderPersist)
	if len(got) != 1 || got[0].Env["WEBHOOK_URL"] != "https://b.example.com" {
		t.Errorf("changed env = %+v", got)
	}

	m = member(nil, "https://a.example.com")
	m.Env["HOST"] = "a.example.com"
	got, _ = rerenderMemberEnv(rev, "app", []store.DesiredService{m}, noRerenderGen, noRerenderPersist)
	if len(got) != 1 || got[0].Env["WEBHOOK_URL"] != "http://localhost:5678" || got[0].Env["HOST"] != "localhost" {
		t.Errorf("removed env = %+v", got)
	}

	got, _ = rerenderMemberEnv(rev, "app", []store.DesiredService{member(nil, "http://localhost:5678")}, noRerenderGen, noRerenderPersist)
	if len(got) != 0 {
		t.Errorf("unchanged env produced %d updates, want none", len(got))
	}
}
