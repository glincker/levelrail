package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRun_DomainsWWWApplyAttach(t *testing.T) {
	var gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(body)
		gotBody = string(body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"domain":"example.com","applied":true,"records":[],"redirect_applied":true,"www_attached":true,"attached_now":true}`))
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"domains", "www-apply", "web", "example.com", "--attach", "--yes", "--api-url", srv.URL})
	if !strings.Contains(gotBody, `"attach":true`) {
		t.Errorf("body = %q, want attach true", gotBody)
	}
	if !strings.Contains(stdout, "attached www host") {
		t.Errorf("stdout = %q", stdout)
	}
}

func TestRun_DomainsWWWPlanShowsFacts(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"domain":"example.com","mode":"both","canonical":"apex","records":[],"www_attached":false,"www_points_here":true}`))
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"domains", "www-plan", "web", "example.com", "--api-url", srv.URL})
	for _, want := range []string{"www attached to app: no", "www DNS points here: yes", "www-apply web example.com --attach --yes"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("stdout missing %q: %q", want, stdout)
		}
	}
}
