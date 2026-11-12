package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/envredact"
	"github.com/GLINCKER/levelrail/internal/runtimesettings"
	"github.com/GLINCKER/levelrail/internal/secretconn"
	"github.com/GLINCKER/levelrail/internal/secretsource"
	"github.com/GLINCKER/levelrail/internal/store"
)

const policyLiteral = "sk_live_0123456789abcdef0123" //nolint:gosec // fixture

func seedPolicyApp(t *testing.T, rt *Router, db *store.DB, envName string) {
	t.Helper()
	ctx := context.Background()
	rt.runtimeSettings = runtimesettings.New(db, func(string) (string, bool) { return "", false })
	if err := db.SaveProject(ctx, store.Project{ID: "proj_1", Name: "saas", CreatedAt: "2026-08-20T00:00:00Z"}); err != nil {
		t.Fatal(err)
	}
	if err := db.SaveEnvironment(ctx, store.Environment{ID: "env-" + envName, ProjectID: "proj_1", Name: envName, CreatedAt: "2026-08-20T00:00:00Z"}); err != nil {
		t.Fatal(err)
	}
	app := store.DesiredService{Name: "web", Image: "img:v1", Port: 80, EnvironmentID: "env-" + envName, Env: map[string]string{"STRIPE_KEY": policyLiteral, "PORT": "80"}}
	if err := db.SaveDesiredService(ctx, app); err != nil {
		t.Fatal(err)
	}
	if err := db.SetServiceEnvironment(ctx, "web", "env-"+envName); err != nil {
		t.Fatal(err)
	}
}

func setPolicy(t *testing.T, rt *Router, mode string, envs ...string) {
	t.Helper()
	ctx := context.Background()
	list, _ := json.Marshal(append([]string{}, envs...))
	if _, err := rt.RuntimeSettings().Set(ctx, runtimesettings.KeySecretsRequireRefEnvironments, list, "test"); err != nil {
		t.Fatal(err)
	}
	m, _ := json.Marshal(mode)
	if _, err := rt.RuntimeSettings().Set(ctx, runtimesettings.KeySecretsRequireRefMode, m, "test"); err != nil {
		t.Fatal(err)
	}
}

func TestSecretPolicyModeMatrix(t *testing.T) {
	rt, db := newTestRouter(t)
	seedPolicyApp(t, rt, db, "Production")
	ctx := context.Background()
	for _, c := range []struct {
		mode string
		envs []string
		want string
	}{
		{"off", []string{"production"}, "off"},
		{"warn", nil, "off"},
		{"warn", []string{"staging"}, "off"},
		{"warn", []string{"PRODUCTION"}, "warn"},
		{"enforce", []string{"production"}, "enforce"},
	} {
		setPolicy(t, rt, c.mode, c.envs...)
		if got, _ := rt.secretPolicyMode(ctx, "env-Production"); got != c.want {
			t.Errorf("mode %s envs %v = %s, want %s", c.mode, c.envs, got, c.want)
		}
	}
	if got, _ := rt.secretPolicyMode(ctx, ""); got != "off" {
		t.Error("an app without an environment is never affected")
	}
}

func TestLiteralSecretKeysSkipsPlaceholdersUnchangedAndRefs(t *testing.T) {
	prior := map[string]string{"OLD_TOKEN": policyLiteral}
	got := literalSecretKeys(map[string]string{
		"OLD_TOKEN": policyLiteral, "NEW_TOKEN": policyLiteral, "PORT": "80", "DB_PASSWORD": envredact.Placeholder, "API_SECRET": "",
	}, prior, nil)
	if len(got) != 1 || got[0] != "NEW_TOKEN" {
		t.Fatalf("keys = %v, want only NEW_TOKEN", got)
	}
}

func TestSecretPolicyEnforceNamesKeyNotValue(t *testing.T) {
	rt, db := newTestRouter(t)
	seedPolicyApp(t, rt, db, "production")
	setPolicy(t, rt, "enforce", "production")
	rec := httptest.NewRecorder()
	if rt.enforceSecretPolicy(context.Background(), rec, "env-production", []string{"STRIPE_KEY"}) {
		t.Fatal("enforce must refuse")
	}
	body := rec.Body.String()
	if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(body, "STRIPE_KEY") || !strings.Contains(body, "secrets.require_ref_mode") || strings.Contains(body, policyLiteral) {
		t.Fatalf("status %d body %s", rec.Code, body)
	}
	setPolicy(t, rt, "warn", "production")
	if !rt.enforceSecretPolicy(context.Background(), httptest.NewRecorder(), "env-production", []string{"STRIPE_KEY"}) {
		t.Fatal("warn must not refuse")
	}
}

