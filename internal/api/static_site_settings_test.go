package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/staticroutes"
	"github.com/GLINCKER/levelrail/internal/store"
	"golang.org/x/crypto/bcrypt"
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

// Settings must be storable before the first publish: the site row only exists after a build.
func TestStaticSiteSettingsBeforeFirstPublish(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	do := func(method, path, body string) (int, []byte) {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, method, path, body))
		return rec.Code, rec.Body.Bytes()
	}
	if code, raw := do(http.MethodPost, "/api/v1/static-sites", `{"name":"fresh"}`); code != http.StatusCreated {
		t.Fatalf("create: %d %s", code, raw)
	}
	code, raw := do(http.MethodPatch, "/api/v1/static-sites/fresh/settings", `{"build_command":"npm run build","output_dir":"dist"}`)
	var set staticSettingsResource
	if code != http.StatusOK || json.Unmarshal(raw, &set) != nil || set.BuildCommand != "npm run build" || set.OutputDir != "dist" {
		t.Fatalf("patch before publish: %d %s", code, raw)
	}
	code, raw = do(http.MethodGet, "/api/v1/static-sites/fresh/settings", "")
	if code != http.StatusOK || json.Unmarshal(raw, &set) != nil || set.OutputDir != "dist" {
		t.Fatalf("get before publish: %d %s", code, raw)
	}
	seedApp(t, db, "plain")
	if code, _ = do(http.MethodPatch, "/api/v1/static-sites/plain/settings", `{"output_dir":"dist"}`); code != http.StatusNotFound {
		t.Fatalf("non-static app = %d, want 404", code)
	}
}

func TestStaticSettingsNormaliseOutputAndPlaceholderImage(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	do := func(method, path, body string) (int, []byte) {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, method, path, body))
		return rec.Code, rec.Body.Bytes()
	}
	if code, raw := do(http.MethodPost, "/api/v1/static-sites", `{"name":"fresh2"}`); code != http.StatusCreated {
		t.Fatalf("create: %d %s", code, raw)
	}
	code, raw := do(http.MethodPatch, "/api/v1/static-sites/fresh2/settings", `{"output_dir":"./dist/"}`)
	var set staticSettingsResource
	if code != http.StatusOK || json.Unmarshal(raw, &set) != nil || set.OutputDir != "dist" {
		t.Fatalf("patch: %d %s", code, raw)
	}
	if err := db.SaveDesiredService(t.Context(), store.DesiredService{Name: "other", Image: "nginx:pending", Port: 80}); err != nil {
		t.Fatal(err)
	}
	if code, _ = do(http.MethodPatch, "/api/v1/static-sites/other/settings", `{"output_dir":"dist"}`); code != http.StatusNotFound {
		t.Fatalf("foreign :pending image = %d, want 404", code)
	}
}

func TestStaticSiteSettingsPasswordGate(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedStaticSite(t, db, t.TempDir(), "docs", "d.example.com")
	seedApp(t, db, "docs")
	do := func(method, body string) (int, string) {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, method, "/api/v1/static-sites/docs/settings", body))
		return rec.Code, rec.Body.String()
	}
	if code, _ := do(http.MethodPatch, `{"password":"short"}`); code != http.StatusUnprocessableEntity {
		t.Fatalf("short password = %d, want 422", code)
	}
	code, body := do(http.MethodPatch, `{"password":"correct horse battery"}`)
	if code != http.StatusOK || !strings.Contains(body, `"protected":true`) || strings.Contains(body, "correct horse") || strings.Contains(body, "$2") {
		t.Fatalf("set password: %d %s", code, body)
	}
	hash, err := db.GetStaticSitePasswordHash(t.Context(), "docs")
	if err != nil || bcrypt.CompareHashAndPassword([]byte(hash), []byte("correct horse battery")) != nil {
		t.Fatalf("stored hash does not verify: %v", err)
	}
	if code, body = do(http.MethodGet, ""); code != http.StatusOK || !strings.Contains(body, `"protected":true`) {
		t.Fatalf("get: %d %s", code, body)
	}
	if code, body = do(http.MethodPatch, `{"password":""}`); code != http.StatusOK || !strings.Contains(body, `"protected":false`) {
		t.Fatalf("clear: %d %s", code, body)
	}
}

func TestStaticSiteSettingsBadPasswordSavesNothing(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedStaticSite(t, db, t.TempDir(), "docs", "d.example.com")
	seedApp(t, db, "docs")
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPatch, "/api/v1/static-sites/docs/settings", `{"output_dir":"dist","password":"short"}`))
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422", rec.Code)
	}
	s, _, err := db.GetStaticSiteSettings(t.Context(), "docs")
	if err != nil || s.OutputDir != "" {
		t.Fatalf("output_dir = %q, err %v; want nothing saved", s.OutputDir, err)
	}
}
