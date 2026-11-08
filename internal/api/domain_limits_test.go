package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDomainLimits_RoundTrip(t *testing.T) {
	rt, db := newTestRouter(t)
	seedAppWithDomain(t, db)
	cookie := loginTestSession(t, rt, db)
	const path = "/api/v1/apps/web/domains/app.example.com/limits"

	call := func(method, body string) (domainLimitsResource, int) {
		t.Helper()
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, method, path, body))
		var got domainLimitsResource
		if rec.Code == http.StatusOK {
			if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
				t.Fatalf("decode: %v", err)
			}
		}
		return got, rec.Code
	}

	got, code := call(http.MethodGet, "")
	if code != http.StatusOK || got.Configured || got.CeilingBodyBytes <= 0 || got.CeilingTimeoutSeconds <= 0 {
		t.Fatalf("default = %+v (status %d), want unconfigured with ceilings", got, code)
	}

	got, code = call(http.MethodPut, `{"max_body_bytes":5242880,"read_timeout_seconds":30,"write_timeout_seconds":60}`)
	if code != http.StatusOK || !got.Configured || got.MaxBodyBytes != 5242880 || got.ReadTimeoutSeconds != 30 || got.WriteTimeoutSeconds != 60 {
		t.Fatalf("PUT = %+v (status %d)", got, code)
	}
	if got, _ = call(http.MethodGet, ""); got.ReadTimeoutSeconds != 30 {
		t.Fatalf("GET after PUT = %+v", got)
	}

	// All-zero is the same as clearing.
	if got, code = call(http.MethodPut, `{}`); code != http.StatusOK || got.Configured {
		t.Fatalf("PUT zeros = %+v (status %d), want cleared", got, code)
	}
	call(http.MethodPut, `{"read_timeout_seconds":5}`)
	if got, code = call(http.MethodDelete, ""); code != http.StatusOK || got.Configured {
		t.Fatalf("DELETE = %+v (status %d)", got, code)
	}
}

func TestDomainLimits_Rejects(t *testing.T) {
	rt, db := newTestRouter(t)
	seedAppWithDomain(t, db)
	cookie := loginTestSession(t, rt, db)
	const path = "/api/v1/apps/web/domains/app.example.com/limits"

	for name, body := range map[string]string{
		"body below floor":  `{"max_body_bytes":1024}`,
		"body above cap":    `{"max_body_bytes":9223372036854775807}`,
		"negative timeout":  `{"read_timeout_seconds":-1}`,
		"timeout above cap": `{"write_timeout_seconds":999999}`,
		"bad json":          `{`,
	} {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, path, body))
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%s: status = %d, want 400", name, rec.Code)
		}
	}
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/apps/web/domains/other.example.com/limits", `{"read_timeout_seconds":5}`))
	if rec.Code != http.StatusNotFound {
		t.Errorf("foreign domain status = %d, want 404", rec.Code)
	}
}
