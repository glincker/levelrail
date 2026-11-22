package main

import (
	"bytes"
	"flag"
	"strings"
	"testing"
)

func TestRequireOneArgPicksApp(t *testing.T) {
	t.Chdir(t.TempDir())
	oldNames, oldIn, oldCI := pickerAppNames, pickerInput, ciMode
	defer func() { pickerAppNames, pickerInput, ciMode = oldNames, oldIn, oldCI }()
	pickerAppNames = func(string, *flag.FlagSet) []string { return []string{"api", "web", "worker"} }

	ciMode = false
	pickerInput = strings.NewReader("wor\n")
	fs := flag.NewFlagSet("t", flag.ContinueOnError)
	var errBuf bytes.Buffer
	if got, ok := requireOneArg(fs, &errBuf, "levelrail-cli", "apps logs", "app name"); !ok || got != "worker" {
		t.Fatalf("picker got (%q, %v)", got, ok)
	}

	ciMode = true
	errBuf.Reset()
	if _, ok := requireOneArg(fs, &errBuf, "levelrail-cli", "apps logs", "app name"); ok {
		t.Fatal("CI mode must not pick")
	}
	if !strings.Contains(errBuf.String(), "valid apps: api, web, worker") {
		t.Fatalf("CI error lacks names: %q", errBuf.String())
	}
}
