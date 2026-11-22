package main

import (
	"bytes"
	"testing"
)

func TestPagesDeleteAcceptsYes(t *testing.T) {
	var out, errb bytes.Buffer
	code := runPagesDelete("levelrail-cli", []string{"--yes", "--help"}, &out, &errb, func(string) (string, bool) { return "", false })
	if code == exitUsage && bytes.Contains(errb.Bytes(), []byte("flag provided but not defined")) {
		t.Fatalf("--yes rejected: %s", errb.String())
	}
}
