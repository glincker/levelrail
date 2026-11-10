package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/runtimesettings"
	"github.com/GLINCKER/levelrail/internal/store"
)

func TestStatementTimingServerTimingHeader(t *testing.T) {
	rt, db := newTestRouter(t)
	if rt.runtimeSettings == nil {
		rt.runtimeSettings = runtimesettings.New(db, func(string) (string, bool) { return "", false })
	}
	t.Cleanup(func() { store.SetStatementTiming(store.StatementTimingConfig{Enabled: false}) })
	cookie := loginTestSession(t, rt, db)
	get := func(path string, auth func(*http.Request)) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		if auth != nil {
			auth(req)
		}
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, req)
		return rec
	}
	admin := func(r *http.Request) { r.AddCookie(cookie) }

	if rec := get("/api/v1/system/slow-statements", admin); rec.Code != http.StatusOK || rec.Header().Get(headerServerTiming) != "" {
		t.Fatalf("off: code %d header %q", rec.Code, rec.Header().Get(headerServerTiming))
	}
	path := rsBase + "/" + runtimesettings.KeyPerfStatementTiming
	if rec := rsDo(t, rt, cookie, http.MethodPut, path, `{"value":true}`); rec.Code != http.StatusOK {
		t.Fatalf("enable = %d %s", rec.Code, rec.Body.String())
	}
	rec := get("/api/v1/system/slow-statements", admin)
	h := rec.Header().Get(headerServerTiming)
	if !strings.HasPrefix(h, "db;dur=") || !strings.Contains(h, "queries;desc=") {
		t.Fatalf("admin header = %q", h)
	}
	var body slowStatementsResource
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || !body.Enabled || body.RingSize != 20 {
		t.Fatalf("body = %+v err %v", body, err)
	}

	for i, abilities := range []string{`["deploy"]`, `["read"]`, `["read","write"]`} {
		code, tok := mintToken(t, rt, cookie, `{"name":"st-`+string(rune('a'+i))+`","abilities":`+abilities+`}`)
		if code != http.StatusCreated {
			t.Fatalf("mint %s = %d", abilities, code)
		}
		bearer := func(r *http.Request) { r.Header.Set("Authorization", "Bearer "+tok.Token) }
		if rec := get("/api/v1/apps", bearer); rec.Header().Get(headerServerTiming) != "" {
			t.Fatalf("%s caller got header %q", abilities, rec.Header().Get(headerServerTiming))
		}
		if rec := get("/api/v1/system/slow-statements", bearer); rec.Code != http.StatusForbidden {
			t.Fatalf("%s caller read slow statements: %d", abilities, rec.Code)
		}
	}
	if rec := get("/api/v1/apps", nil); rec.Header().Get(headerServerTiming) != "" {
		t.Fatalf("anonymous got header")
	}
	if rec := get("/healthz", admin); rec.Header().Get(headerServerTiming) != "" {
		t.Fatalf("non api path got header")
	}

	if rec := rsDo(t, rt, cookie, http.MethodPut, path, `{"value":false}`); rec.Code != http.StatusOK {
		t.Fatalf("disable = %d", rec.Code)
	}
	if rec := get("/api/v1/system/slow-statements", admin); rec.Header().Get(headerServerTiming) != "" || store.StatementTimingEnabled() {
		t.Fatalf("live off did not apply")
	}
}

func timingQueries(t *testing.T, h string) int {
	t.Helper()
	_, after, ok := strings.Cut(h, `queries;desc="`)
	if !ok {
		t.Fatalf("no query count in %q", h)
	}
	n, err := strconv.Atoi(strings.TrimSuffix(after, `"`))
	if err != nil {
		t.Fatalf("query count in %q: %v", h, err)
	}
	return n
}

func TestStatementTimingRootTokenGates(t *testing.T) {
	rt, db := newTestRouter(t)
	if rt.runtimeSettings == nil {
		rt.runtimeSettings = runtimesettings.New(db, func(string) (string, bool) { return "", false })
	}
	t.Cleanup(func() { store.SetStatementTiming(store.StatementTimingConfig{Enabled: false}) })
	cookie := loginTestSession(t, rt, db)
	if rec := rsDo(t, rt, cookie, http.MethodPut, rsBase+"/"+runtimesettings.KeyPerfStatementTiming, `{"value":true}`); rec.Code != http.StatusOK {
		t.Fatalf("enable = %d", rec.Code)
	}
	code, root := mintToken(t, rt, cookie, `{"name":"st-root","abilities":["root"]}`)
	if code != http.StatusCreated {
		t.Fatalf("mint root = %d", code)
	}
	get := func(path, etag string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.Header.Set("Authorization", "Bearer "+root.Token)
		if etag != "" {
			req.Header.Set("If-None-Match", etag)
		}
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, req)
		return rec
	}

	rec := get("/api/v1/apps", "")
	if rec.Code != http.StatusOK || timingQueries(t, rec.Header().Get(headerServerTiming)) < 1 {
		t.Fatalf("root token: code %d header %q", rec.Code, rec.Header().Get(headerServerTiming))
	}
	if etag := rec.Header().Get("ETag"); etag != "" {
		if rec := get("/api/v1/apps", etag); rec.Code == http.StatusNotModified && timingQueries(t, rec.Header().Get(headerServerTiming)) < 1 {
			t.Fatalf("304 lost the count: %q", rec.Header().Get(headerServerTiming))
		}
	}

	rt.apiRateLimit = newAPIRateLimit(1, 1)
	var limited bool
	for range 10 {
		rec := get("/api/v1/apps", "")
		if rec.Code == http.StatusTooManyRequests {
			limited = true
			if h := rec.Header().Get(headerServerTiming); h != "" {
				t.Fatalf("429 carried %q", h)
			}
		}
	}
	if !limited {
		t.Fatal("rate limit never applied")
	}
	rt.apiRateLimit = nil

	if _, err := db.ExecContext(context.Background(), `UPDATE api_token_bindings SET instance_id = 'inst_other' WHERE token_id = ?`, root.ID); err != nil {
		t.Fatal(err)
	}
	if rec := get("/api/v1/apps", ""); rec.Code != http.StatusUnauthorized || rec.Header().Get(headerServerTiming) != "" {
		t.Fatalf("root token bound elsewhere: code %d header %q", rec.Code, rec.Header().Get(headerServerTiming))
	}
}
