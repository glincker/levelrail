package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/secrets"
)

func TestRunWrapMasterKeyRoundTrip(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var in map[string]string
		_ = json.NewDecoder(r.Body).Decode(&in)
		if strings.Contains(r.URL.Path, "/encrypt/") {
			_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]string{"ciphertext": "vault:v1:" + in["plaintext"]}})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]string{"plaintext": strings.TrimPrefix(in["ciphertext"], "vault:v1:")}})
	}))
	defer srv.Close()
	env := map[string]string{envKMSProvider: "vault-transit", envVaultAddr: srv.URL, envVaultToken: "t", envVaultTransitKey: "k"}
	getenv := func(k string) string { return env[k] }

	dir := t.TempDir()
	mk, err := secrets.GenerateMasterKey()
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	path := filepath.Join(dir, masterKeyFilename)
	if err := os.WriteFile(path, []byte(mk.String()), 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}

	var out bytes.Buffer
	if err := runWrapMasterKey(context.Background(), nil, dir, &out, getenv); err != nil {
		t.Fatalf("wrap: %v", err)
	}
	got, _ := os.ReadFile(path) //nolint:gosec // t.TempDir path
	if !secrets.IsWrappedMasterKey(string(got)) || strings.Contains(string(got), mk.String()) {
		t.Fatalf("file must hold the wrapped blob only: %q", got)
	}
	plain, err := openMasterKeyMaterial(context.Background(), string(got), getenv)
	if err != nil || plain != mk.String() {
		t.Fatalf("open = %q err=%v", plain, err)
	}
	if err := runWrapMasterKey(context.Background(), nil, dir, &out, getenv); err == nil {
		t.Fatal("wrapping twice must be refused")
	}
	if _, err := openMasterKeyMaterial(context.Background(), string(got), func(string) string { return "" }); err == nil {
		t.Fatal("a wrapped key with no key service configured must fail closed")
	}
	if err := runWrapMasterKey(context.Background(), []string{"--unwrap"}, dir, &out, getenv); err != nil {
		t.Fatalf("unwrap: %v", err)
	}
	back, _ := os.ReadFile(path) //nolint:gosec // t.TempDir path
	if string(back) != mk.String() {
		t.Fatalf("unwrap must restore the plain key")
	}
}
