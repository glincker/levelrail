package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestAppsBuildSettingsUsage(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := runAppsBuildSettings("levelrail-cli", []string{"--help"}, &out, &errOut, func(string) (string, bool) { return "", false }); code != exitOK {
		t.Fatalf("help exit = %d", code)
	}
	if !strings.Contains(out.String(), "build-settings set <name>") {
		t.Fatalf("usage missing set verb: %s", out.String())
	}
	if code := runAppsBuildSettings("levelrail-cli", []string{"bogus"}, &out, &errOut, func(string) (string, bool) { return "", false }); code != exitUsage {
		t.Fatalf("unknown verb exit = %d", code)
	}
}
