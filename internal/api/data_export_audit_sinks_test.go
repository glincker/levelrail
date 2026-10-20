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
)

func TestExportDestinations_AuditStreamSinks(t *testing.T) {
	rt, _, cookie := newExportRouter(t)
	do := func(method, path, body string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, method, path, body))
		return rec
	}
	const token = "hec-token-do-not-leak"

	rec := do(http.MethodPost, "/api/v1/export/destinations",
		`{"name":"splunk","kind":"audit_splunk","endpoint":"https://203.0.113.9:8088","api_key":"`+token+`","options":{"index":"audit","source_type":"lr:audit","gzip":true}}`)
	if rec.Code != http.StatusCreated || strings.Contains(rec.Body.String(), token) {
		t.Fatalf("splunk create status = %d leaked = %t body = %s", rec.Code, strings.Contains(rec.Body.String(), token), rec.Body.String())
	}
	var created exportDestinationResource
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if !created.HasAPIKey || created.Options.Index != "audit" || !created.Options.Gzip || created.Options.InsecureSkipVerify {
		t.Fatalf("created = %+v", created)
	}

	rec = do(http.MethodPatch, "/api/v1/export/destinations/"+created.ID, `{"options":{"index":"audit","insecure_skip_verify":true}}`)
	var patched exportDestinationResource
	_ = json.Unmarshal(rec.Body.Bytes(), &patched)
	if rec.Code != http.StatusOK || !patched.Options.InsecureSkipVerify || !patched.HasAPIKey {
		t.Fatalf("patch status = %d resource = %+v", rec.Code, patched)
	}

	for _, tc := range []struct{ name, body string }{
		{"splunk no token", `{"name":"a","kind":"audit_splunk","endpoint":"https://203.0.113.9:8088"}`},
		{"splunk metadata", `{"name":"b","kind":"audit_splunk","endpoint":"http://169.254.169.254/x","api_key":"t"}`},
		{"syslog bad facility", `{"name":"c","kind":"audit_syslog","endpoint":"udp://203.0.113.9:514","options":{"facility":"nope"}}`},
		{"otlp private", `{"name":"d","kind":"audit_otlp","endpoint":"http://127.0.0.1:4318"}`},
	} {
		if rec := do(http.MethodPost, "/api/v1/export/destinations", tc.body); rec.Code != http.StatusBadRequest {
			t.Errorf("%s: status = %d, want 400, body = %s", tc.name, rec.Code, rec.Body.String())
		}
	}
	for _, body := range []string{
		`{"name":"sys","kind":"audit_syslog","endpoint":"tls://203.0.113.9:6514","options":{"facility":"local0","app_name":"lr"}}`,
		`{"name":"otel","kind":"audit_otlp","endpoint":"https://203.0.113.9:4318","headers":{"X-Api-Key":"k"}}`,
	} {
		if rec := do(http.MethodPost, "/api/v1/export/destinations", body); rec.Code != http.StatusCreated {
			t.Errorf("create %s: status = %d, body = %s", body, rec.Code, rec.Body.String())
		}
	}
}

func TestExportDestinations_TestReportsTypedFix(t *testing.T) {
	t.Setenv(dataexport.AllowPrivateEnv, "true")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusForbidden) }))
	defer srv.Close()
	rt, _, cookie := newExportRouter(t)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/export/destinations",
		`{"name":"splunk","kind":"audit_splunk","endpoint":"`+srv.URL+`","api_key":"bad"}`))
	var created exportDestinationResource
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil || rec.Code != http.StatusCreated {
		t.Fatalf("create status = %d err = %v body = %s", rec.Code, err, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/export/destinations/"+created.ID+"/test", ""))
	var res exportTestResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &res)
	if rec.Code != http.StatusOK || res.OK || res.Fix != "bad_token" {
		t.Fatalf("test status = %d response = %+v, want ok=false fix=bad_token", rec.Code, res)
	}
}

func TestExportDestinations_AuditSinkWritesNeedAWriteAbility(t *testing.T) {
	rt, db, _ := newExportRouter(t)
	const plaintext = "read-only-sink-token" //nolint:gosec // fake fixture
	if err := db.SaveAPIToken(context.Background(), store.APIToken{
		ID: "tok_ro2", Name: "ro", TokenHash: hashToken(plaintext), Abilities: []string{AbilityRead}, CreatedAt: time.Now(),
	}); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/export/destinations",
		strings.NewReader(`{"name":"s","kind":"audit_syslog","endpoint":"udp://203.0.113.9:514"}`))
	req.Header.Set("Authorization", "Bearer "+plaintext)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403 for a read token creating an audit sink", rec.Code)
	}
}
