package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRun_RegistryCredentialsHubStatusAndLimits(t *testing.T) {
	var gotPath, gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotQuery = r.URL.Path, r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/limits") {
			_, _ = w.Write([]byte(`{"authenticated":true,"known":true,"limit":200,"remaining":150,"window_seconds":21600,"source":"1.2.3.4"}`))
			return
		}
		_, _ = w.Write([]byte(`{"valid":true,"kind":"personal","namespace":"me","scopes":["repo:read"],"capabilities":[],"limits":null,"checked_at":"2026-01-01T00:00:00Z"}`))
	}))
	defer srv.Close()

	out, _ := runCLIExpectOK(t, []string{"registry-credentials", "status", "rc1", "--refresh", "--api-url", srv.URL})
	if gotPath != "/api/v1/registry-credentials/rc1/dockerhub" || gotQuery != "refresh=true" || !strings.Contains(out, "valid") {
		t.Errorf("status path=%q query=%q out=%q", gotPath, gotQuery, out)
	}
	out, _ = runCLIExpectOK(t, []string{"registry-credentials", "limits", "rc1", "--api-url", srv.URL})
	if gotPath != "/api/v1/registry-credentials/rc1/limits" || !strings.Contains(out, "150 of 200") {
		t.Errorf("limits path=%q out=%q", gotPath, out)
	}
}
