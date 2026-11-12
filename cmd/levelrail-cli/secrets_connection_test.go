package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func connServer(t *testing.T, body *string) *httptest.Server {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		*body = string(b)
		_ = json.NewEncoder(w).Encode(map[string]any{"name": "team", "type": "doppler", "allowed_prefixes": []string{"team-a"}, "allowed_envs": []string{"prod"}})
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestRun_SecretsConnectionAllowFlagsSendLists(t *testing.T) {
	var body string
	srv := connServer(t, &body)
	out, _ := runCLIExpectOK(t, []string{"secrets", "connection", "update", "team", "--allow-prefix", "team-a", "--allow-prefix", "team-b", "--allow-env", "prod", "--api-url", srv.URL})
	var got map[string][]string
	_ = json.Unmarshal([]byte(body), &got)
	if len(got["allowed_prefixes"]) != 2 || len(got["allowed_envs"]) != 1 {
		t.Fatalf("request body = %s", body)
	}
	if !strings.Contains(out, "allow_prefixes=team-a") || !strings.Contains(out, "allow_envs=prod") {
		t.Fatalf("output lacks the lists:\n%s", out)
	}
}

func TestRun_SecretsConnectionClearSendsEmptyListsAndOmittedKeepsThem(t *testing.T) {
	var body string
	srv := connServer(t, &body)
	runCLIExpectOK(t, []string{"secrets", "connection", "update", "team", "--clear-allow-lists", "--api-url", srv.URL})
	if !strings.Contains(body, `"allowed_prefixes":[]`) || !strings.Contains(body, `"allowed_envs":[]`) {
		t.Fatalf("clear body = %s", body)
	}
	runCLIExpectOK(t, []string{"secrets", "connection", "update", "team", "--project", "p", "--credential-ref", "ns/k", "--api-url", srv.URL})
	if strings.Contains(body, "allowed_") {
		t.Fatalf("omitted flags must not send the lists: %s", body)
	}
}

func TestPrintAppHumanShowsRequireRefWarnings(t *testing.T) {
	var sb strings.Builder
	printAppHuman(&sb, appResource{Name: "web", RequireRefWarnings: []string{"STRIPE_KEY holds a literal secret value, but rule x"}})
	if !strings.Contains(sb.String(), "warning:  STRIPE_KEY holds a literal") {
		t.Fatalf("output:\n%s", sb.String())
	}
}
