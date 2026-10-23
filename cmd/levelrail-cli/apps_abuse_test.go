package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const abuseGetJSON = `{"app":"web","configured":false,"policy":{"mode":"off"},
"condition":{"status":"False","reason":"Off","message":"No policy is set."},
"suggestion":{"peak_rps":40,"rps":20,"burst":40,"from_traffic":true},
"counters":{"allowed":5,"would_block":2,"blocked":0,"fail_open":0,"by_reason":[]},"counters_window_minutes":15,"limits":{}}`

func abuseServer(t *testing.T, saved string, put *string, dry *string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/apps/web/abuse":
			_, _ = w.Write([]byte(saved))
		case r.Method == http.MethodPut:
			*put = string(body)
			_, _ = w.Write([]byte(abuseGetJSON))
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/abuse/dry-run"):
			*dry = string(body)
			_, _ = w.Write([]byte(`{"requests":42,"would_block":7,"percent":16.7,"by_reason":[{"reason":"rate_limit","count":7}],
"top_clients":[{"client":"192.0.2.50","requests":30,"would_block":7,"top_reason":"rate_limit"}],"window_minutes":15,"covered_minutes":9.5,"note":"Preview only."}`))
		default:
			http.NotFound(w, r)
		}
	}))
}

func TestRun_AppsAbuseGetShowsStatusAndSuggestion(t *testing.T) {
	var put, dry string
	srv := abuseServer(t, abuseGetJSON, &put, &dry)
	defer srv.Close()
	out, _ := runCLIExpectOK(t, []string{"apps", "abuse", "get", "web", "--api-url", srv.URL})
	for _, want := range []string{"mode: off", "status: Off", "suggested start: 20 requests/s, burst 40"} {
		if !strings.Contains(out, want) {
			t.Fatalf("output lacks %q:\n%s", want, out)
		}
	}
}

func TestRun_AppsAbuseSetPrefillsFromTheSuggestion(t *testing.T) {
	var put, dry string
	srv := abuseServer(t, abuseGetJSON, &put, &dry)
	defer srv.Close()
	runCLIExpectOK(t, []string{"apps", "abuse", "set", "web", "--mode", "log_only", "--deny-cidr", "203.0.113.0/24,198.51.100.7", "--require-user-agent", "--api-url", srv.URL})
	var p map[string]any
	if err := json.Unmarshal([]byte(put), &p); err != nil {
		t.Fatalf("PUT body %q: %v", put, err)
	}
	if p["mode"] != "log_only" || p["rps"] != float64(20) || p["burst"] != float64(40) || p["require_user_agent"] != true {
		t.Fatalf("PUT body = %v, want the suggested rate prefilled", p)
	}
	if cidrs, _ := p["deny_cidrs"].([]any); len(cidrs) != 2 {
		t.Fatalf("deny_cidrs = %v", p["deny_cidrs"])
	}
}

func TestRun_AppsAbuseSetChangesOnlyItsOwnField(t *testing.T) {
	saved := `{"app":"web","configured":true,"policy":{"mode":"enforce","rps":5,"burst":9,"deny_user_agents":["badbot"]},"condition":{},"suggestion":{},"counters":{},"limits":{}}`
	var put, dry string
	srv := abuseServer(t, saved, &put, &dry)
	defer srv.Close()
	runCLIExpectOK(t, []string{"apps", "abuse", "set", "web", "--rps", "8", "--api-url", srv.URL})
	var p map[string]any
	_ = json.Unmarshal([]byte(put), &p)
	if p["mode"] != "enforce" || p["rps"] != float64(8) || p["burst"] != nil {
		t.Fatalf("PUT body = %v", p)
	}
	if agents, _ := p["deny_user_agents"].([]any); len(agents) != 1 {
		t.Fatalf("an untouched list was dropped: %v", p)
	}
	runCLIExpectOK(t, []string{"apps", "abuse", "set", "web", "--deny-agent", "none", "--api-url", srv.URL})
	p = map[string]any{}
	_ = json.Unmarshal([]byte(put), &p)
	if _, has := p["deny_user_agents"]; has {
		t.Fatalf("--deny-agent none did not clear the list: %v", p)
	}
}

func TestRun_AppsAbuseDryRunSendsTheDraftAndPrintsClients(t *testing.T) {
	var put, dry string
	srv := abuseServer(t, abuseGetJSON, &put, &dry)
	defer srv.Close()
	out, _ := runCLIExpectOK(t, []string{"apps", "abuse", "dry-run", "web", "--rps", "3", "--mode", "enforce", "--minutes", "10", "--api-url", srv.URL})
	var req map[string]any
	if err := json.Unmarshal([]byte(dry), &req); err != nil || req["minutes"] != float64(10) {
		t.Fatalf("dry-run body = %q (%v)", dry, err)
	}
	pol, _ := req["policy"].(map[string]any)
	if pol["rps"] != float64(3) || pol["mode"] != "enforce" {
		t.Fatalf("draft = %v", pol)
	}
	for _, want := range []string{"would have blocked 7 (16.7%)", "192.0.2.50", "rate_limit: 7"} {
		if !strings.Contains(out, want) {
			t.Fatalf("output lacks %q:\n%s", want, out)
		}
	}
	if put != "" {
		t.Fatal("dry-run saved a policy")
	}
}

func TestRun_AppsAbuseUsageErrors(t *testing.T) {
	var stdout, stderr strings.Builder
	if code := run("levelrail-cli-test", []string{"apps", "abuse"}, &stdout, &stderr, envMap()); code != exitUsage {
		t.Fatalf("no verb exit = %d", code)
	}
	stderr.Reset()
	if code := run("levelrail-cli-test", []string{"apps", "abuse", "nope"}, &stdout, &stderr, envMap()); code != exitUsage {
		t.Fatalf("bad verb exit = %d", code)
	}
}
