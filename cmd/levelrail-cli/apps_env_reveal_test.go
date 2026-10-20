package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

const cliSecretValue = "real-secret-value-0123456789abcd"

// fakeRevealServer answers GET app, the secrets list and the reveal routes,
// asking for a TOTP code the first time a password arrives alone.
func fakeRevealServer(t *testing.T) (*httptest.Server, *[]string) {
	t.Helper()
	var mu sync.Mutex
	var bodies []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		bodies = append(bodies, r.Method+" "+r.URL.Path+" "+string(body))
		mu.Unlock()
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/apps/web":
			_ = json.NewEncoder(w).Encode(map[string]any{"name": "web", "image": "img", "port": 80,
				"env": map[string]string{"PORT": "80", "API_SECRET": apiclient.MaskedEnvValue}, "secret_env": []string{"DB_PASSWORD"}})
		case r.URL.Path == "/api/v1/apps/web/secrets":
			_, _ = w.Write([]byte(`[{"key":"DB_PASSWORD"}]`))
		case r.URL.Path == "/api/v1/apps/web/env/reveal" || r.URL.Path == "/api/v1/apps/web/env/export":
			var req struct {
				StepUp *apiclient.StepUpProof `json:"step_up"`
			}
			_ = json.Unmarshal(body, &req)
			if req.StepUp == nil || req.StepUp.Password != "pw" {
				w.WriteHeader(http.StatusForbidden)
				_, _ = w.Write([]byte(`{"error":"bad","code":"step_up_invalid"}`))
				return
			}
			if req.StepUp.Code != "123456" {
				w.WriteHeader(http.StatusForbidden)
				_, _ = w.Write([]byte(`{"error":"code","code":"step_up_code_required"}`))
				return
			}
			if r.URL.Path == "/api/v1/apps/web/env/export" {
				w.Header().Set("Content-Type", "text/plain")
				_, _ = w.Write([]byte("API_SECRET=" + cliSecretValue + "\n"))
				return
			}
			_ = json.NewEncoder(w).Encode(apiclient.RevealResult{Key: "API_SECRET", Value: cliSecretValue, Type: "plain"})
		default:
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":"not found"}`))
		}
	}))
	t.Cleanup(srv.Close)
	return srv, &bodies
}

func stubPrompts(t *testing.T, answers ...string) *[]string {
	t.Helper()
	var asked []string
	prev := promptHidden
	promptHidden = func(_ io.Writer, label string) (string, error) {
		asked = append(asked, label)
		a := answers[0]
		answers = answers[1:]
		return a, nil
	}
	t.Cleanup(func() { promptHidden = prev })
	return &asked
}

func TestAppsEnvGet_MaskedWithoutReveal(t *testing.T) {
	srv, bodies := fakeRevealServer(t)
	stubPrompts(t)
	stdout, _ := runCLIExpectOK(t, []string{"apps", "env", "get", "web", "API_SECRET", "--api-url", srv.URL})
	if strings.Contains(stdout, cliSecretValue) || !strings.Contains(stdout, apiclient.MaskedEnvValue) {
		t.Fatalf("stdout = %q", stdout)
	}
	for _, b := range *bodies {
		if strings.Contains(b, "/reveal") {
			t.Fatalf("reveal called without --reveal: %s", b)
		}
	}
}

func TestAppsEnvGet_RevealPromptsForPasswordThenCode(t *testing.T) {
	srv, bodies := fakeRevealServer(t)
	asked := stubPrompts(t, "pw", "123456")
	stdout, _ := runCLIExpectOK(t, []string{"apps", "env", "get", "web", "API_SECRET", "--reveal", "--api-url", srv.URL})
	if strings.TrimSpace(stdout) != cliSecretValue {
		t.Fatalf("stdout = %q", stdout)
	}
	if len(*asked) != 2 || !strings.Contains((*asked)[1], "Authenticator") {
		t.Fatalf("prompts = %v", *asked)
	}
	for _, b := range *bodies {
		if strings.HasPrefix(b, "GET") && strings.Contains(b, "pw") {
			t.Fatalf("password leaked into a GET: %s", b)
		}
	}
}

func TestAppsEnvExport_RedactedAndRevealed(t *testing.T) {
	srv, _ := fakeRevealServer(t)
	stubPrompts(t)
	stdout, _ := runCLIExpectOK(t, []string{"apps", "env", "export", "web", "--api-url", srv.URL})
	if strings.Contains(stdout, cliSecretValue) || strings.Contains(stdout, apiclient.MaskedEnvValue) || !strings.Contains(stdout, "API_SECRET=\n") {
		t.Fatalf("masked export = %q", stdout)
	}
	stubPrompts(t, "pw", "123456")
	stdout, _ = runCLIExpectOK(t, []string{"apps", "env", "export", "web", "--reveal", "--api-url", srv.URL})
	if !strings.Contains(stdout, "API_SECRET="+cliSecretValue) {
		t.Fatalf("revealed export = %q", stdout)
	}
}

func TestPlanEnvImport_MaskedValueKeepsStoredValue(t *testing.T) {
	current := map[string]string{"API_SECRET": apiclient.MaskedEnvValue, "PORT": "80"}
	entries := []envFileEntry{{Key: "API_SECRET", Value: ""}, {Key: "PORT", Value: "81"}}
	plan := planEnvImport(current, entries, nil, false)
	if plan.Merged["API_SECRET"] != apiclient.MaskedEnvValue || plan.Merged["PORT"] != "81" {
		t.Fatalf("merged = %v", plan.Merged)
	}
	entries = []envFileEntry{{Key: "API_SECRET", Value: "new-value"}}
	if plan = planEnvImport(current, entries, nil, false); plan.Merged["API_SECRET"] != "new-value" {
		t.Fatalf("explicit new value should replace: %v", plan.Merged)
	}
}
