package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
	appspec "github.com/GLINCKER/levelrail/internal/spec"
	"github.com/GLINCKER/levelrail/internal/store"
)

const extTestPassword = "Sup3r-S3cret-pw!"

type extMemSecrets struct {
	mu   sync.Mutex
	vals map[string]string
}

func (m *extMemSecrets) k(s, e string) string { return s + "/" + e }

func (m *extMemSecrets) SetValueGuarded(_ context.Context, s, e, v string, _ bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.vals == nil {
		m.vals = map[string]string{}
	}
	m.vals[m.k(s, e)] = v
	return nil
}
func (m *extMemSecrets) ListKeys(context.Context, string) ([]store.SecretKeyInfo, error) {
	return nil, nil
}
func (m *extMemSecrets) SetLocked(context.Context, string, string, bool) error { return nil }
func (m *extMemSecrets) Exists(_ context.Context, s, e string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.vals[m.k(s, e)]
	return ok, nil
}
func (m *extMemSecrets) ExistsForServices(context.Context, []string, string) (map[string]bool, error) {
	return map[string]bool{}, nil
}
func (m *extMemSecrets) Resolve(_ context.Context, s, e string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.vals[m.k(s, e)], nil
}
func (m *extMemSecrets) DeleteAll(_ context.Context, s string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for k := range m.vals {
		if strings.HasPrefix(k, s+"/") {
			delete(m.vals, k)
		}
	}
	return nil
}

type extFakeRuntime struct {
	docker.Runtime
	mu       sync.Mutex
	states   []docker.ContainerState
	created  []docker.ContainerSpec
	removed  []string
	stopped  []string
	execOut  string
	viewOut  string
	stdin    string
	execCmds [][]string
}

func (f *extFakeRuntime) ListByPrefix(context.Context, string) ([]docker.ContainerState, error) {
	return f.states, nil
}
func (f *extFakeRuntime) InspectByName(_ context.Context, name string) (*docker.ContainerState, error) {
	for i := range f.states {
		if f.states[i].Name == name {
			return &f.states[i], nil
		}
	}
	return nil, nil
}
func (f *extFakeRuntime) Create(_ context.Context, s docker.ContainerSpec) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.created = append(f.created, s)
	return "helper-" + s.Name, nil
}
func (f *extFakeRuntime) Start(context.Context, string) error { return nil }
func (f *extFakeRuntime) Remove(_ context.Context, id string, _ bool) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.removed = append(f.removed, id)
	return nil
}
func (f *extFakeRuntime) Stop(_ context.Context, id string, _ time.Duration) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.stopped = append(f.stopped, id)
	return nil
}
func (f *extFakeRuntime) Exec(_ context.Context, _ string, cmd []string) (io.ReadCloser, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.execCmds = append(f.execCmds, cmd)
	return io.NopCloser(strings.NewReader(f.execOut)), nil
}
func (f *extFakeRuntime) ExecWithInput(_ context.Context, _ string, cmd []string, in io.Reader) (io.ReadCloser, error) {
	b, _ := io.ReadAll(in)
	f.mu.Lock()
	defer f.mu.Unlock()
	f.stdin = string(b)
	f.execCmds = append(f.execCmds, cmd)
	return io.NopCloser(strings.NewReader(f.viewOut)), nil
}

func newExternalTestRouter(t *testing.T) (*Router, *store.DB, *extFakeRuntime, *extMemSecrets, *bytes.Buffer) {
	t.Helper()
	db := openTestDB(t)
	fake := &extFakeRuntime{states: []docker.ContainerState{
		{ID: "c-legacy", Name: "coolify-pg", Image: "postgres:16", Running: true, Networks: []docker.ContainerNetwork{{Name: "coolify", IPAddress: "10.0.9.2"}}},
		{ID: "c-managed", Name: "levelrail-db-main", Image: "postgres:16", Running: true, Labels: map[string]string{appspec.InstanceLabelKey: "x"}},
	}}
	sec := &extMemSecrets{}
	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	rt := NewRouter(logger, testBrand(), db, WithSecretSetter(sec), WithExecRuntime(func(string) (docker.Runtime, error) { return fake, nil }))
	return rt, db, fake, sec, &logs
}

