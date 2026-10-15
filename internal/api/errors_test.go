package api

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/telemetry"
)

func newErrorRouter(t *testing.T) (*Router, *store.DB, *telemetry.DB) {
	t.Helper()
	db := openTestDB(t)
	tel, err := telemetry.Open(context.Background(), filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = tel.Close() })
	sec := &traceSecrets{values: map[string]string{}}
	logger := slog.New(slog.NewTextHandler(discardWriter{}, nil))
	return NewRouter(logger, testBrand(), db, WithSecretSetter(sec), WithErrorStore(tel)), db, tel
}

func sentryEnvelope(msg string) []byte {
	ev := fmt.Sprintf(`{"event_id":"aaaabbbbccccddddeeeeffff00001111","level":"error","message":%q,"release":"v9"}`, msg)
	return []byte(`{"event_id":"aaaabbbbccccddddeeeeffff00001111"}` + "\n" +
		fmt.Sprintf(`{"type":"event","length":%d}`, len(ev)) + "\n" + ev + "\n")
}

func TestSentryIngestAuthLimitAndIssues(t *testing.T) {
	t.Setenv("APP_ERROR_INGEST_PER_MIN", "3")
	rt, db, _ := newErrorRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedWebAppForTest(t, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/error-tracking", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("enable = %d %s", rec.Code, rec.Body)
	}
	var res errorTrackingResource
	_ = json.NewDecoder(rec.Body).Decode(&res)
	key := strings.TrimPrefix(strings.SplitN(res.DSN, "@", 2)[0], "http://")
	if key == "" || !strings.HasSuffix(res.DSN, "/api/v1/sentry/1") || res.Env["SENTRY_DSN"] != res.DSN {
		t.Fatalf("enable response = %+v", res)
	}

	post := func(key string, body []byte, gz bool) *httptest.ResponseRecorder {
		if gz {
			var b bytes.Buffer
			zw := gzip.NewWriter(&b)
			_, _ = zw.Write(body)
			_ = zw.Close()
			body = b.Bytes()
		}
		req := httptest.NewRequest(http.MethodPost, "/api/v1/sentry/api/1/envelope/", bytes.NewReader(body))
		if gz {
			req.Header.Set("Content-Encoding", "gzip")
		}
		if key != "" {
			req.Header.Set("X-Sentry-Auth", "Sentry sentry_version=7, sentry_key="+key+", sentry_client=x/1")
		}
		w := httptest.NewRecorder()
		rt.Handler().ServeHTTP(w, req)
		return w
	}
	if w := post("", sentryEnvelope("a"), false); w.Code != http.StatusUnauthorized {
		t.Fatalf("no key = %d, want 401", w.Code)
	}
	if w := post(key+"x", sentryEnvelope("a"), false); w.Code != http.StatusUnauthorized {
		t.Fatalf("wrong key = %d, want 401", w.Code)
	}
	w := post(key, sentryEnvelope("user 1 not found"), false)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"id":"aaaabbbbccccddddeeeeffff00001111"`) {
		t.Fatalf("ok = %d %s", w.Code, w.Body)
	}
	if w := post(key, sentryEnvelope("user 2 not found"), true); w.Code != http.StatusOK {
		t.Fatalf("gzip = %d %s", w.Code, w.Body)
	}
	if w := post(key, sentryEnvelope("x"), false); w.Code != http.StatusOK {
		t.Fatalf("third = %d", w.Code)
	}
	if w := post(key, sentryEnvelope("x"), false); w.Code != http.StatusTooManyRequests || w.Header().Get("Retry-After") == "" {
		t.Fatalf("over limit = %d retry-after=%q, want 429 with header", w.Code, w.Header().Get("Retry-After"))
	}

	rec = httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/errors?status=open&since=1h", ""))
	var issues []telemetry.ErrorIssue
	_ = json.NewDecoder(rec.Body).Decode(&issues)
	if rec.Code != http.StatusOK || len(issues) != 2 {
		t.Fatalf("list = %d %+v, want 2 issues (user N grouped)", rec.Code, issues)
	}
	var grouped telemetry.ErrorIssue
	for _, i := range issues {
		if i.Count == 2 {
			grouped = i
		}
	}
	if grouped.Fingerprint == "" || grouped.LastRelease != "v9" {
		t.Fatalf("issues = %+v", issues)
	}

	path := "/api/v1/apps/web/errors/" + grouped.Fingerprint
	rec = httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPatch, path, `{"status":"resolved"}`))
	var got errorIssueResource
	_ = json.NewDecoder(rec.Body).Decode(&got)
	if rec.Code != http.StatusOK || got.Issue.Status != "resolved" || len(got.Events) != 2 {
		t.Fatalf("patch = %d %+v", rec.Code, got)
	}
	rec = httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPatch, path, `{"status":"bogus"}`))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("bad status = %d", rec.Code)
	}
	rec = httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/errors/nope", ""))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("missing = %d", rec.Code)
	}
}

func TestErrorBucketRefills(t *testing.T) {
	var s errorTrackingState
	now := time.Unix(1000, 0)
	for i := 0; i < 60; i++ {
		if ok, _ := s.allow("a", 60, now); !ok {
			t.Fatalf("token %d denied", i)
		}
	}
	if ok, wait := s.allow("a", 60, now); ok || wait <= 0 {
		t.Fatalf("empty bucket allowed=%v wait=%v", ok, wait)
	}
	if ok, _ := s.allow("a", 60, now.Add(2*time.Second)); !ok {
		t.Fatal("bucket did not refill")
	}
}
