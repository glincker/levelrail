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

func writeJSONResp(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func onboardingServer(t *testing.T, ob apiclient.NodeOnboarding, listCalls *atomic.Int32, gotJoin *apiclient.CreateNodeJoinTokenRequest) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/nodes/join-tokens":
			if gotJoin != nil {
				_ = json.NewDecoder(r.Body).Decode(gotJoin)
			}
			writeJSONResp(w, http.StatusCreated, apiclient.CreateNodeJoinTokenResponse{Token: "tok", JoinCommands: &apiclient.JoinCommands{Docker: "docker run -e APP_NODE_ZONE=z", Binary: "sudo agent"}})
		case r.URL.Path == "/api/v1/nodes":
			n := listCalls.Add(1)
			if n < 2 {
				writeJSONResp(w, http.StatusOK, []apiclient.NodeResource{})
				return
			}
			writeJSONResp(w, http.StatusOK, []apiclient.NodeResource{{ID: "nd_1", Name: "edge-1", Status: "online"}})
		case r.URL.Path == "/api/v1/nodes/nd_1/onboarding":
			writeJSONResp(w, http.StatusOK, ob)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func sampleOnboarding(fail bool) apiclient.NodeOnboarding {
	now := time.Now()
	ob := apiclient.NodeOnboarding{
		NodeID: "nd_1", Stage: "ready", Reported: true, ReportedAt: &now,
		Detected: &apiclient.NodeDetectedDomain{Provider: "hetzner", Region: "eu-central", Zone: "fsn1-dc14", Source: "metadata"},
		Domain:   apiclient.NodeDetectedDomain{Provider: "hetzner", Region: "eu-central", Zone: "fsn1-dc14"},
		Checks: []apiclient.NodePreflightCheck{
			{ID: "docker", Status: "pass", Detail: "Docker 27 answered"},
			{ID: "firewall", Status: "unknown", Detail: "cannot be proven from the node", Fix: "Allow inbound UDP 51820"},
		},
		Summary: apiclient.NodePreflightSummary{Pass: 1, Unknown: 1, Ready: true},
	}
	if fail {
		ob.Stage = "needs_attention"
		ob.Checks = append(ob.Checks, apiclient.NodePreflightCheck{ID: "clock", Status: "fail", Detail: "clock is 3m off", Fix: "Enable time sync", FixCommand: "sudo timedatectl set-ntp true"})
		ob.Summary = apiclient.NodePreflightSummary{Pass: 1, Fail: 1, Unknown: 1}
	}
	return ob
}

func TestRun_NodesAddPreflightPassesAndSendsDomain(t *testing.T) {
	var calls atomic.Int32
	var join apiclient.CreateNodeJoinTokenRequest
	srv := onboardingServer(t, sampleOnboarding(false), &calls, &join)
	stdout, stderr := runCLIExpectOK(t, []string{"nodes", "add", "--name", "edge-1", "--provider", "hetzner", "--region", "eu-central", "--zone", "fsn1-dc14",
		"--preflight", "--poll-interval", "10ms", "--timeout", "5s", "--api-url", srv.URL})
	if join.Provider != "hetzner" || join.Region != "eu-central" || join.Zone != "fsn1-dc14" {
		t.Errorf("join request = %+v", join)
	}
	for _, want := range []string{"docker run", "docker", "PASS", "UNKNOWN", "Allow inbound UDP 51820", "provider=hetzner"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("stdout lacks %q:\n%s", want, stdout)
		}
	}
	if !strings.Contains(stderr, "waiting for the agent") {
		t.Errorf("stderr should narrate progress, got %q", stderr)
	}
}

func TestRun_NodesAddPreflightExitsNonZeroWithFix(t *testing.T) {
	var calls atomic.Int32
	srv := onboardingServer(t, sampleOnboarding(true), &calls, nil)
	var out, errb bytes.Buffer
	code := run("levelrail-cli-test", []string{"nodes", "add", "--name", "edge-1", "--preflight", "--poll-interval", "10ms", "--timeout", "5s", "--api-url", srv.URL}, &out, &errb, envMap())
	if code != exitAPIError {
		t.Fatalf("exit = %d, want %d", code, exitAPIError)
	}
	if !strings.Contains(out.String(), "Enable time sync") || !strings.Contains(out.String(), "sudo timedatectl set-ntp true") {
		t.Errorf("failing check must print its fix:\n%s", out.String())
	}
}

func TestRun_NodesAddPreflightNeedsAName(t *testing.T) {
	var out, errb bytes.Buffer
	if code := run("levelrail-cli-test", []string{"nodes", "add", "--preflight", "--api-url", "http://127.0.0.1:1"}, &out, &errb, envMap()); code != exitUsage {
		t.Errorf("exit = %d, want usage", code)
	}
}

func TestRun_NodesAddPreflightTimesOut(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			writeJSONResp(w, http.StatusCreated, apiclient.CreateNodeJoinTokenResponse{Token: "tok"})
			return
		}
		writeJSONResp(w, http.StatusOK, []apiclient.NodeResource{})
	}))
	defer srv.Close()
	var out, errb bytes.Buffer
	code := run("levelrail-cli-test", []string{"nodes", "add", "--name", "never", "--preflight", "--poll-interval", "10ms", "--timeout", "100ms", "--api-url", srv.URL}, &out, &errb, envMap())
	if code == exitOK || !strings.Contains(errb.String(), "did not finish onboarding") {
		t.Errorf("exit = %d, stderr = %q", code, errb.String())
	}
}

func TestRun_NodesPreflightByName(t *testing.T) {
	var calls atomic.Int32
	calls.Store(5)
	srv := onboardingServer(t, sampleOnboarding(false), &calls, nil)
	stdout, _ := runCLIExpectOK(t, []string{"nodes", "preflight", "edge-1", "--api-url", srv.URL})
	if !strings.Contains(stdout, "1 passed") || !strings.Contains(stdout, "1 unverified") {
		t.Errorf("stdout = %q", stdout)
	}
	jsonOut, _ := runCLIExpectOK(t, []string{"nodes", "preflight", "nd_1", "--json", "--api-url", srv.URL})
	var got apiclient.NodeOnboarding
	if err := json.Unmarshal([]byte(jsonOut), &got); err != nil || got.Stage != "ready" {
		t.Errorf("json = %q err = %v", jsonOut, err)
	}
}

func TestRun_NodesDomains(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/nodes/domains" {
			http.NotFound(w, r)
			return
		}
		writeJSONResp(w, http.StatusOK, apiclient.NodeDomains{
			Providers: []apiclient.NodeDomainGroup{{Name: "aws", Nodes: 2, Online: 2, Workloads: 3, CPUMilli: 8000, MemoryBytes: 16 << 30}},
			Zones:     []apiclient.NodeDomainGroup{{Name: "us-east-1/us-east-1a", Nodes: 1}},
			Unlabeled: 1, Total: 3,
			Risks: []apiclient.NodeDomainRisk{{Level: "warn", Message: "all 3 replicas of web run in zone us-east-1a"}},
		})
	}))
	defer srv.Close()
	stdout, _ := runCLIExpectOK(t, []string{"nodes", "domains", "--api-url", srv.URL})
	for _, want := range []string{"PROVIDER", "aws", "8.0 cores", "16.0 GiB", "ZONE", "1 of 3 nodes have no", "WARN: all 3 replicas of web"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("stdout lacks %q:\n%s", want, stdout)
		}
	}
}
