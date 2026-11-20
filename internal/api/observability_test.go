package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

type obsReceiver struct {
	mu    sync.Mutex
	paths []string
	auth  []string
	code  int
}

func (o *obsReceiver) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	o.mu.Lock()
	o.paths = append(o.paths, r.URL.Path)
	o.auth = append(o.auth, r.Header.Get("Authorization"))
	code := o.code
	o.mu.Unlock()
	w.WriteHeader(code)
}

func TestObservabilityExports_PresetCreateTestListDelete(t *testing.T) {
	t.Setenv("APP_EXPORT_ALLOW_PRIVATE_NETWORKS", "true")
	recv := &obsReceiver{code: http.StatusOK}
	srv := httptest.NewServer(recv)
	t.Cleanup(srv.Close)
	rt, _, cookie := newExportRouter(t)
	do := func(method, path, body string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, method, path, body))
		return rec
	}
	hdrValue := "Bearer zz-leakcheck-value" //nolint:gosec // synthetic test value

	cat := do(http.MethodGet, "/api/v1/observability/presets", "")
	if cat.Code != http.StatusOK || !strings.Contains(cat.Body.String(), `"id":"grafana-cloud"`) || strings.Contains(cat.Body.String(), "metrics_otlp") {
		t.Fatalf("catalog = %d %s", cat.Code, cat.Body.String())
	}

	body := `{"preset":"otlp-http","name":"prod","fields":{"endpoint":"` + srv.URL + `","header_value":"` + hdrValue + `"}}`
	rec := do(http.MethodPost, "/api/v1/observability/exports", body)
	if rec.Code != http.StatusCreated || strings.Contains(rec.Body.String(), "zz-leakcheck") {
		t.Fatalf("create = %d %s", rec.Code, rec.Body.String())
	}
	var created observabilityExportResource
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created.ID != "prod" || len(created.Signals) != 2 || !created.Signals[0].HasCredential {
		t.Fatalf("created = %+v", created)
	}
	if dup := do(http.MethodPost, "/api/v1/observability/exports", body); dup.Code != http.StatusConflict {
		t.Fatalf("duplicate = %d", dup.Code)
	}

	rec = do(http.MethodPost, "/api/v1/observability/exports/prod/test", "")
	var ok obsTestResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &ok); err != nil || !ok.OK || len(ok.Results) != 2 {
		t.Fatalf("test = %d %s", rec.Code, rec.Body.String())
	}
	recv.mu.Lock()
	gotPaths, gotAuth := strings.Join(recv.paths, ","), recv.auth
	recv.code = http.StatusUnauthorized
	recv.mu.Unlock()
	if !strings.Contains(gotPaths, "/v1/logs") || !strings.Contains(gotPaths, "/v1/metrics") || gotAuth[0] != hdrValue {
		t.Fatalf("receiver saw %s %v", gotPaths, gotAuth)
	}

	rec = do(http.MethodPost, "/api/v1/observability/exports/prod/test", "")
	var bad obsTestResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &bad); err != nil || bad.OK || bad.Results[0].Status != 401 || bad.Results[0].Hint == "" || bad.Results[0].Fix != "bad_token" {
		t.Fatalf("401 test = %d %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "zz-leakcheck") {
		t.Fatal("test response leaked the credential")
	}

	rec = do(http.MethodGet, "/api/v1/observability/exports", "")
	if rec.Code != http.StatusOK || strings.Contains(rec.Body.String(), "zz-leakcheck") || !strings.Contains(rec.Body.String(), `"id":"prod"`) {
		t.Fatalf("list = %d %s", rec.Code, rec.Body.String())
	}

	if rec = do(http.MethodDelete, "/api/v1/observability/exports/prod", ""); rec.Code != http.StatusNoContent {
		t.Fatalf("delete = %d", rec.Code)
	}
	if rec = do(http.MethodPost, "/api/v1/observability/exports/prod/test", ""); rec.Code != http.StatusNotFound {
		t.Fatalf("test after delete = %d", rec.Code)
	}
}

func TestObservabilityExports_RejectsBadInput(t *testing.T) {
	t.Parallel()
	rt, _, cookie := newExportRouter(t)
	for name, body := range map[string]string{
		"unknown preset":   `{"preset":"nope","name":"x","fields":{}}`,
		"bad name":         `{"preset":"otlp-http","name":"Bad Name","fields":{"endpoint":"https://203.0.113.9"}}`,
		"missing field":    `{"preset":"grafana-cloud","name":"x","fields":{"endpoint":"https://203.0.113.9"}}`,
		"private endpoint": `{"preset":"otlp-http","name":"x","fields":{"endpoint":"http://127.0.0.1:9"}}`,
		"bad signal":       `{"preset":"better-stack","name":"x","fields":{"source_token":"t"},"signals":["metrics"]}`,
	} {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/observability/exports", body))
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%s: status = %d body = %s", name, rec.Code, rec.Body.String())
		}
	}
}
