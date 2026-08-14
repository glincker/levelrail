package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestRun_Dispatch(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		wantExit   int
		wantStdout string
		wantStderr string
	}{
		{name: "no args", args: nil, wantExit: exitUsage, wantStderr: "Usage:"},
		{name: "root help", args: []string{"-h"}, wantExit: exitOK, wantStdout: "Usage:"},
		{name: "root help long form", args: []string{"--help"}, wantExit: exitOK, wantStdout: "Usage:"},
		{name: "unknown command", args: []string{"bogus"}, wantExit: exitUsage, wantStderr: "unknown command"},
		{name: "apps help", args: []string{"apps", "-h"}, wantExit: exitOK, wantStdout: "apps create"},
		{name: "apps no subcommand", args: []string{"apps"}, wantExit: exitUsage, wantStderr: "Usage:"},
		{name: "apps unknown subcommand", args: []string{"apps", "bogus"}, wantExit: exitUsage, wantStderr: "unknown apps subcommand"},
		{name: "apps create help", args: []string{"apps", "create", "-h"}, wantExit: exitOK, wantStderr: "Usage:"},
		{name: "apps create no flags", args: []string{"apps", "create"}, wantExit: exitValidation, wantStderr: "specify one of"},
		{name: "apps get no name", args: []string{"apps", "get"}, wantExit: exitUsage, wantStderr: "requires exactly one"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			// Isolated fake env and prog name: none of these dispatch
			// tests should reach the network or a real credentials
			// file, and every one of them returns before doing so
			// (help, usage errors, and validation errors are all
			// resolved before any Client is constructed).
			got := run("levelrail-cli-test", tt.args, &stdout, &stderr, envMap(nil))
			if got != tt.wantExit {
				t.Errorf("exit = %d, want %d (stdout=%q stderr=%q)", got, tt.wantExit, stdout.String(), stderr.String())
			}
			if tt.wantStdout != "" && !strings.Contains(stdout.String(), tt.wantStdout) {
				t.Errorf("stdout = %q, want it to contain %q", stdout.String(), tt.wantStdout)
			}
			if tt.wantStderr != "" && !strings.Contains(stderr.String(), tt.wantStderr) {
				t.Errorf("stderr = %q, want it to contain %q", stderr.String(), tt.wantStderr)
			}
		})
	}
}

func TestRun_CreateJSONErrorGoesToStdout(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"apps", "create", "--json"}, &stdout, &stderr, envMap(nil))
	if got != exitValidation {
		t.Fatalf("exit = %d, want %d", got, exitValidation)
	}
	if !strings.Contains(stdout.String(), `"error"`) {
		t.Errorf("stdout = %q, want a JSON error object", stdout.String())
	}
	if !strings.Contains(stderr.String(), "specify one of") {
		t.Errorf("stderr = %q, want the human error message too", stderr.String())
	}
}
