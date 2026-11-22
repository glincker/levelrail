package macosrt

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func baseSpec() Spec {
	return Spec{Role: RoleAgent, Binary: "/usr/local/bin/levelrail-agent", LogDir: "/tmp/lr-logs"}
}

func TestValidate_RejectsSecretArgs(t *testing.T) {
	for _, args := range [][]string{
		{"--join-token=abc"},
		{"--token", "abc"},
		{"--server", "https://user:pw@cp.example.com"},
		{"--server", "https://user@cp.example.com"},
	} {
		s := baseSpec()
		s.Args = args
		if err := s.Validate(); err == nil {
			t.Errorf("args %v accepted", args)
		}
	}
	s := baseSpec()
	s.Args = []string{"--server", "https://cp.example.com", "--token-file", "--verbose"}
	if err := s.Validate(); err != nil {
		t.Errorf("clean args refused: %v", err)
	}
}

func TestInstall_WritesPlistOwnerOnly(t *testing.T) {
	dir := t.TempDir()
	m := Manager{Dir: dir, UID: 501, Runner: nopRunner{}}
	s := baseSpec()
	s.LogDir = filepath.Join(dir, "logs")
	p, err := m.Install(t.Context(), s)
	if err != nil {
		t.Fatal(err)
	}
	st, err := os.Stat(p)
	if err != nil || st.Mode().Perm() != 0o600 {
		t.Errorf("plist mode = %v err %v, want 0600", st.Mode().Perm(), err)
	}
}

type nopRunner struct{}

func (nopRunner) Run(_ context.Context, _ ...string) ([]byte, error) { return nil, nil }
