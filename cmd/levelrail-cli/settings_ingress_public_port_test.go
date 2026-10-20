package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRun_SettingsIngress_Set_PublicHTTPSPort(t *testing.T) {
	var gotBody ingressSettingsResource
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut {
			_ = json.NewDecoder(r.Body).Decode(&gotBody)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(gotBody)
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{
		"settings", "ingress", "set", "--public-https-port", "443", "--tls-terminated-upstream", "--api-url", srv.URL,
	})

	if gotBody.PublicHTTPSPort != 443 || !gotBody.TLSTerminatedUpstream {
		t.Errorf("request body = %+v, want port 443 and upstream true", gotBody)
	}
	if !strings.Contains(stdout, "public_https_port:  443") {
		t.Errorf("stdout = %q, want public_https_port line", stdout)
	}
}
