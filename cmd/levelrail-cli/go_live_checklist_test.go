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

func checklistFixture(state string, live bool) apiclient.GoLiveChecklist {
	return apiclient.GoLiveChecklist{App: "shop", Domain: "shop.dev", State: state, Live: live, URL: "https://shop.dev", Steps: []apiclient.ChecklistStep{
		{ID: "app", Title: "App is running and healthy", Status: "ok", Summary: "running"},
		{ID: "firewall", Title: "Server firewall allows ports 80 and 443", Status: "fail", Summary: "blocked",
			FirewallFix: &apiclient.ChecklistFirewallFix{FixID: "host.firewall.open_web_ports", Kind: "auto", Commands: []string{"ufw allow 80/tcp"}, Undo: []string{"ufw delete allow 80/tcp"}},
			Primary:     &apiclient.ChecklistAction{Kind: "fix", Label: "Open ports 80 and 443", FixID: "host.firewall.open_web_ports"},
			Provider:    &apiclient.ChecklistProvider{Name: "Hetzner Cloud", DocsURL: "https://docs.hetzner.com/x"}},
		{ID: "live", Title: "https://shop.dev serves this app", Status: "pending", Summary: "waiting", Scope: "control_plane_host"},
	}}
}

func runChecklistCLI(t *testing.T, h http.Handler, args ...string) (int, string, string) {
	t.Helper()
	srv := httptest.NewServer(h)
	defer srv.Close()
	var out, errb bytes.Buffer
	env := func(k string) (string, bool) {
		switch k {
		case envAPIURL:
			return srv.URL, true
		case envAPIToken:
			return "tok", true
		}
		return "", false
	}
	code := runGoLiveChecklist("levelrail-cli", args, &out, &errb, env)
	return code, out.String(), errb.String()
}

func TestGoLiveChecklistPrintsEveryStep(t *testing.T) {
	var gotQuery string
	code, out, _ := runChecklistCLI(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		_ = json.NewEncoder(w).Encode(checklistFixture("needs_attention", false))
	}), "shop", "Shop.dev")
	if code != exitOK || gotQuery != "domain=shop.dev" {
		t.Fatalf("code=%d query=%q", code, gotQuery)
	}
	for _, want := range []string{"[FAIL] Server firewall", "fix (auto): ufw allow 80/tcp", "undo: ufw delete allow 80/tcp", "Hetzner Cloud firewall: https://docs.hetzner.com/x", "cannot prove the internet", "fixes apply host.firewall.open_web_ports"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in:\n%s", want, out)
		}
	}
}

func TestGoLiveChecklistJSON(t *testing.T) {
	code, out, _ := runChecklistCLI(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(checklistFixture("waiting_dns", false))
	}), "--json", "shop")
	var cl apiclient.GoLiveChecklist
	if code != exitOK || json.Unmarshal([]byte(out), &cl) != nil || len(cl.Steps) != 3 {
		t.Fatalf("code=%d out=%s", code, out)
	}
}

func TestGoLiveChecklistWaitExitCodes(t *testing.T) {
	checklistPoll = time.Millisecond
	t.Cleanup(func() { checklistPoll = 5 * time.Second })
	var calls atomic.Int32
	code, out, _ := runChecklistCLI(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if calls.Add(1) < 3 {
			_ = json.NewEncoder(w).Encode(checklistFixture("waiting_certificate", false))
			return
		}
		_ = json.NewEncoder(w).Encode(checklistFixture("live", true))
	}), "--wait", "--timeout", "5s", "shop")
	if code != exitOK || calls.Load() != 3 || !strings.Contains(out, "live: https://shop.dev") {
		t.Fatalf("code=%d calls=%d out=%s", code, calls.Load(), out)
	}
	code, _, _ = runChecklistCLI(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(checklistFixture("waiting_dns", false))
	}), "--wait", "--timeout", "30ms", "shop")
	if code != exitNotLive {
		t.Fatalf("not live within the timeout must exit %d, got %d", exitNotLive, code)
	}
}

func TestGoLiveChecklistUsageAndErrors(t *testing.T) {
	if code, _, errOut := runChecklistCLI(t, http.NotFoundHandler()); code != exitUsage || !strings.Contains(errOut, "requires an app") {
		t.Fatalf("no args: %d %q", code, errOut)
	}
	code, _, _ := runChecklistCLI(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, `{"error":"app not found"}`, http.StatusNotFound)
	}), "missing")
	if code != exitAPIError {
		t.Fatalf("API error exit = %d", code)
	}
}
