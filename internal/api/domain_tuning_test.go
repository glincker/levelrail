package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestDomainTuning_RoundTripAndRejects(t *testing.T) {
	rt, db := newTestRouter(t)
	seedAppWithDomain(t, db)
	cookie := loginTestSession(t, rt, db)
	const path = "/api/v1/apps/web/domains/app.example.com/tuning"
	call := func(method, p, body string) (domainTuningResource, int) {
		t.Helper()
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, method, p, body))
		var got domainTuningResource
		if rec.Code == http.StatusOK {
			if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
				t.Fatalf("decode: %v", err)
			}
		}
		return got, rec.Code
	}

	got, code := call(http.MethodGet, path, "")
	if code != http.StatusOK || got.Configured || got.MinTimeoutSeconds <= 0 || got.MaxTimeoutSeconds <= 0 || got.MaxStreamSeconds <= 0 {
		t.Fatalf("default = %+v (%d), want unconfigured with bounds", got, code)
	}
	got, code = call(http.MethodPut, path, `{"preset":"websocket","websocket":true,"stream_timeout_seconds":3600,"stream_close_delay_seconds":60}`)
	if code != http.StatusOK || !got.Configured || !got.WebSocket || got.StreamTimeoutSeconds != 3600 || got.Preset != "websocket" {
		t.Fatalf("PUT = %+v (%d)", got, code)
	}
	if !got.Effective {
		t.Fatalf("fresh save must report effective: %+v", got)
	}
	if err := db.ReplaceDomainConditions(context.Background(), []store.DomainCondition{{Domain: "app.example.com", Code: store.DomainCondTuningDropped, Severity: store.DomainConditionDegraded}}); err != nil {
		t.Fatal(err)
	}
	if got, _ = call(http.MethodGet, path, ""); !got.Configured || got.Effective {
		t.Fatalf("dropped tuning must stay configured but not effective: %+v", got)
	}
	for name, body := range map[string]string{
		"unknown preset":        `{"preset":"turbo"}`,
		"raw config field":      `{"caddy_json":"{}"}`,
		"timeout above max":     `{"dial_timeout_seconds":99999999}`,
		"negative timeout":      `{"response_header_timeout_seconds":-1}`,
		"stream without socket": `{"stream_timeout_seconds":60}`,
		"compress a stream":     `{"compression":true,"streaming":true}`,
		"stream above max":      `{"websocket":true,"stream_timeout_seconds":99999999}`,
	} {
		if _, code := call(http.MethodPut, path, body); code != http.StatusBadRequest {
			t.Errorf("%s: status = %d, want 400", name, code)
		}
	}
	if _, code := call(http.MethodPut, "/api/v1/apps/web/domains/other.example.com/tuning", `{"compression":true}`); code != http.StatusNotFound {
		t.Errorf("foreign domain status = %d, want 404", code)
	}
	if got, code = call(http.MethodPut, path, `{}`); code != http.StatusOK || got.Configured {
		t.Fatalf("PUT empty = %+v (%d), want cleared", got, code)
	}
	if keys := changedTuningKeys(domainTuningStoreRow(false), domainTuningStoreRow(true)); len(keys) != 2 || keys[0] != "compression" {
		t.Errorf("changed keys = %v", keys)
	}
}

func domainTuningStoreRow(on bool) store.DomainTuning {
	if on {
		return store.DomainTuning{Compression: true, DialTimeoutSeconds: 5}
	}
	return store.DomainTuning{}
}
