package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/telemetry"
)

func logExplorerDo(t *testing.T, rt *Router, cookie *http.Cookie, method, target, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := authedRequest(t, cookie, method, target, body)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	return rec
}

func TestSavedLogQueries_AuthzAndValidation(t *testing.T) {
	rt, db := newTestRouter(t)
	admin := loginTestSession(t, rt, db)
	if err := db.SaveDesiredService(context.Background(), store.DesiredService{Name: "web", Image: "img:1", Port: 3000}); err != nil {
		t.Fatal(err)
	}
	reader := storeUserWithAbilitiesForTest(t, db, "reader@example.com", []string{AbilityRead})
	readerCookie := sessionCookieForTest(t, rt, reader.ID)

	body := `{"name":"errors","app":"web","spec":{"q":"timeout","level":"error","fields":["status>=500"],"range":"1h"},"visibility":"instance","pinned":true}`
	rec := logExplorerDo(t, rt, admin, http.MethodPost, "/api/v1/logs/queries", body)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create = %d %s", rec.Code, rec.Body.String())
	}
	var created savedLogQueryResource
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if !created.Pinned || created.Visibility != store.SavedLogQueryInstance || !created.Mine {
		t.Fatalf("unexpected resource %+v", created)
	}

	private := logExplorerDo(t, rt, admin, http.MethodPost, "/api/v1/logs/queries", `{"name":"mine","spec":{"q":"x"}}`)
	if private.Code != http.StatusCreated {
		t.Fatalf("private create = %d %s", private.Code, private.Body.String())
	}

	list := logExplorerDo(t, rt, readerCookie, http.MethodGet, "/api/v1/logs/queries?app=web", "")
	if list.Code != http.StatusOK {
		t.Fatalf("reader list = %d %s", list.Code, list.Body.String())
	}
	var got struct {
		Queries []savedLogQueryResource `json:"queries"`
	}
	if err := json.Unmarshal(list.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Queries) != 1 || got.Queries[0].ID != created.ID || got.Queries[0].Mine {
		t.Fatalf("reader should see only the shared query, got %+v", got.Queries)
	}

	if c := logExplorerDo(t, rt, readerCookie, http.MethodDelete, "/api/v1/logs/queries/"+created.ID, "").Code; c != http.StatusForbidden {
		t.Fatalf("non-owner delete of shared query = %d, want 403", c)
	}
	var privRes savedLogQueryResource
	_ = json.Unmarshal(private.Body.Bytes(), &privRes)
	if c := logExplorerDo(t, rt, readerCookie, http.MethodDelete, "/api/v1/logs/queries/"+privRes.ID, "").Code; c != http.StatusNotFound {
		t.Fatalf("delete of someone else's private query = %d, want 404", c)
	}
	if c := logExplorerDo(t, rt, readerCookie, http.MethodPost, "/api/v1/logs/queries", `{"name":"share","visibility":"instance","spec":{}}`).Code; c != http.StatusForbidden {
		t.Fatalf("reader sharing = %d, want 403", c)
	}

	bad := []string{
		`{"name":"","spec":{}}`,
		`{"name":"a","spec":{"level":"loud"}}`,
		`{"name":"a","spec":{"fields":["nope"]}}`,
		`{"name":"a","spec":{"range":"999d"}}`,
		`{"name":"a","pinned":true,"spec":{}}`,
		`{"name":"a","app":"ghost","spec":{}}`,
	}
	for _, b := range bad {
		if c := logExplorerDo(t, rt, admin, http.MethodPost, "/api/v1/logs/queries", b).Code; c < 400 {
			t.Errorf("body %s accepted with %d", b, c)
		}
	}

	if c := logExplorerDo(t, rt, admin, http.MethodDelete, "/api/v1/logs/queries/"+created.ID, "").Code; c != http.StatusNoContent {
		t.Fatalf("owner delete = %d", c)
	}
}

func TestLogSignatures_TwoErrorShapes(t *testing.T) {
	rt, db, tdb := newTestRouterWithTelemetry(t)
	cookie := loginTestSession(t, rt, db)
	if err := db.SaveDesiredService(context.Background(), store.DesiredService{Name: "web", Image: "img:1", Port: 3000}); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	var entries []telemetry.LogEntry
	add := func(age time.Duration, msg string) {
		entries = append(entries, telemetry.LogEntry{ResourceID: "service:web", Stream: "stderr", Timestamp: now.Add(-age), Message: msg})
	}
	for i := 0; i < 4; i++ {
		add(time.Duration(i+1)*time.Minute, "ERROR upstream 10.0.0."+string(rune('1'+i))+" timed out after 30"+string(rune('0'+i))+"ms")
	}
	add(30*time.Second, "ERROR cannot parse order 4411")
	add(20*time.Second, "ERROR cannot parse order 9921")
	add(10*time.Second, "INFO ok")
	if err := tdb.WriteLogBatch(context.Background(), entries); err != nil {
		t.Fatal(err)
	}
	window := "from=" + now.Add(-time.Hour).Format(time.RFC3339) + "&to=" + now.Add(time.Minute).Format(time.RFC3339)

	rec := logExplorerDo(t, rt, cookie, http.MethodGet, "/api/v1/apps/web/logs/signatures?"+window, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d %s", rec.Code, rec.Body.String())
	}
	var res logSignaturesResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatal(err)
	}
	if len(res.Signatures) != 2 || res.Signatures[0].Count != 4 || res.Signatures[1].Count != 2 {
		t.Fatalf("signatures = %+v", res.Signatures)
	}
	if !strings.Contains(res.Signatures[1].Template, "cannot parse order <num>") {
		t.Fatalf("template = %q", res.Signatures[1].Template)
	}

	lines := logExplorerDo(t, rt, cookie, http.MethodGet, "/api/v1/apps/web/logs?"+window+"&signature="+res.Signatures[1].ID, "")
	var lr logsResponse
	_ = json.Unmarshal(lines.Body.Bytes(), &lr)
	if lr.Total != 2 {
		t.Fatalf("lines for signature = %d, want 2", lr.Total)
	}

	if c := logExplorerDo(t, rt, cookie, http.MethodGet, "/api/v1/apps/web/logs/signatures?limit=0", "").Code; c != http.StatusBadRequest {
		t.Fatalf("limit=0 = %d", c)
	}
}
