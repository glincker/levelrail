package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func TestRun_SettingsDDNS_SetAndShow(t *testing.T) {
	var method, path string
	var body apiclient.DDNSUpdate
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method, path = r.Method, r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&body)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(apiclient.DDNSResource{Enabled: true, Provider: "duckdns", Source: "stored", TargetConfigured: true, Host: "www.duckdns.org", IntervalSecs: 120})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"settings", "ddns", "set", "--provider", "duckdns", "--domains", "a,b", "--target", "tok", "--interval", "2m", "--api-url", srv.URL})
	if method != http.MethodPut || path != "/api/v1/settings/ddns" {
		t.Errorf("method/path = %s %s", method, path)
	}
	if body.Target == nil || *body.Target != "tok" || body.IntervalSecs == nil || *body.IntervalSecs != 120 || *body.Provider != "duckdns" {
		t.Errorf("body = %+v", body)
	}
	if !strings.Contains(stdout, "source: stored") || !strings.Contains(stdout, "host www.duckdns.org") || strings.Contains(stdout, "tok\n") {
		t.Errorf("stdout = %q", stdout)
	}

	runCLIExpectOK(t, []string{"settings", "ddns", "disable", "--api-url", srv.URL})
	if body.Enabled == nil || *body.Enabled {
		t.Errorf("disable body = %+v", body)
	}
}

func TestBuildDDNSUpdateRejectsBadInput(t *testing.T) {
	for _, f := range []ddnsFlags{{interval: "soon"}, {quorum: "0"}, {allowPrivate: "maybe"}} {
		if _, err := buildDDNSUpdate(f); err == nil {
			t.Errorf("%+v: want error", f)
		}
	}
}

func TestRun_SettingsDDNS_ClearTarget(t *testing.T) {
	var raw string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b := new(strings.Builder)
		_, _ = io.Copy(b, r.Body)
		raw = b.String()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(apiclient.DDNSResource{Provider: "generic", Source: "stored"})
	}))
	defer srv.Close()

	runCLIExpectOK(t, []string{"settings", "ddns", "set", "--clear-target", "--api-url", srv.URL})
	if !strings.Contains(raw, `"target":""`) {
		t.Errorf("clear body = %s", raw)
	}
	raw = ""
	stderr := runCLIExpectValidationError(t, []string{"settings", "ddns", "set", "--target", "", "--api-url", srv.URL})
	if raw != "" || !strings.Contains(stderr, "--clear-target") {
		t.Errorf("empty --target sent=%q stderr=%q", raw, stderr)
	}
}
