package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestAuthRotateRewritesProfileAtomically(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	exp := time.Now().Add(30 * 24 * time.Hour).UTC().Truncate(time.Second)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/auth/whoami", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer old-secret" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"kind": "token", "name": "ci", "token_id": "tok_1", "abilities": []string{"write"}})
	})
	mux.HandleFunc("POST /api/v1/auth/tokens/tok_1/rotate", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{"id": "tok_2", "name": "ci", "token": "new-secret", "expires_at": exp, "replaces": "tok_1"})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	if err := writeCredentialsFile("levelrail-cli", "default", credentials{APIURL: srv.URL, Token: "old-secret", TrustedDevice: "dev-9"}); err != nil {
		t.Fatal(err)
	}
	lookup := func(string) (string, bool) { return "", false }
	var out, errOut bytes.Buffer
	if code := runAuthRotate("levelrail-cli", []string{"--json"}, &out, &errOut, lookup); code != exitOK {
		t.Fatalf("exit = %d, stderr = %s", code, errOut.String())
	}
	creds, err := readCredentialsFile("levelrail-cli", "default")
	if err != nil {
		t.Fatal(err)
	}
	if creds.Token != "new-secret" || creds.TrustedDevice != "dev-9" || creds.TokenExpiresAt != exp.Format(time.RFC3339) {
		t.Fatalf("credentials after rotate = %+v", creds)
	}
	if strings.Contains(out.String(), "new-secret") {
		t.Fatalf("the secret must not be printed when the profile was updated: %s", out.String())
	}
	path := filepath.Join(home, ".config", "levelrail-cli", "credentials")
	entries, _ := os.ReadDir(filepath.Dir(path))
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".credentials-") {
			t.Fatalf("temp file left behind: %s", e.Name())
		}
	}
	if st, err := os.Stat(path); err != nil || st.Mode().Perm() != 0o600 {
		t.Fatalf("credentials file mode = %v, err = %v, want 0600", st.Mode().Perm(), err)
	}
}

func TestPickRotationTarget(t *testing.T) {
	revoked := time.Now()
	toks := []tokenResource{{ID: "a", Name: "ci"}, {ID: "b", Name: "dup"}, {ID: "c", Name: "dup"}, {ID: "d", Name: "gone", RevokedAt: &revoked}}
	if id, err := pickRotationTarget(toks, "ci"); err != nil || id != "a" {
		t.Fatalf("by name = %q, %v", id, err)
	}
	if id, err := pickRotationTarget(toks, "b"); err != nil || id != "b" {
		t.Fatalf("by id = %q, %v", id, err)
	}
	for _, k := range []string{"dup", "gone", "nope"} {
		if _, err := pickRotationTarget(toks, k); err == nil {
			t.Errorf("%q should not resolve", k)
		}
	}
}