func TestExternalDatabase_PasswordNeverLeaks(t *testing.T) {
	rt, db, fake, sec, logs := newExternalTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	body := `{"name":"legacy","engine":"postgres","host":"coolify-pg","network":"coolify","username":"app","password":"` + extTestPassword + `","database":"appdb"}`
	rec := viewerDo(t, rt, cookie, http.MethodPost, "/api/v1/external-databases", body)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create = %d %s", rec.Code, rec.Body.String())
	}
	all := rec.Body.String()
	for _, p := range []string{"/api/v1/external-databases", "/api/v1/external-databases/legacy", "/api/v1/databases", "/api/v1/databases/legacy"} {
		r := viewerDo(t, rt, cookie, http.MethodGet, p, "")
		if r.Code != http.StatusOK {
			t.Fatalf("GET %s = %d %s", p, r.Code, r.Body.String())
		}
		all += r.Body.String()
	}
	pr := viewerDo(t, rt, cookie, http.MethodPost, "/api/v1/external-databases/legacy/probe", "")
	all += pr.Body.String()
	tr := viewerDo(t, rt, cookie, http.MethodPost, "/api/v1/external-databases/test", body)
	all += tr.Body.String()
	if strings.Contains(all, extTestPassword) {
		t.Fatalf("password appears in an API response: %s", all)
	}
	if !strings.Contains(rec.Body.String(), `"has_password":true`) {
		t.Errorf("create response should say a password is stored: %s", rec.Body.String())
	}
	if got := sec.vals["external-database/legacy/password"]; got != extTestPassword {
		t.Errorf("stored secret = %q", got)
	}
	if strings.Contains(logs.String(), extTestPassword) {
		t.Fatalf("password appears in logs:\n%s", logs.String())
	}
	for _, c := range fake.execCmds {
		if strings.Contains(strings.Join(c, " "), extTestPassword) {
			t.Fatalf("password in argv: %v", c)
		}
	}
	entries, err := db.ListAuditEntries(t.Context(), 100, nil, store.AuditEntryFilter{})
	if err == nil {
		for _, e := range entries {
			if strings.Contains(e.Path, extTestPassword) {
				t.Fatalf("password in audit path %q", e.Path)
			}
		}
	}

	rev := viewerDo(t, rt, cookie, http.MethodGet, "/api/v1/external-databases/legacy/password", "")
	if rev.Code != http.StatusOK || !strings.Contains(rev.Body.String(), extTestPassword) {
		t.Fatalf("reveal = %d %s", rev.Code, rev.Body.String())
	}
	if rev.Header().Get("Cache-Control") != "no-store" {
		t.Error("reveal must not be cacheable")
	}
	if strings.Contains(logs.String(), extTestPassword) {
		t.Fatal("reveal logged the password")
	}
}

func TestExternalDatabase_AddressPolicy(t *testing.T) {
	rt, db, _, _, _ := newExternalTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	for _, host := range []string{"169.254.169.254", "127.0.0.1", "localhost", "fe80::1", "metadata.google.internal", "0.0.0.0", "bad host"} {
		body, _ := json.Marshal(map[string]any{"name": "x", "engine": "postgres", "host": host})
		rec := viewerDo(t, rt, cookie, http.MethodPost, "/api/v1/external-databases", string(body))
		if rec.Code != http.StatusBadRequest {
			t.Errorf("host %q = %d, want 400", host, rec.Code)
		}
	}
	t.Setenv("APP_EXTERNAL_DB_ALLOW_LINK_LOCAL", "true")
	rec := viewerDo(t, rt, cookie, http.MethodPost, "/api/v1/external-databases", `{"name":"meta","engine":"postgres","host":"169.254.169.254"}`)
	if rec.Code != http.StatusCreated {
		t.Errorf("opt in = %d %s", rec.Code, rec.Body.String())
	}
}

