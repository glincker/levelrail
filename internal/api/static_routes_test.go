package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/GLINCKER/levelrail/internal/staticroutes"
)

func TestStaticRoutesEndpoints(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	root := t.TempDir()
	seedStaticSite(t, db, root, "docs", "d.example.com")
	seedApp(t, db, "docs")
	dir := filepath.Join(root, "docs", "abc")

	do := func(method, path, body string) (int, []byte) {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, method, path, body))
		return rec.Code, rec.Body.Bytes()
	}

	code, raw := do(http.MethodGet, "/api/v1/apps/docs/static-routes", "")
	var before staticRoutesResponse
	if code != http.StatusOK || json.Unmarshal(raw, &before) != nil || before.Published || len(before.Rules) != 0 {
		t.Fatalf("a site with no sidecar: %d %s", code, raw)
	}

	rs, err := staticroutes.Compile(staticroutes.Input{Redirects: "/old /index.html 301\n", Headers: "/*\n  X-A: b\n"}, staticroutes.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	if err := staticroutes.WriteSidecar(dir, rs); err != nil {
		t.Fatal(err)
	}

	code, raw = do(http.MethodGet, "/api/v1/apps/docs/static-routes", "")
	var got staticRoutesResponse
	if code != http.StatusOK || json.Unmarshal(raw, &got) != nil || !got.Published || len(got.Rules) != 1 || len(got.Headers) != 1 {
		t.Fatalf("effective rules: %d %s", code, raw)
	}

	code, raw = do(http.MethodPost, "/api/v1/apps/docs/static-routes/test", `{"path":"/old"}`)
	var d staticroutes.Decision
	if code != http.StatusOK || json.Unmarshal(raw, &d) != nil || d.Kind != staticroutes.KindRedirect || d.Location != "/index.html" || d.Rule != "_redirects:1" {
		t.Fatalf("test redirect: %d %s", code, raw)
	}
	code, raw = do(http.MethodPost, "/api/v1/apps/docs/static-routes/test", `{"path":"/"}`)
	if code != http.StatusOK || json.Unmarshal(raw, &d) != nil || d.Kind != staticroutes.KindServe || d.File != "index.html" || len(d.Headers) != 1 {
		t.Fatalf("test file: %d %s", code, raw)
	}
	if code, _ = do(http.MethodPost, "/api/v1/apps/docs/static-routes/test", `{"path":"nope"}`); code != http.StatusBadRequest {
		t.Fatalf("relative path = %d, want 400", code)
	}
	if code, _ = do(http.MethodGet, "/api/v1/apps/ghost/static-routes", ""); code != http.StatusNotFound {
		t.Fatalf("unknown site = %d, want 404", code)
	}

	code, raw = do(http.MethodPost, "/api/v1/apps/docs/static-routes/lint", `{"redirects":"/a /b\nbad\n/c //evil.example.com","headers":"/*\n  Host: x\n"}`)
	var lint staticRoutesLintResponse
	if code != http.StatusOK || json.Unmarshal(raw, &lint) != nil || len(lint.Rules) != 1 || len(lint.Problems) != 3 || lint.Error != "" {
		t.Fatalf("lint: %d %s", code, raw)
	}
}
