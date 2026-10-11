package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func fastGoLivePolling(t *testing.T) {
	t.Helper()
	oldSleep, oldInterval := goLiveSleep, goLivePollInterval
	goLiveSleep = func(time.Duration) {}
	goLivePollInterval = time.Millisecond
	t.Cleanup(func() { goLiveSleep, goLivePollInterval = oldSleep, oldInterval })
}

// goLiveServer answers PATCH domains, then GET go-live with pending for
// pendingPolls polls before reporting liveState.
func goLiveServer(t *testing.T, pendingPolls int32, liveState string, patchBody *apiclient.EditDomainsRequest) *httptest.Server {
	t.Helper()
	var polls atomic.Int32
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPatch && r.URL.Path == "/api/v1/apps/web/domains":
			if patchBody != nil {
				_ = json.NewDecoder(r.Body).Decode(patchBody)
			}
			_ = json.NewEncoder(w).Encode(apiclient.EditDomainsResult{
				App: "web", Domains: []string{"app.example.com"}, Changed: true,
				DNSResults: []apiclient.DomainDNSResult{{Domain: "app.example.com", DNS: "created", Message: "record created"}},
				GoLive: []apiclient.GoLiveResult{{
					App: "web", Domain: "app.example.com", State: "pending",
					Steps: []apiclient.GoLiveStep{{ID: "dns", State: "done", Detail: "DNS record created at cloudflare"}},
				}},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/apps/web/domains/app.example.com/go-live":
			state := "pending"
			if polls.Add(1) > pendingPolls {
				state = liveState
			}
			_ = json.NewEncoder(w).Encode(apiclient.GoLiveResult{
				App: "web", Domain: "app.example.com", State: state, URL: "https://app.example.com",
				Steps: []apiclient.GoLiveStep{
					{ID: "dns", State: "done"},
					{ID: "propagation", State: map[bool]string{true: "done", false: "pending"}[state == "live"], Detail: "waiting for DNS"},
				},
			})
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	}))
}

func TestRun_AppsDomainsAddWaitReachesLive(t *testing.T) {
	fastGoLivePolling(t)
	var body apiclient.EditDomainsRequest
	srv := goLiveServer(t, 2, "live", &body)
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"apps", "domains", "add", "web", "App.Example.com", "--dns", "auto", "--replace", "--wait", "--api-url", srv.URL}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d, want 0 (stdout=%q stderr=%q)", got, stdout.String(), stderr.String())
	}
	if body.DNS != "auto" || !body.Replace || len(body.Add) != 1 || body.Add[0] != "app.example.com" {
		t.Errorf("request = %+v, want dns=auto replace=true add=[app.example.com]", body)
	}
	out := stdout.String()
	if !strings.Contains(out, "dns app.example.com: created") || !strings.Contains(out, "live at https://app.example.com") {
		t.Errorf("stdout = %q, want the dns result and the live url", out)
	}
}

func TestRun_AppsDomainsAddWaitTimesOutNonZero(t *testing.T) {
	fastGoLivePolling(t)
	srv := goLiveServer(t, 1<<20, "live", nil)
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"apps", "domains", "add", "web", "app.example.com", "--wait", "--timeout", "30ms", "--api-url", srv.URL}, &stdout, &stderr, envMap())
	if got != exitAPIError {
		t.Fatalf("exit = %d, want %d when the domain never goes live", got, exitAPIError)
	}
}

func TestRun_AppsDomainsRemoveSendsRemoveDNS(t *testing.T) {
	var body apiclient.EditDomainsRequest
	srv := goLiveServer(t, 0, "live", &body)
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"apps", "domains", "remove", "web", "app.example.com", "--remove-dns", "--api-url", srv.URL}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d (stderr=%q)", got, stderr.String())
	}
	if !body.RemoveDNS || len(body.Remove) != 1 {
		t.Errorf("request = %+v, want remove_dns=true", body)
	}
}

func TestRun_DomainsGoLiveFailedExitsNonZero(t *testing.T) {
	fastGoLivePolling(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(apiclient.GoLiveResult{
			App: "web", Domain: "app.example.com", State: "failed",
			Steps: []apiclient.GoLiveStep{{ID: "dns", State: "conflict", Detail: "a different record exists"}},
		})
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"domains", "go-live", "web", "app.example.com", "--wait", "--api-url", srv.URL}, &stdout, &stderr, envMap())
	if got != exitAPIError {
		t.Fatalf("exit = %d, want %d", got, exitAPIError)
	}
	if !strings.Contains(stdout.String(), "conflict") {
		t.Errorf("stdout = %q, want the conflict step", stdout.String())
	}
}

func TestRun_SettingsDomainAutomationSetKeepsUnsetKeys(t *testing.T) {
	var sent apiclient.DomainAutomationOverride
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case http.MethodGet:
			_ = json.NewEncoder(w).Encode(apiclient.DomainAutomationResource{
				DomainAutomationPolicy: apiclient.DomainAutomationPolicy{AutoDNS: true, ForceHTTPS: true, WWWPolicy: "off", VerifyAfter: true},
			})
		case http.MethodPut:
			_ = json.NewDecoder(r.Body).Decode(&sent)
			_ = json.NewEncoder(w).Encode(apiclient.DomainAutomationResource{})
		}
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"settings", "domain-automation", "set", "--www-policy", "redirect_to_apex", "--api-url", srv.URL}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d (stderr=%q)", got, stderr.String())
	}
	if sent.WWWPolicy == nil || *sent.WWWPolicy != "redirect_to_apex" {
		t.Errorf("www_policy = %v", sent.WWWPolicy)
	}
	if sent.AutoDNS == nil || !*sent.AutoDNS || sent.ForceHTTPS == nil || !*sent.ForceHTTPS {
		t.Errorf("unset keys must keep their current values: %+v", sent)
	}
}

func TestRun_DomainsBackfillIsDryRunByDefault(t *testing.T) {
	var confirm bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var b struct {
			Confirm bool `json:"confirm"`
		}
		_ = json.NewDecoder(r.Body).Decode(&b)
		confirm = b.Confirm
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(apiclient.BaseDomainBackfill{DryRun: !b.Confirm, BaseDomain: "apps.example.com"})
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"domains", "backfill-base-domain", "--api-url", srv.URL}, &stdout, &stderr, envMap())
	if got != exitOK || confirm || !strings.Contains(stdout.String(), "dry run") {
		t.Fatalf("exit=%d confirm=%v stdout=%q", got, confirm, stdout.String())
	}
}
