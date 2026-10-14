package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestGenKeyAndMissingEnv(t *testing.T) {
	var out, errb bytes.Buffer
	none := func(string) (string, bool) { return "", false }
	if code := run([]string{"-gen-key"}, &out, &errb, none); code != 0 || !strings.Contains(out.String(), "public") {
		t.Fatalf("gen-key code=%d out=%q err=%q", code, out.String(), errb.String())
	}
	if code := run([]string{"-sign-key-env", "NOPE", "-out", t.TempDir()}, &out, &errb, none); code != 1 {
		t.Fatalf("missing env code = %d", code)
	}
}
