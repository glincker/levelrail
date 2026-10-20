package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func TestRun_SecretsRotateDEK_NeedsAppOrAll(t *testing.T) {
	for _, args := range [][]string{
		{"secrets", "rotate-dek"},
		{"secrets", "rotate-dek", "web", "--all"},
		{"secrets", "rotate-dek", "a", "b"},
	} {
		var stdout, stderr bytes.Buffer
		if got := run("levelrail-cli-test", args, &stdout, &stderr, envMap()); got != exitValidation {
			t.Errorf("%v exit = %d, want %d (stderr=%q)", args, got, exitValidation, stderr.String())
		}
	}
}

func TestRun_SecretsKeyStatus(t *testing.T) {
	var gotPath string
	age := 120
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_ = json.NewEncoder(w).Encode(apiclient.EncryptionKeysStatus{
			MasterKey: apiclient.MasterKeyStatus{Configured: true, AgeDays: &age},
			Apps:      []apiclient.AppKeyStatus{{App: "web", AgeDays: 100, ValueCount: 3, Overdue: true}},
			Policy:    apiclient.KeyPolicy{MaxKeyAgeDays: 90, RemindEveryDays: 7},
			Hints:     []apiclient.EncryptionKeyHint{{Code: "app_key_old", Message: "The data key for web is older than 90 days."}},
		})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"secrets", "key-status", "--api-url", srv.URL})
	if gotPath != "/api/v1/system/encryption-keys" {
		t.Errorf("path = %q", gotPath)
	}
	for _, want := range []string{"master key:  120 days old", "web", "OVERDUE", "hint:"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("stdout %q missing %q", stdout, want)
		}
	}
}

func TestRun_SecretsKeyPolicy(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Errorf("method = %s", r.Method)
		}
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		_ = json.NewEncoder(w).Encode(apiclient.EncryptionKeysStatus{Policy: apiclient.KeyPolicy{MaxKeyAgeDays: 60, RemindEveryDays: 7}})
	}))
	defer srv.Close()

	runCLIExpectOK(t, []string{"secrets", "key-policy", "--max-age-days", "60", "--api-url", srv.URL})
	if gotBody["max_key_age_days"] != float64(60) {
		t.Errorf("body = %v", gotBody)
	}
	if _, set := gotBody["remind_every_days"]; set {
		t.Errorf("unset flag was sent: %v", gotBody)
	}

	var stdout, stderr bytes.Buffer
	if got := run("levelrail-cli-test", []string{"secrets", "key-policy", "--api-url", srv.URL}, &stdout, &stderr, envMap()); got != exitValidation {
		t.Errorf("no flags exit = %d", got)
	}
}
