package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/statuspage"
	"github.com/GLINCKER/levelrail/internal/store"
)

func newStatusRouter(t *testing.T, perMinute int) (*Router, *store.DB, *http.Cookie) {
	t.Helper()
	db := openTestDB(t)
	logger := slog.New(slog.NewTextHandler(discardWriter{}, nil))
	rt := NewRouter(logger, testBrand(), db, WithStatusPage(db, statuspage.Config{}, perMinute))
	return rt, db, loginTestSession(t, rt, db)
}

func publicGet(rt *Router, target string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, target, nil)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	return rec
}

func TestStatusPage_NotConfigured(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	if rec := apiDo(t, rt, cookie, http.MethodGet, "/api/v1/status-page", ""); rec.Code != http.StatusNotImplemented {
		t.Errorf("management status = %d, want 501", rec.Code)
	}
	if rec := publicGet(rt, "/public/status"); rec.Code != http.StatusNotFound {
		t.Errorf("public status = %d, want 404", rec.Code)
	}
}

func TestStatusPage_ManagementValidation(t *testing.T) {
	rt, db, cookie := newStatusRouter(t, 100)
	seedApp(t, db, "secret-internal-app")

	bad := map[string]string{
		"no display name":     `{"kind":"app","target":"secret-internal-app","display_name":""}`,
		"unknown app":         `{"kind":"app","target":"ghost","display_name":"Ghost"}`,
		"bad domain":          `{"kind":"domain","target":"not a host","display_name":"Site"}`,
		"check with userinfo": `{"kind":"check","target":"https://user@example.com","display_name":"API"}`,
		"check wrong scheme":  `{"kind":"check","target":"ftp://example.com","display_name":"API"}`,
		"unknown kind":        `{"kind":"database","target":"x","display_name":"DB"}`,
		"display name length": `{"kind":"domain","target":"example.com","display_name":"` + strings.Repeat("a", 81) + `"}`,
	}
	for name, body := range bad {
		if rec := apiDo(t, rt, cookie, http.MethodPost, "/api/v1/status-page/components", body); rec.Code != http.StatusBadRequest {
			t.Errorf("%s status = %d, want 400, body = %s", name, rec.Code, rec.Body.String())
		}
	}
	for name, body := range map[string]string{
		"bad domain setting": `{"enabled":true,"custom_domain":"http://x"}`,
		"long title":         `{"enabled":true,"title":"` + strings.Repeat("t", 200) + `"}`,
	} {
		if rec := apiDo(t, rt, cookie, http.MethodPut, "/api/v1/status-page", body); rec.Code != http.StatusBadRequest {
			t.Errorf("%s status = %d, want 400", name, rec.Code)
		}
	}
	for name, body := range map[string]string{
		"maintenance without window": `{"kind":"maintenance","title":"x"}`,
		"bad status":                 `{"kind":"incident","title":"x","status":"scheduled"}`,
		"bad impact":                 `{"kind":"incident","title":"x","impact":"huge"}`,
		"unknown component":          `{"kind":"incident","title":"x","component_ids":["sc_nope"]}`,
		"no title":                   `{"kind":"incident","title":""}`,
	} {
		if rec := apiDo(t, rt, cookie, http.MethodPost, "/api/v1/status-page/incidents", body); rec.Code != http.StatusBadRequest {
			t.Errorf("%s status = %d, want 400, body = %s", name, rec.Code, rec.Body.String())
		}
	}
}

