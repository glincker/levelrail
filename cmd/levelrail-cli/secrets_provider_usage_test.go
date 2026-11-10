package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSecretsProviderUsageListsOnePassword(t *testing.T) {
	u := secretsProviderUsage("levelrail-cli")
	for _, want := range []string{"--type onepassword", "--base-url CONNECT_URL", "op://NAME/"} {
		if !strings.Contains(u, want) {
			t.Fatalf("usage missing %q", want)
		}
	}
}

func TestSecretsProviderAddInfisicalIdentityPair(t *testing.T) {
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&got)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"name":"infra","type":"infisical","project":"w","environment":"prod","has_token":true}`))
	}))
	defer srv.Close()
	secretPath := filepath.Join(t.TempDir(), "cs")
	if err := os.WriteFile(secretPath, []byte("s3cr3t\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	stdout, _ := runCLIExpectOK(t, []string{"secrets", "provider", "add", "infra", "--type", "infisical", "--project", "w", "--environment", "prod", "--client-id", "cid", "--client-secret-file", secretPath, "--api-url", srv.URL})
	if got["token"] != "infisical-identity:cid:s3cr3t" {
		t.Fatalf("token = %v, want the prefixed identity credential", got["token"])
	}
	if strings.Contains(stdout, "s3cr3t") {
		t.Fatalf("stdout must never echo the client secret: %q", stdout)
	}
}

func TestInfisicalIdentityCredentialRejectsBadInput(t *testing.T) {
	for name, c := range map[string][4]string{
		"wrong type":  {"doppler", "id", "f", ""},
		"half a pair": {"infisical", "id", "", ""},
		"both creds":  {"infisical", "id", "f", "tok"},
		"colon in id": {"infisical", "a:b", "f", ""},
	} {
		if _, err := infisicalIdentityCredential(c[0], c[1], c[2], c[3]); err == nil {
			t.Errorf("%s: want an error", name)
		}
	}
}
