package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestRun_Nodes_Help(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"nodes", "-h"}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d, want %d", got, exitOK)
	}
	if !strings.Contains(stdout.String(), "nodes list") || !strings.Contains(stdout.String(), "nodes drain") {
		t.Errorf("stdout = %q, want usage text mentioning list and drain", stdout.String())
	}
}

func TestRun_Nodes_UnknownSubcommand(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"nodes", "bogus"}, &stdout, &stderr, envMap())
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d", got, exitUsage)
	}
	if !strings.Contains(stderr.String(), "unknown nodes subcommand") {
		t.Errorf("stderr = %q, want an unknown subcommand error", stderr.String())
	}
}

func TestRun_Nodes_NoArgs(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"nodes"}, &stdout, &stderr, envMap())
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d", got, exitUsage)
	}
	if !strings.Contains(stderr.String(), "nodes list") {
		t.Errorf("stderr = %q, want usage text", stderr.String())
	}
}
