package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNodesProvisionsOracleKeyFromFileAndStdin(t *testing.T) {
	pemText := "-----BEGIN PRIVATE KEY-----\nabc\n-----END PRIVATE KEY-----\n"
	path := filepath.Join(t.TempDir(), "k.pem")
	if err := os.WriteFile(path, []byte(pemText), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := readOraclePrivateKey(path, strings.NewReader(""))
	if err != nil || !strings.Contains(got, "BEGIN PRIVATE KEY") {
		t.Fatalf("file read: %q %v", got, err)
	}
	got, err = readOraclePrivateKey("-", strings.NewReader(pemText))
	if err != nil || got == "" {
		t.Fatalf("stdin read: %q %v", got, err)
	}
	if _, err = readOraclePrivateKey("", strings.NewReader("")); err == nil {
		t.Fatal("empty key source must be refused")
	}
	if _, err = readOraclePrivateKey("-", strings.NewReader("not a key")); err == nil {
		t.Fatal("non PEM input must be refused")
	}
}
