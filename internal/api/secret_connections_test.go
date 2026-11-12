package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/secretconn"
	"github.com/GLINCKER/levelrail/internal/secretprovider"
	"github.com/GLINCKER/levelrail/internal/secretsource"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/vault"
)

func newConnRouter(t *testing.T) (*Router, *store.DB) {
	rt, db := newTestRouter(t)
	sec := &fakeProviderSecrets{vals: map[string]string{}, fail: map[string]error{}}
	rt.SetSecretConnections(&secretconn.Service{
		Rows: db, Envs: db, Secrets: sec, Vault: db, Sources: secretsource.NewResolver(),
		VaultRes: vault.NewResolver(), ProvRes: secretprovider.NewResolver(),
		ProvTun: secretprovider.TunablesFrom(nil),
	})
	return rt, db
}

func TestSecretConnectionsWriteNeedsRoot(t *testing.T) {
	rt, db := newConnRouter(t)
	const plaintext = "write-scoped-token" //nolint:gosec // fixture
	if err := db.SaveAPIToken(context.Background(), store.APIToken{
		ID: "tok_w", Name: "w", TokenHash: hashToken(plaintext), Abilities: []string{AbilityWrite, AbilityRead}, CreatedAt: time.Now(),
	}); err != nil {
		t.Fatal(err)
	}
	do := func(method, path, body string) int {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+plaintext)
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, req)
		return rec.Code
	}
	for _, c := range [][3]string{
		{"POST", secretConnectionsPath, `{"name":"a","type":"vault"}`},
		{"PUT", secretConnectionsPath + "/a", `{}`},
		{"DELETE", secretConnectionsPath + "/a", ""},
		{"POST", secretConnectionsPath + "/a/test", ""},
	} {
		if got := do(c[0], c[1], c[2]); got != http.StatusForbidden {
			t.Errorf("%s %s = %d, want 403", c[0], c[1], got)
		}
	}
	if got := do("GET", secretConnectionsPath, ""); got != http.StatusOK {
		t.Errorf("list with read ability = %d", got)
	}
}

func TestSecretConnectionCredentialNeverInResponseOrAudit(t *testing.T) {
	rt, db := newConnRouter(t)
	cookie := loginTestSession(t, rt, db)
	const cred = "api-canary-93ab" //nolint:gosec // fixture
	body := `{"name":"prod-vault","type":"vault","credential":"` + cred + `","config":{"address":"https://v.example:8200"}}`
	do := func(method, path, b string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, method, path, b))
		return rec
	}
	var all strings.Builder
	rec := do("POST", secretConnectionsPath, body)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create = %d %s", rec.Code, rec.Body.String())
	}
	all.WriteString(rec.Body.String())
	all.WriteString(do("GET", secretConnectionsPath, "").Body.String())
	all.WriteString(do("PUT", secretConnectionsPath+"/prod-vault", `{"credential":"`+cred+`2","config":{"address":"https://v.example:8200"}}`).Body.String())
	all.WriteString(do("POST", secretConnectionsPath+"/prod-vault/test", "").Body.String())
	if rec := do("DELETE", secretConnectionsPath+"/prod-vault", ""); rec.Code != http.StatusNoContent {
		t.Fatalf("delete = %d", rec.Code)
	}
	entries, err := db.ListAuditEntries(context.Background(), 100, nil, store.AuditEntryFilter{Action: "secret_connection"})
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 4 {
		t.Fatalf("audit rows = %d, want created, updated, tested, deleted", len(entries))
	}
	for _, e := range entries {
		all.WriteString(e.Detail + e.Path + e.Action)
	}
	if strings.Contains(all.String(), cred) {
		t.Fatal("credential value leaked into a response or an audit row")
	}
}
