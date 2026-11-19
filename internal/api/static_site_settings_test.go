package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/GLINCKER/levelrail/internal/staticroutes"
)

func TestStaticSiteSettingsAndRules(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	root := t.TempDir()
	seedStaticSite(t, db, root, "docs", "d.example.com")
	seedApp(t, db, "docs")
	dir := filepath.Join(root, "docs", "abc")
	rs, err := staticroutes.Compile(staticroutes.Input{Redirects: "/old /file-target 301\n"}, staticroutes.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	if err := staticroutes.WriteSidecar(dir, rs); err != nil {
		t.Fatal(err)
	}
	do := func(method, path, body string) (int, []byte) {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, method, path, body))
		return rec.Code, rec.Body.Bytes()
	}

	code, raw := do(http.MethodPatch, "/api/v1/static-sites/docs/settings", `{"spa_fallback":true,"trailing_slash":"add","build_command":"npm run build","output_dir":"dist"}`)
	var set staticSettingsResource
	if code != http.StatusOK || json.Unmarshal(raw, &set) != nil || set.SPAFallback == nil || !*set.SPAFallback || !set.Effective.SPA || set.OutputDir != "dist" {
		t.Fatalf("patch settings: %d %s", code, raw)
	}
	if code, _ = do(http.MethodPatch, "/api/v1/static-sites/docs/settings", `{"trailing_slash":"sideways"}`); code != http.StatusUnprocessableEntity {
		t.Fatalf("bad trailing slash = %d, want 422", code)
	}
	if code, _ = do(http.MethodPatch, "/api/v1/static-sites/docs/settings", `{"output_dir":"../etc"}`); code != http.StatusUnprocessableEntity {
		t.Fatalf("escaping output dir = %d, want 422", code)
	}
	code, raw = do(http.MethodPatch, "/api/v1/static-sites/docs/settings", `{"reset":["spa_fallback"]}`)
	if code != http.StatusOK || json.Unmarshal(raw, &set) != nil || set.SPAFallback != nil || set.TrailingSlash != "add" {
		t.Fatalf("reset: %d %s", code, raw)
	}

	// A lint failure stores nothing and reports each failing entry.
	code, raw = do(http.MethodPut, "/api/v1/static-sites/docs/rules", `{"redirects":[{"from":"/a","to":"/b"},{"from":"/c"}],"headers":[]}`)
	var fail staticRulesLintFailure
	if code != http.StatusUnprocessableEntity || json.Unmarshal(raw, &fail) != nil || len(fail.Problems) != 1 || fail.Problems[0].Index != 1 {
		t.Fatalf("lint failure: %d %s", code, raw)
	}

	// A valid edit is live in the sidecar right away and ranks before the file rule.
	code, raw = do(http.MethodPut, "/api/v1/static-sites/docs/rules", `{"redirects":[{"from":"/old","to":"/ui-target","status":302}],"headers":[{"path":"/*","set":{"X-Frame-Options":"DENY"}}]}`)
	var eff staticEffectiveRulesResource
	if code != http.StatusOK || json.Unmarshal(raw, &eff) != nil || len(eff.Redirects) != 2 || eff.Redirects[0].Source != "ui" || eff.Redirects[1].Source != "file" || len(eff.UI.Redirects) != 1 {
		t.Fatalf("put rules: %d %s", code, raw)
	}
	live, err := staticroutes.LoadSidecar(dir)
	if err != nil || live == nil || len(live.Rules) != 2 || live.Rules[0].To != "/ui-target" || len(live.Headers) != 1 || live.Options.TrailingSlash != "add" {
		t.Fatalf("live sidecar after edit: %+v %v", live, err)
	}
	code, raw = do(http.MethodGet, "/api/v1/static-sites/docs/rules", "")
	if code != http.StatusOK || json.Unmarshal(raw, &eff) != nil || !eff.Published || eff.Headers[0].Source != "ui" {
		t.Fatalf("get rules: %d %s", code, raw)
	}
	// Clearing the remaining overrides restores the compiled defaults on the live site at once.
	code, raw = do(http.MethodPatch, "/api/v1/static-sites/docs/settings", `{"reset":["trailing_slash"]}`)
	var cleared staticSettingsResource
	if code != http.StatusOK || json.Unmarshal(raw, &cleared) != nil || cleared.Effective.TrailingSlash != "" || cleared.Effective.SPA {
		t.Fatalf("reset live: %d %s", code, raw)
	}
	if live, err := staticroutes.LoadSidecar(dir); err != nil || live == nil || live.Options.SPA || live.Options.TrailingSlash != "" {
		t.Fatalf("sidecar after reset: %+v %v", live, err)
	}

	if code, _ = do(http.MethodGet, "/api/v1/static-sites/ghost/settings", ""); code != http.StatusNotFound {
		t.Fatalf("unknown site = %d, want 404", code)
	}
}
