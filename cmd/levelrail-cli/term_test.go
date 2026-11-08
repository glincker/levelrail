package main

import (
	"bytes"
	"io"
	"strings"
	"testing"
)

func TestTermStyleNoEscapesWhenNotTerminal(t *testing.T) {
	var buf bytes.Buffer
	st := styleFor(&buf)
	out := st.paint(ansiRed, "bad") + st.symbol("✔", "ok")
	if strings.Contains(out, "\x1b") || out != "badok" {
		t.Fatalf("out = %q", out)
	}
}

func TestTermStyleNoColorEnvAndFlag(t *testing.T) {
	prevStdout, prevLookup := stdoutIsTerminal, rawLookupEnv
	t.Cleanup(func() { stdoutIsTerminal, rawLookupEnv, noColorFlag = prevStdout, prevLookup, false })
	stdoutIsTerminal = func(io.Writer) bool { return true }
	rawLookupEnv = func(string) (string, bool) { return "", false }
	var buf bytes.Buffer
	if got := styleFor(&buf).paint(ansiRed, "x"); !strings.Contains(got, "\x1b[31m") {
		t.Fatalf("tty should color, got %q", got)
	}
	noColorFlag = true
	if got := styleFor(&buf).paint(ansiRed, "x"); got != "x" {
		t.Fatalf("flag ignored: %q", got)
	}
	noColorFlag = false
	rawLookupEnv = func(k string) (string, bool) { return "1", k == envNoColor }
	if got := styleFor(&buf).paint(ansiRed, "x"); got != "x" {
		t.Fatalf("NO_COLOR ignored: %q", got)
	}
}
