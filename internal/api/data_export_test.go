package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/dataexport"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/telemetry"
)

type exportTestSecrets struct{ m map[string]string }

func (s *exportTestSecrets) SetValue(_ context.Context, svc, key, v string) error {
	s.m[svc+"/"+key] = v
	return nil
}

func (s *exportTestSecrets) Resolve(_ context.Context, svc, key string) (string, error) {
	return s.m[svc+"/"+key], nil
}

func (s *exportTestSecrets) DeleteAll(_ context.Context, _ string) error { return nil }

type exportTestMetrics map[string][]telemetry.Sample

func (m exportTestMetrics) LatestByMetric(_ context.Context, metric string) ([]telemetry.Sample, error) {
	return m[metric], nil
}

func newExportRouter(t *testing.T) (*Router, *store.DB, *http.Cookie) {
	t.Helper()
	rt, db := newTestRouter(t)
	svc := dataexport.NewService(dataexport.Deps{
		Store: db, Secrets: &exportTestSecrets{m: map[string]string{}}, Brand: "plat",
		Metrics: exportTestMetrics{"cpu_percent": {
			{ResourceID: "service:web", Value: 12.5, Timestamp: time.Unix(100, 0)},
			{ResourceID: "node:n1", Value: 3, Timestamp: time.Unix(100, 0)},
		}},
		AppCount: func(context.Context) (int, error) { return 1, nil },
	})
	rt.SetDataExport(svc)
	return rt, db, loginTestSession(t, rt, db)
}

func TestExportDestinations_CRUDNeverReturnsSecrets(t *testing.T) {
	t.Parallel()
	rt, _, cookie := newExportRouter(t)
	do := func(method, path, body string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, method, path, body))
		return rec
	}

	rec := do(http.MethodPost, "/api/v1/export/destinations",
		`{"name":"loki","kind":"logs_loki","endpoint":"https://203.0.113.9:3100","apps":["web"],"min_level":"warn","headers":{"X-Scope-OrgID":"tenant-secret-value"}}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "tenant-secret-value") {
		t.Fatalf("create response leaked a header value: %s", rec.Body.String())
	}
	var created exportDestinationResource
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created.ID == "" || !created.Enabled || len(created.HeaderNames) != 1 || created.HeaderNames[0] != "X-Scope-OrgID" {
		t.Fatalf("created = %+v", created)
	}

	for _, tc := range []struct{ name, body string }{
		{"private endpoint", `{"name":"x","kind":"logs_http","endpoint":"http://127.0.0.1:9000"}`},
		{"bad kind", `{"name":"x","kind":"nope","endpoint":"https://203.0.113.9"}`},
		{"dup name", `{"name":"loki","kind":"logs_http","endpoint":"https://203.0.113.9"}`},
	} {
		rec := do(http.MethodPost, "/api/v1/export/destinations", tc.body)
		want := http.StatusBadRequest
		if tc.name == "dup name" {
			want = http.StatusConflict
		}
		if rec.Code != want {
			t.Errorf("%s: status = %d, want %d, body = %s", tc.name, rec.Code, want, rec.Body.String())
		}
	}

	rec = do(http.MethodPatch, "/api/v1/export/destinations/"+created.ID, `{"enabled":false,"min_level":"error"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("patch status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var patched exportDestinationResource
	_ = json.Unmarshal(rec.Body.Bytes(), &patched)
	if patched.Enabled || patched.MinLevel != "error" || len(patched.HeaderNames) != 1 {
		t.Fatalf("patched = %+v, want disabled, min_level error, headers kept", patched)
	}

	rec = do(http.MethodGet, "/api/v1/export/destinations", "")
	var list []exportDestinationResource
	_ = json.Unmarshal(rec.Body.Bytes(), &list)
	if rec.Code != http.StatusOK || len(list) != 1 {
		t.Fatalf("list status = %d len = %d", rec.Code, len(list))
	}

	if rec = do(http.MethodDelete, "/api/v1/export/destinations/"+created.ID, ""); rec.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d", rec.Code)
	}
	if rec = do(http.MethodGet, "/api/v1/export/destinations/"+created.ID, ""); rec.Code != http.StatusNotFound {
		t.Fatalf("get after delete status = %d", rec.Code)
	}
}

func TestExportDestinations_NotConfiguredAndReadTokenForbidden(t *testing.T) {
	t.Parallel()
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/export/destinations", ""))
	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("status = %d, want 501 when no service is wired", rec.Code)
	}

	rt2, db2, _ := newExportRouter(t)
	const plaintext = "read-only-export-token" //nolint:gosec // fake fixture
	if err := db2.SaveAPIToken(context.Background(), store.APIToken{
		ID: "tok_ro", Name: "ro", TokenHash: hashToken(plaintext), Abilities: []string{AbilityRead}, CreatedAt: time.Now(),
	}); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/api/v1/export/destinations", "/api/v1/audit-log/export"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.Header.Set("Authorization", "Bearer "+plaintext)
		rec := httptest.NewRecorder()
		rt2.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusForbidden {
			t.Errorf("%s: status = %d, want 403 for a read token", path, rec.Code)
		}
	}
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	req.Header.Set("Authorization", "Bearer "+plaintext)
	rec = httptest.NewRecorder()
	rt2.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `plat_cpu_percent{app="web"} 12.5`) {
		t.Fatalf("/metrics status = %d body = %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Header().Get("Content-Type"), "version=0.0.4") {
		t.Fatalf("content type = %q", rec.Header().Get("Content-Type"))
	}
}