func TestRequireRefWarningsInAppGet(t *testing.T) {
	rt, db := newTestRouter(t)
	seedPolicyApp(t, rt, db, "production")
	root := seedMatrixToken(t, db, "rootp", []string{AbilityRoot})
	get := func() map[string]any {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/apps/web", nil)
		req.Header.Set("Authorization", "Bearer "+root)
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, req)
		if strings.Contains(rec.Body.String(), policyLiteral) {
			t.Fatal("literal leaked in apps get")
		}
		var out map[string]any
		_ = json.Unmarshal(rec.Body.Bytes(), &out)
		return out
	}
	if _, ok := get()["require_ref_warnings"]; ok {
		t.Fatal("no warnings while the policy is off")
	}
	setPolicy(t, rt, "warn", "production")
	w, _ := get()["require_ref_warnings"].([]any)
	if len(w) != 1 || !strings.Contains(w[0].(string), "STRIPE_KEY") {
		t.Fatalf("warnings = %v", w)
	}
	setPolicy(t, rt, "enforce", "production")
	if w, _ := get()["require_ref_warnings"].([]any); len(w) != 1 {
		t.Fatalf("existing literals stay listed under enforce: %v", w)
	}
}

func TestSecretPolicyEnforceRefusesNewLiteralOnAppUpdate(t *testing.T) {
	rt, db := newTestRouter(t)
	seedPolicyApp(t, rt, db, "production")
	setPolicy(t, rt, "enforce", "production")
	root := seedMatrixToken(t, db, "rootq", []string{AbilityRoot})
	put := func(body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPut, "/api/v1/apps/web", strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+root)
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, req)
		return rec
	}
	rec := put(`{"image":"img:v1","port":80,"env":{"STRIPE_KEY":"` + policyLiteral + `","NEW_TOKEN":"` + policyLiteral + `x"}}`)
	if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), "NEW_TOKEN") || strings.Contains(rec.Body.String(), "STRIPE_KEY,") || strings.Contains(rec.Body.String(), policyLiteral) {
		t.Fatalf("new literal = %d %s", rec.Code, rec.Body.String())
	}
	if rec := put(`{"image":"img:v1","port":80,"env":{"STRIPE_KEY":"` + policyLiteral + `","PORT":"81"}}`); rec.Code != http.StatusOK {
		t.Fatalf("unchanged literal and plain edit = %d %s", rec.Code, rec.Body.String())
	}
}

// Delegation: a writer may set a reference only inside the connection's lists for the app's environment.
func TestDelegatedSecretRefWriteHonoursConnectionLists(t *testing.T) {
	rt, db := newConnRouter(t)
	seedPolicyApp(t, rt, db, "production")
	svc := rt.secretConns.(*secretconn.Service)
	svc.Build = func(secretconn.Connection) secretsource.Provider { return secretsource.NewFake("team") }
	prefixes, envs := []string{"team-a"}, []string{"production"}
	if _, err := svc.Create(context.Background(), "root", "team", secretconn.Input{Type: secretconn.TypeDoppler, Credential: "tok",
		Config: secretconn.Config{Project: "p", Environment: "e"}, AllowedPrefixes: &prefixes, AllowedEnvs: &envs}); err != nil {
		t.Fatal(err)
	}
	writer := seedMatrixToken(t, db, "wsd", []string{AbilityWrite})
	for ref, want := range map[string]int{
		`{"ref":"secret://team/team-a/db#pw"}`:      http.StatusOK,
		`{"ref":"secret://team/team-ab/db#pw"}`:     http.StatusForbidden,
		`{"ref":"secret://team/other/db#pw"}`:       http.StatusForbidden,
		`{"ref":"secret://nope/team-a/db#pw"}`:      http.StatusForbidden,
		`{"conn":"team","path":"team-a","key":"k"}`: http.StatusOK,
	} {
		if rec := putRefAs(rt, writer, ref); rec.Code != want {
			t.Errorf("%s = %d, want %d: %s", ref, rec.Code, want, rec.Body.String())
		}
	}
	// A different environment is refused for the same connection and path.
	if err := db.SetServiceEnvironment(context.Background(), "web", ""); err != nil {
		t.Fatal(err)
	}
	if rec := putRefAs(rt, writer, `{"ref":"secret://team/team-a/db#pw"}`); rec.Code != http.StatusForbidden {
		t.Errorf("app without an allowed environment = %d, want 403", rec.Code)
	}
	entries, _ := db.ListAuditEntries(context.Background(), 50, nil, store.AuditEntryFilter{Action: "app.secret_ref.denied"})
	if len(entries) == 0 {
		t.Error("a refused write must be audited")
	}
}
