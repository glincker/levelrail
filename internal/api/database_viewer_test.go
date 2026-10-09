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

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/store"
)

// fakeViewerRuntime records the script and command a console statement
// sent to the engine client, and replies with a canned stdout.
type fakeViewerRuntime struct {
	*fakeExecAppRuntime
	stdin string
	cmd   []string
	out   string
	fail  *docker.ExecExitError
	calls int
}

type failingReader struct {
	r   io.Reader
	err error
}

func (f *failingReader) Read(p []byte) (int, error) {
	n, err := f.r.Read(p)
	if err == io.EOF {
		return n, f.err
	}
	return n, err
}
func (f *failingReader) Close() error { return nil }

func (f *fakeViewerRuntime) ExecWithInput(_ context.Context, _ string, cmd []string, stdin io.Reader) (io.ReadCloser, error) {
	f.calls++
	f.cmd = cmd
	b, _ := io.ReadAll(stdin)
	f.stdin = string(b)
	if f.fail != nil {
		return &failingReader{r: strings.NewReader(""), err: f.fail}, nil
	}
	return io.NopCloser(strings.NewReader(f.out)), nil
}

func newViewerTestRouter(t *testing.T, engine string) (*Router, *store.DB, *fakeViewerRuntime) {
	t.Helper()
	db := openTestDB(t)
	fake := &fakeViewerRuntime{fakeExecAppRuntime: &fakeExecAppRuntime{inspectState: &docker.ContainerState{ID: "c1", Running: true}}}
	rt := NewRouter(discardLogger(), testBrand(), db, WithExecRuntime(func(string) (docker.Runtime, error) { return fake, nil }))
	if err := db.SaveDesiredDatabase(context.Background(), store.DesiredDatabase{Name: "main", Engine: engine, Version: "16"}); err != nil {
		t.Fatalf("seed database: %v", err)
	}
	return rt, db, fake
}

func viewerDo(t *testing.T, rt *Router, cookie *http.Cookie, method, target, body string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, method, target, body))
	return rec
}

func TestDatabaseQuery_ReadOnlyScriptAndResult(t *testing.T) {
	rt, db, fake := newViewerTestRouter(t, store.EnginePostgres)
	cookie := loginTestSession(t, rt, db)
	fake.out = "id,name\n1,ada\n2,bob\n"

	rec := viewerDo(t, rt, cookie, http.MethodPost, "/api/v1/databases/main/query", `{"sql":"select id, name from users"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var got struct {
		Columns     []string    `json:"columns"`
		Rows        [][]*string `json:"rows"`
		Mode        string      `json:"mode"`
		Fingerprint string      `json:"fingerprint"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Columns) != 2 || len(got.Rows) != 2 || *got.Rows[1][1] != "bob" || got.Mode != "read" || len(got.Fingerprint) != 16 {
		t.Errorf("response = %+v", got)
	}
	for _, want := range []string{"BEGIN READ ONLY;", "DECLARE lr_cursor NO SCROLL CURSOR FOR", "ROLLBACK;"} {
		if !strings.Contains(fake.stdin, want) {
			t.Errorf("script missing %q:\n%s", want, fake.stdin)
		}
	}
	if strings.Contains(strings.Join(fake.cmd, " "), "users") {
		t.Error("SQL must travel on stdin, not argv")
	}
	if !strings.Contains(strings.Join(fake.cmd, " "), "default_transaction_read_only=on") {
		t.Errorf("cmd = %v, want read-only PGOPTIONS", fake.cmd)
	}
}

func TestDatabaseQuery_GuardRejectsWithoutExec(t *testing.T) {
	cases := []struct{ name, sql string }{
		{"multi statement", "select 1; drop table users"},
		{"insert", "insert into users values (1)"},
		{"copy program", "copy users to program 'id'"},
		{"select into", "select * into backup from users"},
		{"data modifying cte", "with d as (delete from users returning *) select * from d"},
		{"side effect function", "select pg_terminate_backend(pid) from pg_stat_activity"},
		{"meta command", "select 1 \\g"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rt, db, fake := newViewerTestRouter(t, store.EnginePostgres)
			cookie := loginTestSession(t, rt, db)
			body, _ := json.Marshal(map[string]string{"sql": tc.sql})
			rec := viewerDo(t, rt, cookie, http.MethodPost, "/api/v1/databases/main/query", string(body))
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400, body = %s", rec.Code, rec.Body.String())
			}
			if fake.calls != 0 {
				t.Errorf("guard rejection must not reach the database, exec calls = %d", fake.calls)
			}
		})
	}
}

