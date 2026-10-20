package main

import (
	"bytes"
	"reflect"
	"strings"
	"testing"
)

func TestExtractDebugFlag(t *testing.T) {
	tests := []struct {
		name          string
		args          []string
		wantRemaining []string
		wantDebug     bool
	}{
		{name: "absent", args: []string{"apps", "list"}, wantRemaining: []string{"apps", "list"}, wantDebug: false},
		{name: "trailing", args: []string{"apps", "list", "--debug"}, wantRemaining: []string{"apps", "list"}, wantDebug: true},
		{name: "leading", args: []string{"--debug", "apps", "list"}, wantRemaining: []string{"apps", "list"}, wantDebug: true},
		{name: "middle", args: []string{"apps", "--debug", "list"}, wantRemaining: []string{"apps", "list"}, wantDebug: true},
		{
			name:          "not stripped after -- terminator",
			args:          []string{"apps", "exec", "web", "--", "echo", "--debug"},
			wantRemaining: []string{"apps", "exec", "web", "--", "echo", "--debug"},
			wantDebug:     false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotRemaining, gotDebug := extractDebugFlag(tt.args)
			if !reflect.DeepEqual(gotRemaining, tt.wantRemaining) {
				t.Errorf("remaining = %v, want %v", gotRemaining, tt.wantRemaining)
			}
			if gotDebug != tt.wantDebug {
				t.Errorf("debug = %v, want %v", gotDebug, tt.wantDebug)
			}
		})
	}
}

func TestRun_Debug_TracesToStderrNeverStdout(t *testing.T) {
	t.Chdir(t.TempDir())

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"apps", "list", "--debug", "--token", "faketoken", "--api-url", "http://127.0.0.1:0"}, &stdout, &stderr, envMap())
	if got != exitNetwork {
		t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", got, exitNetwork, stdout.String(), stderr.String())
	}
	if strings.Contains(stdout.String(), "DEBUG:") {
		t.Errorf("stdout = %q, debug trace must never reach stdout", stdout.String())
	}
	if !strings.Contains(stderr.String(), "DEBUG: GET") {
		t.Errorf("stderr = %q, want a DEBUG trace line", stderr.String())
	}
	if strings.Contains(stderr.String(), "faketoken") {
		t.Errorf("stderr = %q, leaked the bearer token", stderr.String())
	}
}

func TestRun_NoDebug_NoTraceOnStderr(t *testing.T) {
	t.Chdir(t.TempDir())

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"apps", "list", "--token", "faketoken", "--api-url", "http://127.0.0.1:0"}, &stdout, &stderr, envMap())
	if got != exitNetwork {
		t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", got, exitNetwork, stdout.String(), stderr.String())
	}
	if strings.Contains(stderr.String(), "DEBUG:") {
		t.Errorf("stderr = %q, want no debug trace without --debug", stderr.String())
	}
}