func TestExternalDatabase_NameClashAndValidation(t *testing.T) {
	rt, db, _, _, _ := newExternalTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	if err := db.SaveDesiredDatabase(t.Context(), store.DesiredDatabase{Name: "main", Engine: "postgres", Version: "16"}); err != nil {
		t.Fatal(err)
	}
	rec := viewerDo(t, rt, cookie, http.MethodPost, "/api/v1/external-databases", `{"name":"main","engine":"postgres","host":"pg"}`)
	if rec.Code != http.StatusConflict {
		t.Errorf("managed name clash = %d", rec.Code)
	}
	rec = viewerDo(t, rt, cookie, http.MethodPost, "/api/v1/external-databases", `{"name":"Bad Name","engine":"postgres","host":"pg"}`)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("bad name = %d", rec.Code)
	}
	rec = viewerDo(t, rt, cookie, http.MethodPost, "/api/v1/external-databases", `{"name":"ora","engine":"oracle","host":"pg"}`)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("bad engine = %d", rec.Code)
	}
}

func TestExternalDatabase_TestEndpointStatuses(t *testing.T) {
	rt, db, fake, _, _ := newExternalTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	body := `{"engine":"postgres","host":"pg.internal","username":"u","password":"x"}`
	cases := map[string]string{
		`psql: error: connection to server failed: FATAL:  password authentication failed for user "u"` + "\nLR_EXIT:2\n": "auth_failed",
		`psql: error: connection to server failed: Connection refused` + "\nLR_EXIT:2\n":                                  "unreachable",
		"u|appdb|PostgreSQL 16|appdb,other\nLR_EXIT:0\n":                                                                  "reachable",
	}
	for out, want := range cases {
		fake.execOut = out
		rec := viewerDo(t, rt, cookie, http.MethodPost, "/api/v1/external-databases/test", body)
		if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"status":"`+want+`"`) {
			t.Errorf("out %q: %d %s, want %s", out, rec.Code, rec.Body.String(), want)
		}
	}
	if len(fake.created) == 0 || len(fake.removed) != len(fake.created) {
		t.Errorf("helpers created %d removed %d", len(fake.created), len(fake.removed))
	}
}

func TestExternalDatabase_AdoptNeverTouchesContainer(t *testing.T) {
	rt, db, fake, _, _ := newExternalTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	rec := viewerDo(t, rt, cookie, http.MethodGet, "/api/v1/external-databases/candidates", "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "coolify-pg") || strings.Contains(rec.Body.String(), "levelrail-db-main") {
		t.Fatalf("candidates = %d %s", rec.Code, rec.Body.String())
	}
	rec = viewerDo(t, rt, cookie, http.MethodPost, "/api/v1/external-databases/adopt", `{"container":"coolify-pg","name":"legacy","password":"`+extTestPassword+`","database":"appdb"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("adopt = %d %s", rec.Code, rec.Body.String())
	}
	var got externalDatabaseResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Host != "coolify-pg" || got.Network != "coolify" || got.Username != "postgres" || got.SourceContainer != "coolify-pg" || got.Engine != "postgres" {
		t.Fatalf("adopted = %+v", got)
	}
	rec = viewerDo(t, rt, cookie, http.MethodPost, "/api/v1/external-databases/adopt", `{"container":"levelrail-db-main"}`)
	if rec.Code != http.StatusNotFound {
		t.Errorf("platform-managed container adoptable: %d", rec.Code)
	}

	// Delete must not touch the container or any remote database either.
	del := viewerDo(t, rt, cookie, http.MethodDelete, "/api/v1/external-databases/legacy", "")
	if del.Code != http.StatusNoContent {
		t.Fatalf("delete = %d %s", del.Code, del.Body.String())
	}
	fake.mu.Lock()
	defer fake.mu.Unlock()
	for _, id := range append(append([]string{}, fake.removed...), fake.stopped...) {
		if strings.Contains(id, "c-legacy") || id == "coolify-pg" {
			t.Fatalf("source container was stopped or removed: %s", id)
		}
	}
	if len(fake.stopped) != 0 {
		t.Errorf("stop called: %v", fake.stopped)
	}
	for _, spec := range fake.created {
		if !strings.HasPrefix(spec.Name, "extdb-") {
			t.Errorf("created a non-helper container %q", spec.Name)
		}
	}
}

func TestExternalDatabase_ViewerAndWriteGuard(t *testing.T) {
	rt, db, fake, _, _ := newExternalTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	rec := viewerDo(t, rt, cookie, http.MethodPost, "/api/v1/external-databases", `{"name":"legacy","engine":"postgres","host":"coolify-pg","network":"coolify","username":"app","password":"`+extTestPassword+`","database":"appdb"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create = %d %s", rec.Code, rec.Body.String())
	}
	fake.viewOut = "id,name\n1,ada\n"
	q := viewerDo(t, rt, cookie, http.MethodPost, "/api/v1/databases/legacy/query", `{"sql":"select id, name from users"}`)
	if q.Code != http.StatusOK || !strings.Contains(q.Body.String(), "ada") {
		t.Fatalf("query = %d %s", q.Code, q.Body.String())
	}
	if !strings.Contains(fake.stdin, "BEGIN READ ONLY;") {
		t.Errorf("not read-only:\n%s", fake.stdin)
	}
	last := fake.created[len(fake.created)-1]
	if last.Env["PGHOST"] != "coolify-pg" || last.Network == nil || last.Network.Name != "coolify" {
		t.Errorf("helper spec = %+v", last)
	}
	for _, c := range fake.execCmds {
		if strings.Contains(strings.Join(c, " "), extTestPassword) {
			t.Fatal("password in argv")
		}
	}
	calls := len(fake.execCmds)
	w := viewerDo(t, rt, cookie, http.MethodPost, "/api/v1/databases/legacy/query/write", `{"sql":"delete from users"}`)
	if w.Code != http.StatusBadRequest || len(fake.execCmds) != calls {
		t.Errorf("write without confirm = %d, exec calls %d -> %d", w.Code, calls, len(fake.execCmds))
	}
	ro := viewerDo(t, rt, cookie, http.MethodPost, "/api/v1/databases/legacy/query", `{"sql":"delete from users"}`)
	if ro.Code != http.StatusBadRequest || len(fake.execCmds) != calls {
		t.Errorf("write via read route = %d", ro.Code)
	}
}

func TestExternalDatabase_AppConnectionResolves(t *testing.T) {
	rt, db, _, _, _ := newExternalTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	viewerDo(t, rt, cookie, http.MethodPost, "/api/v1/external-databases", `{"name":"legacy","engine":"postgres","host":"coolify-pg","network":"coolify","username":"app","database":"appdb"}`)
	if err := db.SaveDesiredService(t.Context(), store.DesiredService{Name: "web", Image: "img:1", Port: 80}); err != nil {
		t.Fatal(err)
	}
	rec := viewerDo(t, rt, cookie, http.MethodPost, "/api/v1/apps/web/connections", `{"database":"legacy","field":"url","env_var":"DATABASE_URL"}`)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"host":"coolify-pg"`) {
		t.Fatalf("connection = %d %s", rec.Code, rec.Body.String())
	}
	bad := viewerDo(t, rt, cookie, http.MethodPost, "/api/v1/apps/web/connections", `{"database":"legacy","field":"nope"}`)
	if bad.Code != http.StatusBadRequest {
		t.Errorf("bad field = %d", bad.Code)
	}
	del := viewerDo(t, rt, cookie, http.MethodDelete, "/api/v1/external-databases/legacy", "")
	if del.Code != http.StatusConflict {
		t.Errorf("delete in use = %d, want 409", del.Code)
	}
}