func TestDatabaseQuery_DatabaseErrorSurfacedAndRecorded(t *testing.T) {
	rt, db, fake := newViewerTestRouter(t, store.EnginePostgres)
	cookie := loginTestSession(t, rt, db)
	fake.fail = &docker.ExecExitError{ExitCode: 3, Stderr: "psql:<stdin>:3: ERROR:  relation \"nope\" does not exist\nLINE 2: select * from nope\n"}

	rec := viewerDo(t, rt, cookie, http.MethodPost, "/api/v1/databases/main/query", `{"sql":"select * from nope"}`)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), `relation \"nope\" does not exist`) {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	hist := viewerDo(t, rt, cookie, http.MethodGet, "/api/v1/databases/main/query-history", "")
	var rows []queryHistoryResource
	if err := json.Unmarshal(hist.Body.Bytes(), &rows); err != nil || len(rows) != 1 || rows[0].OK {
		t.Fatalf("history = %s err = %v", hist.Body.String(), err)
	}
}

func TestDatabaseQueryWrite_RequiresConfirmAndAudits(t *testing.T) {
	rt, db, fake := newViewerTestRouter(t, store.EnginePostgres)
	cookie := loginTestSession(t, rt, db)

	rec := viewerDo(t, rt, cookie, http.MethodPost, "/api/v1/databases/main/query/write", `{"sql":"delete from t"}`)
	if rec.Code != http.StatusBadRequest || fake.calls != 0 {
		t.Fatalf("missing confirm: status = %d calls = %d", rec.Code, fake.calls)
	}

	rec = viewerDo(t, rt, cookie, http.MethodPost, "/api/v1/databases/main/query/write", `{"sql":"delete from t where id = 1","confirm":"main"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(fake.stdin, "BEGIN;\ndelete from t where id = 1") || !strings.Contains(fake.stdin, "COMMIT;") {
		t.Errorf("script = %s", fake.stdin)
	}
	if !strings.Contains(strings.Join(fake.cmd, " "), "default_transaction_read_only=off") {
		t.Errorf("write mode must lift the read-only default: %v", fake.cmd)
	}

	entries, err := db.ListAuditEntries(context.Background(), 50, nil, store.AuditEntryFilter{})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, e := range entries {
		if strings.HasPrefix(e.Path, "/api/v1/databases/main/query/write#") && e.Ability == AbilityRoot {
			found = true
		}
	}
	if !found {
		t.Errorf("no fingerprint audit entry among %d entries", len(entries))
	}
}

func TestDatabaseQueryWrite_NonAdminTokenForbidden(t *testing.T) {
	rt, db, fake := newViewerTestRouter(t, store.EnginePostgres)
	const plaintext = "viewer-token" //nolint:gosec // fake fixture
	if err := db.SaveAPIToken(context.Background(), store.APIToken{
		ID: "tok_viewer", Name: "viewer", TokenHash: hashToken(plaintext), Abilities: []string{AbilityRead, AbilityReadSensitive}, CreatedAt: time.Now(),
	}); err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, bearerRequest(http.MethodPost, "/api/v1/databases/main/query/write", `{"sql":"delete from t","confirm":"main"}`, plaintext))
	if rec.Code != http.StatusForbidden || fake.calls != 0 {
		t.Fatalf("status = %d calls = %d, want 403 and no exec", rec.Code, fake.calls)
	}

	fake.out = "?column?\n1\n"
	rec = httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, bearerRequest(http.MethodPost, "/api/v1/databases/main/query", `{"sql":"select 1"}`, plaintext))
	if rec.Code != http.StatusOK {
		t.Fatalf("read:sensitive token read query status = %d, body = %s", rec.Code, rec.Body.String())
	}
}

func TestDatabaseQuery_ReadOnlyTokenForbidden(t *testing.T) {
	rt, db, _ := newViewerTestRouter(t, store.EnginePostgres)
	const plaintext = "ro-token" //nolint:gosec // fake fixture
	if err := db.SaveAPIToken(context.Background(), store.APIToken{
		ID: "tok_ro", Name: "ro", TokenHash: hashToken(plaintext), Abilities: []string{AbilityRead}, CreatedAt: time.Now(),
	}); err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, bearerRequest(http.MethodGet, "/api/v1/databases/main/schema", "", plaintext))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403: plain read must not browse row data or schema", rec.Code)
	}
}

func TestDatabaseSchema_Postgres(t *testing.T) {
	rt, db, fake := newViewerTestRouter(t, store.EnginePostgres)
	cookie := loginTestSession(t, rt, db)
	cellJSON := `[{"schema":"public","name":"users","kind":"table","row_estimate":42,"size_bytes":8192,` +
		`"columns":[{"name":"id","type":"integer","nullable":false,"default":"","primary_key":true}],` +
		`"indexes":[{"name":"users_pkey","definition":"CREATE UNIQUE INDEX","unique":true,"primary":true}]}]`
	fake.out = "json_agg\n\"" + strings.ReplaceAll(cellJSON, `"`, `""`) + "\"\n"

	rec := viewerDo(t, rt, cookie, http.MethodGet, "/api/v1/databases/main/schema", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var got databaseSchemaResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Engine != "postgres" || len(got.Schemas) != 1 || got.Schemas[0].Tables[0].RowEstimate != 42 || !got.Schemas[0].Tables[0].Columns[0].PrimaryKey {
		t.Errorf("schema = %+v", got)
	}
}

func TestDatabaseTableRows_RejectsUnknownSortColumn(t *testing.T) {
	rt, db, fake := newViewerTestRouter(t, store.EnginePostgres)
	cookie := loginTestSession(t, rt, db)
	fake.out = "attname\nid\nname\n"

	rec := viewerDo(t, rt, cookie, http.MethodGet, "/api/v1/databases/main/tables/public/users/rows?sort=id%3B+drop+table+users", "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if fake.calls != 1 {
		t.Errorf("only the catalog lookup should have run, calls = %d", fake.calls)
	}
}

func TestDatabaseViewer_EngineGating(t *testing.T) {
	rt, db, fake := newViewerTestRouter(t, store.EngineRedis)
	cookie := loginTestSession(t, rt, db)

	rec := viewerDo(t, rt, cookie, http.MethodPost, "/api/v1/databases/main/query", `{"sql":"select 1"}`)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("redis query status = %d, want 400", rec.Code)
	}
	fake.out = "0\nuser:1\n"
	rec = viewerDo(t, rt, cookie, http.MethodGet, "/api/v1/databases/main/keys", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("redis keys status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(strings.Join(fake.cmd, " "), "redis-cli") {
		t.Errorf("cmd = %v", fake.cmd)
	}

	rt2, db2, _ := newViewerTestRouter(t, store.EnginePostgres)
	cookie2 := loginTestSession(t, rt2, db2)
	rec = viewerDo(t, rt2, cookie2, http.MethodGet, "/api/v1/databases/main/keys", "")
	if rec.Code != http.StatusBadRequest {
		t.Errorf("postgres keys status = %d, want 400", rec.Code)
	}
}

func TestDatabaseSavedQueries_RoundTrip(t *testing.T) {
	rt, db, _ := newViewerTestRouter(t, store.EnginePostgres)
	cookie := loginTestSession(t, rt, db)

	rec := viewerDo(t, rt, cookie, http.MethodPost, "/api/v1/databases/main/saved-queries", `{"name":"recent users","sql":"select * from users order by id desc"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var created savedQueryResource
	_ = json.Unmarshal(rec.Body.Bytes(), &created)

	rec = viewerDo(t, rt, cookie, http.MethodPost, "/api/v1/databases/main/saved-queries", `{"name":"recent users","sql":"select 1"}`)
	if rec.Code != http.StatusConflict {
		t.Errorf("duplicate status = %d, want 409", rec.Code)
	}
	rec = viewerDo(t, rt, cookie, http.MethodGet, "/api/v1/databases/main/saved-queries", "")
	var list []savedQueryResource
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil || len(list) != 1 {
		t.Fatalf("list = %s err = %v", rec.Body.String(), err)
	}
	rec = viewerDo(t, rt, cookie, http.MethodDelete, "/api/v1/databases/main/saved-queries/"+created.ID, "")
	if rec.Code != http.StatusNoContent {
		t.Errorf("delete status = %d", rec.Code)
	}
}
