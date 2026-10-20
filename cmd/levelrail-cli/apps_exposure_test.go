package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRun_AppsExposure_SetSendsBodyAndPreviewUsesPreviewRoute(t *testing.T) {
	var gotPath, gotMethod string
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotMethod = r.URL.Path, r.Method
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"mode":"direct","container_port":8000,"host_port":24100,"bind_address":"public","changes":[],"urls":[]}`))
	}))
	defer srv.Close()

	runCLIExpectOK(t, []string{"apps", "ports", "web", "--mode", "direct", "--host-port", "24100", "--port", "8000", "--api-url", srv.URL})
	if gotMethod != http.MethodPut || gotPath != "/api/v1/apps/web/exposure" || body["mode"] != "direct" || body["host_port"] != float64(24100) || body["container_port"] != float64(8000) {
		t.Errorf("set: %s %s %v", gotMethod, gotPath, body)
	}
	runCLIExpectOK(t, []string{"apps", "exposure", "web", "--mode", "proxied", "--preview", "--api-url", srv.URL})
	if gotMethod != http.MethodPost || gotPath != "/api/v1/apps/web/exposure/preview" {
		t.Errorf("preview: %s %s", gotMethod, gotPath)
	}
}
