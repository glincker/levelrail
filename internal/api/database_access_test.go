package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/dbaccess"
	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/store"
)

// scriptedRuntime answers each exec with the next canned stdout and records
// every script it was sent.
type scriptedRuntime struct {
	*fakeExecAppRuntime
	scripts []string
	outs    []string
}

func (s *scriptedRuntime) ExecWithInput(_ context.Context, _ string, _ []string, stdin io.Reader) (io.ReadCloser, error) {
	b, _ := io.ReadAll(stdin)
	s.scripts = append(s.scripts, string(b))
	out := ""
	if len(s.outs) > 0 {
		out, s.outs = s.outs[0], s.outs[1:]
	}
	return io.NopCloser(strings.NewReader(out)), nil
}

func newAccessTestRouter(t *testing.T) (*Router, *store.DB, *scriptedRuntime) {
	t.Helper()
	db := openTestDB(t)
	fake := &scriptedRuntime{fakeExecAppRuntime: &fakeExecAppRuntime{inspectState: &docker.ContainerState{ID: "c1", Running: true}}}
	rt := NewRouter(discardLogger(), testBrand(), db,
		WithExecRuntime(func(string) (docker.Runtime, error) { return fake, nil }),
		WithDatabaseAccess(db, dbaccess.DefaultTTLLimits()),
	)
	if err := db.SaveDesiredDatabase(context.Background(), store.DesiredDatabase{Name: "main", Engine: store.EnginePostgres, Version: "16"}); err != nil {
		t.Fatalf("seed database: %v", err)
	}
	return rt, db, fake
}

const rolesCSV = "main,t,t,t,t,f,-1,,2\napp_reader,t,f,f,f,f,5,,0\n"

func TestDatabaseUsers_CreateKeepsPasswordOutOfEverything(t *testing.T) {
	rt, db, fake := newAccessTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	fake.outs = []string{"", ""} // role listing is empty, then the create transaction

	rec := viewerDo(t, rt, cookie, http.MethodPost, "/api/v1/databases/main/users", `{"name":"report_ro","preset":"read_only"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var created struct {
		Credential struct {
			Password    string `json:"password"`
			InternalURL string `json:"internal_url"`
		} `json:"credential"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	pw := created.Credential.Password
	if len(pw) < 20 || !strings.Contains(created.Credential.InternalURL, pw) {
		t.Fatalf("credential not revealed once: %+v", created.Credential)
	}
	script := fake.scripts[len(fake.scripts)-1]
	if strings.Contains(script, pw) || !strings.Contains(script, "SCRAM-SHA-256$") {
		t.Fatalf("plaintext password reached SQL:\n%s", script)
	}
	entries, err := db.ListAuditEntries(context.Background(), 50, nil, store.AuditEntryFilter{})
	if err != nil {
		t.Fatal(err)
	}
	foundAction := false
	for _, e := range entries {
		blob := e.Path + e.Method + e.ActorName + e.RemoteAddr
		if strings.Contains(blob, pw) {
			t.Fatalf("password leaked into audit entry %+v", e)
		}
		if e.Method == dbaccess.ActionUserCreate {
			foundAction = true
		}
	}
	if !foundAction {
		t.Error("create was not audited with its action constant")
	}

	fake.outs = []string{rolesCSV + "report_ro,t,f,f,f,f,20,,0\n"}
	list := viewerDo(t, rt, cookie, http.MethodGet, "/api/v1/databases/main/users", "")
	if list.Code != http.StatusOK || strings.Contains(list.Body.String(), pw) || strings.Contains(list.Body.String(), "SCRAM") {
		t.Fatalf("list leaked or failed: %d %s", list.Code, list.Body.String())
	}
	if !strings.Contains(list.Body.String(), `"report_ro"`) || !strings.Contains(list.Body.String(), `"protected":true`) {
		t.Errorf("list body = %s", list.Body.String())
	}
}

func TestDatabaseUsers_GuardRails(t *testing.T) {
	cases := []struct {
		name, method, path, body string
		want                     int
	}{
		{"create collides with admin role", "POST", "/api/v1/databases/main/users", `{"name":"main"}`, http.StatusBadRequest},
		{"create collides with system role", "POST", "/api/v1/databases/main/users", `{"name":"postgres"}`, http.StatusBadRequest},
		{"create bad preset", "POST", "/api/v1/databases/main/users", `{"name":"someone","preset":"dba"}`, http.StatusBadRequest},
		{"create reserved temp prefix", "POST", "/api/v1/databases/main/users", `{"name":"tmp_abc"}`, http.StatusBadRequest},
		{"rotate admin refused", "POST", "/api/v1/databases/main/users/main/rotate", ``, http.StatusConflict},
		{"disable admin refused", "POST", "/api/v1/databases/main/users/main/disable", ``, http.StatusConflict},
		{"delete admin refused", "DELETE", "/api/v1/databases/main/users/main", ``, http.StatusConflict},
		{"delete unknown is idempotent", "DELETE", "/api/v1/databases/main/users/ghost", ``, http.StatusNoContent},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rt, db, fake := newAccessTestRouter(t)
			cookie := loginTestSession(t, rt, db)
			fake.outs = []string{rolesCSV, rolesCSV}
			rec := viewerDo(t, rt, cookie, tc.method, tc.path, tc.body)
			if rec.Code != tc.want {
				t.Fatalf("status = %d, want %d, body = %s", rec.Code, tc.want, rec.Body.String())
			}
			for _, s := range fake.scripts {
				if strings.Contains(s, "DROP ROLE") || strings.Contains(s, "ALTER ROLE") {
					t.Errorf("a refused change reached the database:\n%s", s)
				}
			}
		})
	}
}