func TestMetricsRequiresAuth(t *testing.T) {
	t.Parallel()
	rt, _, _ := newExportRouter(t)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401 without a token", rec.Code)
	}
}

func TestAuditLogExport_FormatsRangeAndRedaction(t *testing.T) {
	t.Parallel()
	rt, db, cookie := newExportRouter(t)
	base := time.Now().UTC().Add(-time.Hour).Truncate(time.Second)
	for i, p := range []string{"/a", "/api/v1/hooks/ghp_abcdefghijklmnopqrstuvwxyz0123456789", "/c"} {
		if err := db.SaveAuditEntry(context.Background(), store.AuditEntry{
			ID: "aud_" + string(rune('a'+i)), ActorType: "token", ActorName: "ci", Ability: "write", Method: "POST",
			Path: p, StatusCode: 200, CreatedAt: store.FormatAuditTime(base.Add(time.Duration(i) * time.Minute)), ClientKind: "api",
		}); err != nil {
			t.Fatal(err)
		}
	}
	get := func(q string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/audit-log/export"+q, ""))
		return rec
	}

	rec := get("?from=" + base.Add(time.Minute).Format(time.RFC3339) + "&to=" + base.Add(2*time.Minute).Format(time.RFC3339))
	if rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != "application/x-ndjson" {
		t.Fatalf("status = %d type = %q", rec.Code, rec.Header().Get("Content-Type"))
	}
	lines := strings.Split(strings.TrimSpace(rec.Body.String()), "\n")
	if len(lines) != 1 || strings.Contains(lines[0], "ghp_") || !strings.Contains(lines[0], `"id":"aud_b"`) {
		t.Fatalf("ndjson lines = %v", lines)
	}

	rec = get("?format=csv")
	if rec.Code != http.StatusOK || !strings.HasPrefix(rec.Body.String(), "id,time,actor_type") {
		t.Fatalf("csv = %d %q", rec.Code, rec.Body.String())
	}
	if n := strings.Count(strings.TrimSpace(rec.Body.String()), "\n"); n < 3 {
		t.Fatalf("csv rows = %d, want header plus the 3 seeded rows", n)
	}
	for q, want := range map[string]int{"?format=xml": 400, "?from=yesterday": 400} {
		if rec := get(q); rec.Code != want {
			t.Errorf("%s status = %d, want %d", q, rec.Code, want)
		}
	}
}
