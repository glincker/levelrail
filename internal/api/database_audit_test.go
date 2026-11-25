package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/dbaudit"
	"github.com/GLINCKER/levelrail/internal/store"
)

type auditTestSettings struct{ text bool }

func (s auditTestSettings) StoreText(context.Context) bool  { return s.text }
func (auditTestSettings) QueueSize(context.Context) int     { return 1000 }
func (auditTestSettings) RetentionDays(context.Context) int { return 90 }

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