func TestDatabaseUsers_RequireRoot(t *testing.T) {
	rt, db, _ := newAccessTestRouter(t)
	const plaintext = "write-scoped-access-token" //nolint:gosec // fake fixture, not a real credential
	if err := db.SaveAPIToken(context.Background(), store.APIToken{
		ID: "tok_w", Name: "writer", TokenHash: hashToken(plaintext), Abilities: []string{AbilityWrite, AbilityWriteSensitive, AbilityReadSensitive}, CreatedAt: time.Now(),
	}); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct{ method, path, body string }{
		{"POST", "/api/v1/databases/main/users", `{"name":"someone"}`},
		{"POST", "/api/v1/databases/main/access/temp", `{}`},
		{"POST", "/api/v1/databases/main/users/x/rotate", ``},
		{"PUT", "/api/v1/databases/main/network/scope", `{"scope":"project"}`},
		{"POST", "/api/v1/databases/main/access/grants", `{}`},
	} {
		req := httptest.NewRequest(c.method, c.path, strings.NewReader(c.body))
		req.Header.Set("Authorization", "Bearer "+plaintext)
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusForbidden {
			t.Errorf("%s %s = %d, want 403 without root", c.method, c.path, rec.Code)
		}
	}
}

func TestDatabaseTemp_IssueClampsAndTracks(t *testing.T) {
	rt, db, fake := newAccessTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	fake.outs = []string{"", ""}

	rec := viewerDo(t, rt, cookie, http.MethodPost, "/api/v1/databases/main/access/temp", `{"preset":"read_write","ttl_minutes":1}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var got struct {
		Temp struct {
			ID        string `json:"id"`
			Role      string `json:"role"`
			ExpiresAt string `json:"expires_at"`
		} `json:"temp"`
		Clamped    bool `json:"clamped"`
		Credential struct {
			Password string `json:"password"`
		} `json:"credential"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if !got.Clamped || !strings.HasPrefix(got.Temp.Role, dbaccess.TempRolePrefix) {
		t.Fatalf("response = %+v", got)
	}
	script := fake.scripts[len(fake.scripts)-1]
	if !strings.Contains(script, "VALID UNTIL") || strings.Contains(script, got.Credential.Password) {
		t.Fatalf("temp script wrong:\n%s", script)
	}
	exp, err := time.Parse(store.DatabaseAccessTimeLayout, got.Temp.ExpiresAt)
	if err != nil || time.Until(exp) < 14*time.Minute {
		t.Fatalf("expiry %v err %v: a 1 minute request must be clamped up to the 15 minute floor", got.Temp.ExpiresAt, err)
	}

	list := viewerDo(t, rt, cookie, http.MethodGet, "/api/v1/databases/main/access/temp", "")
	if list.Code != http.StatusOK || strings.Contains(list.Body.String(), got.Credential.Password) || !strings.Contains(list.Body.String(), got.Temp.Role) {
		t.Fatalf("list = %d %s", list.Code, list.Body.String())
	}

	owner := viewerDo(t, rt, cookie, http.MethodPost, "/api/v1/databases/main/access/temp", `{"preset":"owner"}`)
	if owner.Code != http.StatusBadRequest {
		t.Errorf("owner temp preset status = %d, want 400", owner.Code)
	}
}

func TestDatabaseTemp_RevokeNowDropsRole(t *testing.T) {
	rt, db, fake := newAccessTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	fake.outs = []string{"", ""}
	rec := viewerDo(t, rt, cookie, http.MethodPost, "/api/v1/databases/main/access/temp", `{}`)
	var got struct {
		Temp struct {
			ID   string `json:"id"`
			Role string `json:"role"`
		} `json:"temp"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &got)

	del := viewerDo(t, rt, cookie, http.MethodDelete, "/api/v1/databases/main/access/temp/"+got.Temp.ID, "")
	if del.Code != http.StatusNoContent {
		t.Fatalf("revoke status = %d, body = %s", del.Code, del.Body.String())
	}
	last := fake.scripts[len(fake.scripts)-1]
	if !strings.Contains(last, "DROP ROLE") || !strings.Contains(last, got.Temp.Role) {
		t.Fatalf("revoke did not drop the role:\n%s", last)
	}
	live, _ := db.ListDatabaseAccessUsers(context.Background(), "main", store.DatabaseAccessKindTemp)
	if len(live) != 0 {
		t.Errorf("revoked credential still listed: %+v", live)
	}
}

func TestDatabaseTemp_SweeperRevokesAfterRestartAndAudits(t *testing.T) {
	rt, db, fake := newAccessTestRouter(t)
	ctx := context.Background()
	past := time.Now().Add(-time.Minute).UTC().Format(store.DatabaseAccessTimeLayout)
	if err := db.RecordDatabaseAccessUser(ctx, store.DatabaseAccessUser{
		ID: "t1", Database: "main", Role: "tmp_dead", Kind: store.DatabaseAccessKindTemp, Preset: "read_only", CreatedAt: past, ExpiresAt: past,
	}); err != nil {
		t.Fatal(err)
	}
	// Simulate a crash mid-revoke: the row is claimed but never completed.
	if ok, err := db.ClaimDatabaseAccessUser(ctx, "t1", time.Now().Add(-time.Hour), time.Minute); err != nil || !ok {
		t.Fatalf("seed claim: %v %v", ok, err)
	}

	sw := &dbaccess.Sweeper{Store: dbaccess.StoreAdapter{DB: db}, Revoker: rt, Auditor: rt, Logger: discardLogger()}
	if n := sw.Sweep(ctx); n != 1 {
		t.Fatalf("boot sweep revoked %d, want 1", n)
	}
	if n := sw.Sweep(ctx); n != 0 {
		t.Fatalf("second sweep revoked %d, want 0 (exactly once)", n)
	}
	if len(fake.scripts) != 1 || !strings.Contains(fake.scripts[0], `"tmp_dead"`) || !strings.Contains(fake.scripts[0], "pg_terminate_backend") {
		t.Fatalf("scripts = %v", fake.scripts)
	}
	entries, _ := db.ListAuditEntries(ctx, 20, nil, store.AuditEntryFilter{})
	var found bool
	for _, e := range entries {
		if e.Method == dbaccess.ActionTempExpire && e.ActorType == "system" && e.ActorName == dbaccess.SystemActor {
			found = true
		}
	}
	if !found {
		t.Error("expiry was not audited with the system actor")
	}
}

func TestDatabaseScope_DryRunThenConfirm(t *testing.T) {
	rt, db, _ := newAccessTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()
	for _, p := range []store.Project{{ID: "p1", Name: "shop"}, {ID: "p2", Name: "other"}} {
		if err := db.SaveProject(ctx, p); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.UpdateDatabaseProject(ctx, "main", "p1"); err != nil {
		t.Fatal(err)
	}
	seedRef := func(name, project string) {
		svc := store.DesiredService{Name: name, Image: "img:1", Port: 80, DatabaseEnv: map[string]store.DatabaseEnvRef{"DATABASE_URL": {Database: "main", Field: "url"}}}
		if err := db.SaveDesiredService(ctx, svc); err != nil {
			t.Fatal(err)
		}
		if err := db.UpdateServiceProject(ctx, name, project); err != nil {
			t.Fatal(err)
		}
	}
	seedRef("web", "p1")
	seedRef("stranger", "p2")

	dry := viewerDo(t, rt, cookie, http.MethodPut, "/api/v1/databases/main/network/scope", `{"scope":"project","dry_run":true}`)
	var body struct {
		Applied         bool `json:"applied"`
		ConfirmRequired bool `json:"confirm_required"`
		Lost            []struct {
			App struct {
				Name string `json:"name"`
			} `json:"app"`
		} `json:"lost"`
	}
	_ = json.Unmarshal(dry.Body.Bytes(), &body)
	if dry.Code != http.StatusOK || body.Applied || len(body.Lost) != 1 || body.Lost[0].App.Name != "stranger" {
		t.Fatalf("dry run = %d %s", dry.Code, dry.Body.String())
	}
	if s, _ := db.GetDatabaseAccessSettings(ctx, "main"); s.Scope != store.DatabaseNetworkScopePlatform {
		t.Fatalf("dry run changed the scope to %q", s.Scope)
	}

	noConfirm := viewerDo(t, rt, cookie, http.MethodPut, "/api/v1/databases/main/network/scope", `{"scope":"project"}`)
	_ = json.Unmarshal(noConfirm.Body.Bytes(), &body)
	if noConfirm.Code != http.StatusOK || body.Applied || !body.ConfirmRequired {
		t.Fatalf("unconfirmed apply = %d %s", noConfirm.Code, noConfirm.Body.String())
	}

	ok := viewerDo(t, rt, cookie, http.MethodPut, "/api/v1/databases/main/network/scope", `{"scope":"project","confirm":true}`)
	_ = json.Unmarshal(ok.Body.Bytes(), &body)
	if ok.Code != http.StatusOK || !body.Applied {
		t.Fatalf("confirmed apply = %d %s", ok.Code, ok.Body.String())
	}
	if s, _ := db.GetDatabaseAccessSettings(ctx, "main"); s.Scope != "project" {
		t.Fatalf("scope = %q", s.Scope)
	}

	if err := db.UpdateDatabaseProject(ctx, "main", ""); err != nil {
		t.Fatal(err)
	}
	unplaced := viewerDo(t, rt, cookie, http.MethodPut, "/api/v1/databases/main/network/scope", `{"scope":"project"}`)
	if unplaced.Code != http.StatusConflict {
		t.Errorf("unplaced scope status = %d, want 409", unplaced.Code)
	}
}

func TestDatabaseNetwork_ReachabilityVerdictPrivate(t *testing.T) {
	rt, db, _ := newAccessTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()
	svc := store.DesiredService{Name: "web", Image: "img:1", Port: 80, DatabaseEnv: map[string]store.DatabaseEnvRef{"DATABASE_URL": {Database: "main", Field: "url"}}}
	if err := db.SaveDesiredService(ctx, svc); err != nil {
		t.Fatal(err)
	}
	rec := viewerDo(t, rt, cookie, http.MethodGet, "/api/v1/databases/main/network", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var n struct {
		Verdict struct {
			Level string `json:"level"`
			Text  string `json:"text"`
		} `json:"verdict"`
		Clients []struct {
			App string `json:"app"`
		} `json:"clients"`
		Scope struct {
			Current string `json:"current"`
		} `json:"scope"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &n)
	if n.Verdict.Level != "private" || len(n.Clients) != 1 || n.Clients[0].App != "web" || n.Scope.Current != "platform" {
		t.Fatalf("network = %s", rec.Body.String())
	}
	if !strings.Contains(n.Verdict.Text, "1 app") {
		t.Errorf("verdict text = %q", n.Verdict.Text)
	}
}