func TestStatusPage_PublicPageExposesOnlyChosenNames(t *testing.T) {
	rt, db, cookie := newStatusRouter(t, 100)
	seedApp(t, db, "secret-internal-app")

	if rec := publicGet(rt, "/public/status"); rec.Code != http.StatusNotFound {
		t.Fatalf("disabled page status = %d, want 404 (opt-in)", rec.Code)
	}
	if rec := apiDo(t, rt, cookie, http.MethodPut, "/api/v1/status-page", `{"enabled":true,"title":"Acme status","description":"Live service health"}`); rec.Code != http.StatusOK {
		t.Fatalf("enable status = %d, body = %s", rec.Code, rec.Body.String())
	}
	rec := apiDo(t, rt, cookie, http.MethodPost, "/api/v1/status-page/components", `{"kind":"app","target":"secret-internal-app","display_name":"Payments"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("component status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var comp statusComponentResource
	_ = json.Unmarshal(rec.Body.Bytes(), &comp)
	rec = apiDo(t, rt, cookie, http.MethodPost, "/api/v1/status-page/incidents",
		`{"kind":"incident","title":"Slow payments","impact":"minor","body":"We are **investigating** <script>x</script>","component_ids":["`+comp.ID+`"]}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("incident status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var inc statusIncidentResource
	_ = json.Unmarshal(rec.Body.Bytes(), &inc)

	for _, path := range []string{"/public/status", "/public/status.json", "/public/status.rss"} {
		rec := publicGet(rt, path)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s status = %d", path, rec.Code)
		}
		body := rec.Body.String()
		if strings.Contains(body, "secret-internal-app") {
			t.Errorf("%s leaks the internal app name", path)
		}
		if !strings.Contains(body, "Slow payments") {
			t.Errorf("%s must list the incident", path)
		}
		if strings.Contains(body, "<script>") {
			t.Errorf("%s must escape operator text", path)
		}
		if rec.Header().Get("Cache-Control") == "" || rec.Header().Get("ETag") == "" {
			t.Errorf("%s must be cacheable", path)
		}
		if got := rec.Header().Get("Content-Security-Policy"); !strings.Contains(got, "default-src 'none'") {
			t.Errorf("%s CSP = %q", path, got)
		}
	}
	page := publicGet(rt, "/public/status").Body.String()
	if !strings.Contains(page, "Payments") || !strings.Contains(page, "Acme status") {
		t.Error("page must show the display name and title")
	}

	rec = publicGet(rt, "/public/status")
	req := httptest.NewRequest(http.MethodGet, "/public/status", nil)
	req.Header.Set("If-None-Match", rec.Header().Get("ETag"))
	cached := httptest.NewRecorder()
	rt.Handler().ServeHTTP(cached, req)
	if cached.Code != http.StatusNotModified {
		t.Errorf("conditional request status = %d, want 304", cached.Code)
	}

	up := `{"status":"resolved","body":"Fixed."}`
	if rec = apiDo(t, rt, cookie, http.MethodPost, "/api/v1/status-page/incidents/"+inc.ID+"/updates", up); rec.Code != http.StatusOK {
		t.Fatalf("update status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if body := publicGet(rt, "/public/status.json").Body.String(); !strings.Contains(body, `"resolved"`) || !strings.Contains(body, "Fixed.") {
		t.Errorf("resolved update must show up: %s", body)
	}

	if rec = apiDo(t, rt, cookie, http.MethodPut, "/api/v1/status-page", `{"enabled":false}`); rec.Code != http.StatusOK {
		t.Fatal("disable failed")
	}
	if rec = publicGet(rt, "/public/status"); rec.Code != http.StatusNotFound {
		t.Errorf("after disabling status = %d, want 404", rec.Code)
	}
}

func TestStatusPage_PublicRateLimited(t *testing.T) {
	rt, db, cookie := newStatusRouter(t, 2)
	_ = db
	apiDo(t, rt, cookie, http.MethodPut, "/api/v1/status-page", `{"enabled":true}`)
	var limited bool
	for i := 0; i < 5; i++ {
		if rec := publicGet(rt, "/public/status"); rec.Code == http.StatusTooManyRequests {
			limited = true
			if rec.Header().Get("Retry-After") == "" {
				t.Error("429 needs Retry-After")
			}
		}
	}
	if !limited {
		t.Error("public route must be rate limited")
	}
}

func TestStatusPage_CustomDomainServesOnlyStatus(t *testing.T) {
	rt, _, cookie := newStatusRouter(t, 100)
	apiDo(t, rt, cookie, http.MethodPut, "/api/v1/status-page", `{"enabled":true,"title":"Acme","custom_domain":"status.example.com"}`)

	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("dashboard")) })
	h := rt.StatusHostHandler(next)
	get := func(host, path string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.Host = host
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}
	if rec := get("status.example.com", "/"); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Acme") {
		t.Errorf("custom domain root = %d %q", rec.Code, rec.Body.String())
	}
	if rec := get("STATUS.example.com:443", "/status.json"); rec.Code != http.StatusOK {
		t.Errorf("host match must ignore case and port, got %d", rec.Code)
	}
	for _, path := range []string{"/api/v1/apps", "/login", "/settings"} {
		if rec := get("status.example.com", path); rec.Code != http.StatusNotFound {
			t.Errorf("custom domain %s = %d, want 404 (no dashboard on the public host)", path, rec.Code)
		}
	}
	if rec := get("dash.example.com", "/"); rec.Body.String() != "dashboard" {
		t.Error("other hosts must pass through untouched")
	}
}
