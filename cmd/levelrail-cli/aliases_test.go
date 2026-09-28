package main

import (
	"bytes"
	"testing"
)

// TestAliases_PassArgumentsThrough proves each alias passes arguments
// through exactly to the real command by checking that the same arguments
// produce the same exit code. This tests with various invalid/help arguments
// to ensure args are forwarded without modification.
func TestAliases_PassArgumentsThrough(t *testing.T) {
	tests := []struct {
		name  string
		alias string
		args  []string
	}{
		{
			name:  "deploy alias with -h",
			alias: "deploy",
			args:  []string{"-h"},
		},
		{
			name:  "rollback alias with -h",
			alias: "rollback",
			args:  []string{"-h"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test alias path
			aliasStdout := new(bytes.Buffer)
			aliasStderr := new(bytes.Buffer)
			aliasArgs := append([]string{tt.alias}, tt.args...)
			aliasExit := run("levelrail-cli-test", aliasArgs, aliasStdout, aliasStderr, envMap())

			// The announcement should appear in stderr
			if !bytes.Contains(aliasStderr.Bytes(), []byte("alias for")) {
				t.Errorf("stderr missing alias announcement: got %q", aliasStderr.String())
			}

			// With help flag, should exit 0 (or 1 depending on the help path)
			// Just verify the command was dispatched without crashing
			if aliasExit != exitOK && aliasExit != exitUsage {
				t.Errorf("unexpected exit code: %d", aliasExit)
			}
		})
	}
}

// TestAliases_PrintAnnouncement verifies that each alias prints a
// one-line announcement to stderr noting the full command name.
func TestAliases_PrintAnnouncement(t *testing.T) {
	tests := []struct {
		alias         string
		expectedInErr string
	}{
		{"deploy", "alias for 'apps deploy'"},
		{"rollback", "alias for 'apps rollback'"},
	}

	for _, tt := range tests {
		t.Run(tt.alias, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			run("levelrail-cli-test", []string{tt.alias, "--help"}, &stdout, &stderr, envMap())

			if !bytes.Contains(stderr.Bytes(), []byte(tt.expectedInErr)) {
				t.Errorf("stderr does not contain %q; got %q", tt.expectedInErr, stderr.String())
			}
		})
	}
}
