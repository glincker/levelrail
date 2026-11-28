package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/glincker/levelrail/internal/apiclient"
)

// scriptedGoLive serves the add call, then each scripted go-live result in turn (the last repeats).
func scriptedGoLive(t *testing.T, timeoutSetting string, script []apiclient.GoLiveResult) *httptest.Server {
	t.Helper()
	var polls atomic.Int32
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasPrefix(r.URL.Path, "/api/v1/settings/runtime/"):
			if timeoutSetting == "" {
				http.NotFound(w, r)
				return
			}
			_ = json.NewEncoder(w).Encode(apiclient.RuntimeSettingResource{Key: "k", Value: timeoutSetting})
		case r.Method == http.MethodPatch:
			_ = json.NewEncoder(w).Encode(apiclient.EditDomainsResult{App: "web", Domains: []string{"app.example.com"}, Changed: true, GoLive: script[:1]})
		case r.Method == http.MethodGet:
			i := min(int(polls.Add(1)), len(script)-1)
			_ = json.NewEncoder(w).Encode(script[i])
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	}))
}

func goLiveAt(state string, steps ...apiclient.GoLiveStep) apiclient.GoLiveResult {
	return apiclient.GoLiveResult{App: "web", Domain: "app.example.com", State: state, Steps: steps}
}

func runWait(t *testing.T, srv *httptest.Server, extra ...string) (int, string) {
	t.Helper()
	fastGoLivePolling(t)
	args := append([]string{"apps", "domains", "add", "web", "app.example.com", "--wait", "--api-url", srv.URL}, extra...)
	var stdout, stderr bytes.Buffer
	code := run("levelrail-cli-test", args, &stdout, &stderr, envMap())
	return code, stdout.String()
}

func TestWait_PendingShowsRecordThenIssued(t *testing.T) {
	record := &apiclient.DNSRecordResource{Type: "A", Name: "app.example.com", Value: "203.0.113.7"}
	srv := scriptedGoLive(t, "", []apiclient.GoLiveResult{
		goLiveAt("pending", apiclient.GoLiveStep{ID: "dns", State: "pending", Detail: "no record yet", Record: record}),
		goLiveAt("pending", apiclient.GoLiveStep{ID: "dns", State: "done"}, apiclient.GoLiveStep{ID: "certificate", State: "pending", Detail: "issuing"}),
		goLiveAt("live", apiclient.GoLiveStep{ID: "dns", State: "done"}, apiclient.GoLiveStep{ID: "certificate", State: "done"}),
	})
	defer srv.Close()
	code, out := runWait(t, srv)
	if code != exitOK {
		t.Fatalf("exit = %d, out=%q", code, out)
	}
	for _, want := range []string{"create this record: A  app.example.com  203.0.113.7", "https://app.example.com OK"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func TestWait_WrongTargetTimesOutWithRecordAndNextStep(t *testing.T) {
	record := &apiclient.DNSRecordResource{Type: "CNAME", Name: "app.example.com", Value: "host.example.net"}
	srv := scriptedGoLive(t, "1ms", []apiclient.GoLiveResult{
		goLiveAt("pending", apiclient.GoLiveStep{ID: "propagation", State: "pending", Detail: "points at 198.51.100.9, expected 203.0.113.7", Record: record}),
	})
	defer srv.Close()
	code, out := runWait(t, srv)
	if code != exitAPIError {
		t.Fatalf("exit = %d, want %d", code, exitAPIError)
	}
	for _, want := range []string{"not live after 1ms", "propagation", "create CNAME app.example.com -> host.example.net"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func TestWait_CDNProxiedSuggestsDNSOnly(t *testing.T) {
	srv := scriptedGoLive(t, "", []apiclient.GoLiveResult{
		goLiveAt("failed", apiclient.GoLiveStep{ID: "certificate", State: "failed", Detail: "domain is proxied by a CDN"}),
	})
	defer srv.Close()
	code, out := runWait(t, srv, "--timeout", "5s")
	if code != exitAPIError {
		t.Fatalf("exit = %d, want %d", code, exitAPIError)
	}
	if !strings.Contains(out, "stopped: certificate") || !strings.Contains(out, "DNS only") {
		t.Errorf("want the blocker and the DNS only step:\n%s", out)
	}
}

func TestResolveWaitOptions_FlagBeatsSettingBeatsFallback(t *testing.T) {
	srv := scriptedGoLive(t, "7m", nil)
	defer srv.Close()
	client := apiClientFromFlags("p", srv.URL, "t", "", envMap())
	got := resolveWaitOptions(t.Context(), client, "p", 0, 0)
	if got.timeout.String() != "7m0s" || got.poll.String() != "7m0s" {
		t.Errorf("settings not used: %+v", got)
	}
	if got := resolveWaitOptions(t.Context(), client, "p", 30e9, 2e9); got.timeout.Seconds() != 30 || got.poll.Seconds() != 2 {
		t.Errorf("flags must win: %+v", got)
	}
	bad := scriptedGoLive(t, "", nil)
	defer bad.Close()
	c2 := apiClientFromFlags("p", bad.URL, "t", "", envMap())
	if got := resolveWaitOptions(t.Context(), c2, "p", 0, 0); got.timeout != fallbackGoLiveWait || got.poll != fallbackGoLivePoll {
		t.Errorf("fallback not used: %+v", got)
	}
}
