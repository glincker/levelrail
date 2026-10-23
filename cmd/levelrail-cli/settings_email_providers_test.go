package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

type providerCall struct {
	method, path string
	body         apiclient.EmailProvider
}

func providerServer(t *testing.T, respond func(c providerCall) (int, any)) (*httptest.Server, *[]providerCall) {
	t.Helper()
	var calls []providerCall
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c := providerCall{method: r.Method, path: r.URL.Path}
		if r.Body != nil {
			_ = json.NewDecoder(r.Body).Decode(&c.body)
		}
		calls = append(calls, c)
		code, out := respond(c)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(code)
		if out != nil {
			_ = json.NewEncoder(w).Encode(out)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, &calls
}

func runProviders(t *testing.T, stdin string, args ...string) (code int, stdout, stderr string) {
	t.Helper()
	var out, errBuf bytes.Buffer
	code = runSettingsEmailProviders("levelrail-cli-test", args, strings.NewReader(stdin), &out, &errBuf, envMap())
	return code, out.String(), errBuf.String()
}

func TestSettingsEmailProviders_AddReadsSecretFromStdinNeverArgv(t *testing.T) {
	enabled := true
	srv, calls := providerServer(t, func(c providerCall) (int, any) {
		return http.StatusCreated, apiclient.EmailProvider{ID: "prov_1", Role: "primary", Backend: c.body.Backend, Enabled: &enabled, Name: c.body.Name, ResendAPIKeySet: c.body.ResendAPIKey != ""}
	})
	code, stdout, stderr := runProviders(t, "re_secret_value\n", "add", "--backend", "resend", "--name", "Resend", "--from", "a@example.com", "--secret-stdin", "--api-url", srv.URL)
	if code != exitOK {
		t.Fatalf("exit = %d, stderr = %q", code, stderr)
	}
	c := (*calls)[0]
	if c.method != http.MethodPost || c.path != "/api/v1/settings/email/providers" {
		t.Errorf("call = %s %s", c.method, c.path)
	}
	if c.body.Backend != "resend" || c.body.ResendFrom != "a@example.com" || c.body.ResendAPIKey != "re_secret_value" {
		t.Errorf("body = %+v, want resend with the stdin credential", c.body)
	}
	if strings.Contains(stdout, "re_secret_value") || strings.Contains(stderr, "re_secret_value") {
		t.Error("credential echoed in output")
	}
	if !strings.Contains(stdout, "prov_1") || !strings.Contains(stdout, "primary") {
		t.Errorf("stdout = %q", stdout)
	}
}

func TestSettingsEmailProviders_AddRequiresBackendAndRejectsSecretFlags(t *testing.T) {
	srv, calls := providerServer(t, func(providerCall) (int, any) { return http.StatusCreated, apiclient.EmailProvider{} })
	if code, _, _ := runProviders(t, "", "add", "--api-url", srv.URL); code != exitValidation {
		t.Errorf("add without backend exit = %d, want %d", code, exitValidation)
	}
	if code, _, _ := runProviders(t, "", "add", "--backend", "smtp", "--smtp-password", "x", "--api-url", srv.URL); code == exitOK {
		t.Error("a credential flag must not exist on this command")
	}
	if code, _, stderr := runProviders(t, "", "add", "--backend", "mailgun", "--from", "a@example.com", "--api-url", srv.URL); code != exitValidation || !strings.Contains(stderr, "shared sender") {
		t.Errorf("--from on mailgun exit = %d, stderr = %q", code, stderr)
	}
	if len(*calls) != 0 {
		t.Errorf("invalid input reached the server: %+v", *calls)
	}
}

func TestSettingsEmailProviders_SetChangesOnlyPassedFlags(t *testing.T) {
	enabled := true
	existing := apiclient.EmailProvider{ID: "prov_b", Role: "backup", Backend: "smtp", Name: "Relay", Enabled: &enabled, SMTPHost: "old.example.com", SMTPPort: 587, SMTPFrom: "b@example.com", SMTPPasswordSet: true}
	srv, calls := providerServer(t, func(c providerCall) (int, any) {
		if c.method == http.MethodGet {
			return http.StatusOK, existing
		}
		return http.StatusOK, c.body
	})
	code, _, stderr := runProviders(t, "", "set", "prov_b", "--smtp-host", "new.example.com", "--enabled=false", "--api-url", srv.URL)
	if code != exitOK {
		t.Fatalf("exit = %d, stderr = %q", code, stderr)
	}
	put := (*calls)[1]
	if put.method != http.MethodPut || put.path != "/api/v1/settings/email/providers/prov_b" {
		t.Fatalf("second call = %s %s", put.method, put.path)
	}
	if put.body.SMTPHost != "new.example.com" || put.body.SMTPPort != 587 || put.body.SMTPFrom != "b@example.com" || put.body.Name != "Relay" {
		t.Errorf("untouched fields were not preserved: %+v", put.body)
	}
	if put.body.Enabled == nil || *put.body.Enabled {
		t.Errorf("enabled = %v, want false", put.body.Enabled)
	}
	if put.body.SMTPPassword != "" {
		t.Errorf("set without a credential flag must send no credential, got %q", put.body.SMTPPassword)
	}
}

func TestSettingsEmailProviders_ListRmTestPromote(t *testing.T) {
	enabled := true
	list := apiclient.EmailProviderList{Providers: []apiclient.EmailProvider{
		{ID: "prov_a", Role: "primary", Backend: "resend", Name: "Resend", Enabled: &enabled, ResendAPIKeySet: true},
		{ID: "prov_b", Role: "backup", Backend: "smtp", Name: "Relay", Enabled: &enabled},
	}}
	srv, calls := providerServer(t, func(c providerCall) (int, any) {
		switch {
		case c.method == http.MethodDelete:
			return http.StatusNoContent, nil
		case strings.HasSuffix(c.path, "/test"):
			return http.StatusOK, apiclient.EmailTestResult{To: "me@example.com", Provider: "smtp", ProviderID: "prov_b", Attempts: 1}
		default:
			return http.StatusOK, list
		}
	})
	code, stdout, _ := runProviders(t, "", "list", "--api-url", srv.URL)
	if code != exitOK || !strings.Contains(stdout, "prov_a") || !strings.Contains(stdout, "backup") || !strings.Contains(stdout, "set") || !strings.Contains(stdout, "missing") {
		t.Errorf("list exit = %d, stdout = %q", code, stdout)
	}
	if code, _, _ := runProviders(t, "", "rm", "prov_b", "--api-url", srv.URL); code != exitOK {
		t.Errorf("rm exit = %d", code)
	}
	if code, out, _ := runProviders(t, "", "test", "prov_b", "--to", "me@example.com", "--api-url", srv.URL); code != exitOK || !strings.Contains(out, "me@example.com") {
		t.Errorf("test exit = %d, out = %q", code, out)
	}
	if code, _, _ := runProviders(t, "", "promote", "prov_b", "--api-url", srv.URL); code != exitOK {
		t.Errorf("promote exit = %d", code)
	}
	var paths []string
	for _, c := range *calls {
		paths = append(paths, c.method+" "+c.path)
	}
	want := []string{
		"GET /api/v1/settings/email/providers",
		"DELETE /api/v1/settings/email/providers/prov_b",
		"POST /api/v1/settings/email/providers/prov_b/test",
		"POST /api/v1/settings/email/providers/prov_b/promote",
	}
	if strings.Join(paths, "|") != strings.Join(want, "|") {
		t.Errorf("calls = %v, want %v", paths, want)
	}
}

func TestSettingsEmailProviders_Usage(t *testing.T) {
	if code, _, _ := runProviders(t, ""); code != exitUsage {
		t.Errorf("no subcommand exit = %d, want %d", code, exitUsage)
	}
	if code, _, _ := runProviders(t, "", "bogus"); code != exitUsage {
		t.Errorf("unknown subcommand exit = %d, want %d", code, exitUsage)
	}
	if code, out, _ := runProviders(t, "", "-h"); code != exitOK || !strings.Contains(out, "promote") {
		t.Errorf("help exit = %d", code)
	}
}

func TestSettingsEmailProviders_RevealRefusesWithoutYesPrint(t *testing.T) {
	srv, calls := providerServer(t, func(providerCall) (int, any) { return http.StatusOK, map[string]string{"value": "x"} })
	prev := promptHidden
	promptHidden = func(io.Writer, string) (string, error) {
		t.Error("prompted before --yes-print was confirmed")
		return "", nil
	}
	t.Cleanup(func() { promptHidden = prev })
	code, stdout, stderr := runProviders(t, "", "reveal", "prov_1", "resend_api_key", "--api-url", srv.URL)
	if code != exitValidation || stdout != "" || !strings.Contains(stderr, "--yes-print") {
		t.Fatalf("exit = %d stdout = %q stderr = %q", code, stdout, stderr)
	}
	if len(*calls) != 0 {
		t.Fatalf("made %d API calls without --yes-print", len(*calls))
	}
}

func TestSettingsEmailProviders_RevealPromptsForPasswordThenCode(t *testing.T) {
	var proofs []apiclient.StepUpProof
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			StepUp apiclient.StepUpProof `json:"step_up"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		proofs = append(proofs, body.StepUp)
		if r.URL.RawQuery != "" {
			t.Error("proof must travel in the body, not the query")
		}
		w.Header().Set("Content-Type", "application/json")
		if body.StepUp.Code == "" {
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"error":"code needed","code":"step_up_code_required"}`))
			return
		}
		_, _ = w.Write([]byte(`{"value":"re_secret_value"}`))
	}))
	t.Cleanup(srv.Close)
	answers := []string{"pw", "123456"}
	prev := promptHidden
	promptHidden = func(io.Writer, string) (string, error) {
		a := answers[0]
		answers = answers[1:]
		return a, nil
	}
	t.Cleanup(func() { promptHidden = prev })
	code, stdout, stderr := runProviders(t, "", "reveal", "prov_1", "resend_api_key", "--yes-print", "--api-url", srv.URL)
	if code != exitOK || strings.TrimSpace(stdout) != "re_secret_value" {
		t.Fatalf("exit = %d stdout = %q stderr = %q", code, stdout, stderr)
	}
	if strings.Contains(stderr, "re_secret_value") {
		t.Errorf("stderr leaked the value: %q", stderr)
	}
	if len(proofs) != 2 || proofs[1].Password != "pw" || proofs[1].Code != "123456" {
		t.Fatalf("proofs = %+v", proofs)
	}
}
