package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHandleUpdateIngressSettings_PublicHTTPSPort(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	put := func(body string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/settings/ingress", body))
		return rec
	}

	for _, body := range []string{`{"public_https_port": -1}`, `{"public_https_port": 65536}`} {
		if rec := put(body); rec.Code != http.StatusBadRequest {
			t.Errorf("body %s: status = %d, want 400", body, rec.Code)
		}
	}

	rec := put(`{"public_https_port": 443, "tls_terminated_upstream": true, "acme_enabled": true, "acme_email": "ops@example.com"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var got ingressSettingsResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.PublicHTTPSPort != 443 || !got.TLSTerminatedUpstream || !got.ACMESkippedUpstream {
		t.Errorf("resource = %+v", got)
	}

	// Omitting the new fields leaves them alone.
	rec = put(`{"hsts_enabled": true}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	got = ingressSettingsResource{}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.PublicHTTPSPort != 443 || !got.TLSTerminatedUpstream {
		t.Errorf("fields were clobbered: %+v", got)
	}
}
