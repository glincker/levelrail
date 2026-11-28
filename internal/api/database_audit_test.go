package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/glincker/levelrail/internal/dbaudit"
	"github.com/glincker/levelrail/internal/store"
)

type auditTestSettings struct {
	text  bool
	queue int
}

func (s auditTestSettings) StoreText(context.Context) bool { return s.text }
func (s auditTestSettings) QueueSize(context.Context) int {
	if s.queue > 0 {
		return s.queue
	}
	return 1000
}
func (auditTestSettings) RetentionDays(context.Context) int { return 90 }
func (auditTestSettings) MaxRows(context.Context) int       { return 1000000 }
func (auditTestSettings) TextMaxBytes(context.Context) int  { return 4096 }

const hostileValue = "HOSTILE-VALUE-42"

func auditedRouter(t *testing.T, text bool) (*Router, *store.DB, *fakeViewerRuntime, *dbaudit.Writer) {
	t.Helper()
	rt, db, fake := newViewerTestRouter(t, store.EnginePostgres)
	w := dbaudit.New(db, auditTestSettings{text: text}, dbaudit.Options{FlushInterval: time.Hour})
	rt.WithDatabaseAudit(w)
	return rt, db, fake, w
}

func TestDatabaseAuditEveryExplorerRouteRecords(t *testing.T) {
	rt, db, fake, w := auditedRouter(t, false)
	fake.out = "?column?\n1\n"
	cookie := loginTestSession(t, rt, db)
	// Branch routes record the resolved root, so the branch the route names must exist.
	now := time.Now().UTC().Format(time.RFC3339)
	if err := db.SaveDesiredDatabase(context.Background(), store.DesiredDatabase{Name: "x", Engine: store.EnginePostgres, Version: "16"}); err != nil {
		t.Fatal(err)
	}
	if err := db.CreateDatabaseBranch(context.Background(), store.DatabaseBranch{ID: "bch_audit_x", DatabaseName: "x", ParentName: "main", RootName: "main", Mode: "schema", Source: "live", State: "ready", CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	routes := explorerRoutes("main")
	for _, rr := range routes {
		target := rr.path
		body := ""
		switch {
		case strings.HasSuffix(target, "/rows"):
			target += "?filter=" + hostileValue
		case rr.method == http.MethodPost && strings.Contains(target, "/query"):
			body = `{"sql":"select '` + hostileValue + `'","confirm":"main"}`
		}
		viewerDo(t, rt, cookie, rr.method, target, body)
	}
	w.Flush(context.Background())
	rows, _, err := db.ListDatabaseStatementAudits(context.Background(), store.DatabaseStatementAuditFilter{DatabaseName: "main", Limit: 500})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != len(routes) {
		t.Fatalf("audit rows %d, routes %d", len(rows), len(routes))
	}
	for _, r := range rows {
		if r.Outcome == "" || r.Action == "" || r.OpKind == "" || r.Fingerprint == "" || r.PrincipalID == "" || r.Engine != store.EnginePostgres {
			t.Errorf("incomplete row: %+v", r)
		}
	}
	raw, _ := json.Marshal(rows)
	if strings.Contains(string(raw), hostileValue) {
		t.Fatalf("raw value leaked into the audit: %s", raw)
	}
}

func TestDatabaseAuditStatementTextOnlyWhenSettingOn(t *testing.T) {
	for _, on := range []bool{false, true} {
		rt, db, fake, w := auditedRouter(t, on)
		fake.out = "?column?\n1\n"
		cookie := loginTestSession(t, rt, db)
		viewerDo(t, rt, cookie, http.MethodPost, "/api/v1/databases/main/query", `{"sql":"select 1"}`)
		w.Flush(context.Background())
		rows, _, _ := db.ListDatabaseStatementAudits(context.Background(), store.DatabaseStatementAuditFilter{DatabaseName: "main"})
		if len(rows) != 1 || rows[0].Fingerprint == "" || rows[0].Outcome != "ok" || rows[0].RowsReturned != 1 {
			t.Fatalf("rows %+v", rows)
		}
		if on != (rows[0].StatementText != "") {
			t.Fatalf("setting %v stored text %q", on, rows[0].StatementText)
		}
	}
}

func TestDatabaseAuditReadRouteAuthz(t *testing.T) {
	rt, db, _, w := auditedRouter(t, false)
	for id, abil := range map[string][]string{"viewer": {AbilityRead}, "sensitive": {AbilityRead, AbilityReadSensitive}} {
		if err := db.SaveAPIToken(context.Background(), store.APIToken{ID: "tok_" + id, Name: id, TokenHash: hashToken("secret-" + id), Abilities: abil, CreatedAt: time.Now()}); err != nil {
			t.Fatal(err)
		}
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, bearerRequest(http.MethodGet, "/api/v1/databases/main/audit", "", "secret-"+id))
		if rec.Code != http.StatusForbidden {
			t.Fatalf("%s token: status %d, want 403", id, rec.Code)
		}
	}
	w.Flush(context.Background())
	rows, _, _ := db.ListDatabaseStatementAudits(context.Background(), store.DatabaseStatementAuditFilter{DatabaseName: "main", Outcome: "denied"})
	if len(rows) != 2 {
		t.Fatalf("denied rows %d, want 2", len(rows))
	}
	cookie := loginTestSession(t, rt, db)
	rec := viewerDo(t, rt, cookie, http.MethodGet, "/api/v1/databases/main/audit?outcome=denied&limit=1", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("root status %d: %s", rec.Code, rec.Body.String())
	}
	var out databaseAuditListResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil || len(out.Items) != 1 || out.NextCursor == "" {
		t.Fatalf("page %+v %v", out, err)
	}
	if rec := viewerDo(t, rt, cookie, http.MethodGet, "/api/v1/databases/main/audit?since=nope", ""); rec.Code != http.StatusBadRequest {
		t.Fatalf("bad since: %d", rec.Code)
	}
	if rec := viewerDo(t, rt, cookie, http.MethodGet, "/api/v1/databases/main/audit?cursor=nope", ""); rec.Code != http.StatusBadRequest {
		t.Fatalf("bad cursor: %d", rec.Code)
	}
}

func TestDatabaseAuditFingerprintIdentifiesNormalizedStatement(t *testing.T) {
	rt, db, fake, w := auditedRouter(t, false)
	fake.out = "?column?\n1\n"
	cookie := loginTestSession(t, rt, db)
	for _, sql := range []string{"select * from t where id = 1", "select * from t where id = 2", "select * from u where id = 1"} {
		viewerDo(t, rt, cookie, http.MethodPost, "/api/v1/databases/main/query", `{"sql":"`+sql+`"}`)
	}
	w.Flush(context.Background())
	rows, _, _ := db.ListDatabaseStatementAudits(context.Background(), store.DatabaseStatementAuditFilter{DatabaseName: "main"})
	fp := map[string]string{}
	for _, r := range rows {
		fp[r.Fingerprint] = r.ID
	}
	if len(rows) != 3 || len(fp) != 2 {
		t.Fatalf("want 3 rows and 2 distinct fingerprints (same shape, different literal collapses), got %d rows %d fingerprints", len(rows), len(fp))
	}
}

func TestDatabaseAuditUnauthenticatedWritesNoRow(t *testing.T) {
	rt, db, _, w := auditedRouter(t, false)
	name := strings.Repeat("z", 3000)
	for _, target := range []string{"/api/v1/databases/main/query", "/api/v1/databases/" + name + "/query"} {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, target, strings.NewReader(`{"sql":"select 1"}`)))
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("status %d, want 401", rec.Code)
		}
	}
	w.Flush(context.Background())
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM database_statement_audit`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("rows %d %v, want none", n, err)
	}
	if w.Unauthenticated() != 2 {
		t.Fatalf("counted %d, want 2", w.Unauthenticated())
	}
}

func TestDatabaseAuditFullQueueRefusesWritesNotReads(t *testing.T) {
	rt, db, fake, _ := newViewerTestRouterWithAudit(t, 1)
	fake.out = "?column?\n1\n"
	cookie := loginTestSession(t, rt, db)
	if rec := viewerDo(t, rt, cookie, http.MethodGet, "/api/v1/databases/main/schema", ""); rec.Code == http.StatusServiceUnavailable {
		t.Fatalf("first read refused")
	}
	if rec := viewerDo(t, rt, cookie, http.MethodGet, "/api/v1/databases/main/schema", ""); rec.Code == http.StatusServiceUnavailable {
		t.Fatalf("a read must fail open when the queue is full")
	}
	rec := viewerDo(t, rt, cookie, http.MethodPost, "/api/v1/databases/main/query/write", `{"sql":"select 1","confirm":"main"}`)
	if rec.Code != http.StatusServiceUnavailable || !strings.Contains(rec.Body.String(), "audit_unavailable") {
		t.Fatalf("write status %d body %s, want 503 audit_unavailable", rec.Code, rec.Body.String())
	}
}

func newViewerTestRouterWithAudit(t *testing.T, queue int) (*Router, *store.DB, *fakeViewerRuntime, *dbaudit.Writer) {
	t.Helper()
	rt, db, fake := newViewerTestRouter(t, store.EnginePostgres)
	w := dbaudit.New(db, auditTestSettings{queue: queue}, dbaudit.Options{FlushInterval: time.Hour})
	rt.WithDatabaseAudit(w)
	return rt, db, fake, w
}

func TestDatabaseAuditReusedNameCannotReadOldTrail(t *testing.T) {
	rt, db, _, _ := auditedRouter(t, false)
	desired, err := db.GetDesiredDatabase(context.Background(), "main")
	if err != nil || desired == nil {
		t.Fatalf("desired %v %v", desired, err)
	}
	at := time.Now().UTC().Format(store.StatementAuditTimeLayout)
	old := store.DatabaseStatementAudit{ID: "old1", At: at, DatabaseName: "main", DatabaseID: "previous-incarnation", Action: "db:read", Outcome: "ok"}
	cur := store.DatabaseStatementAudit{ID: "cur1", At: at, DatabaseName: "main", DatabaseID: desired.InstanceID, Action: "db:read", Outcome: "ok"}
	if err := db.InsertDatabaseStatementAudits(context.Background(), []store.DatabaseStatementAudit{old, cur}); err != nil {
		t.Fatal(err)
	}
	cookie := loginTestSession(t, rt, db)
	rec := viewerDo(t, rt, cookie, http.MethodGet, "/api/v1/databases/main/audit", "")
	var out databaseAuditListResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	for _, r := range out.Items {
		if r.ID == "old1" {
			t.Fatalf("old trail readable: %+v", out.Items)
		}
	}
}
