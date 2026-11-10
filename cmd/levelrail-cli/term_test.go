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

func TestExtractNoColorFlagAndCompletion(t *testing.T) {
	rest, set := extractNoColorFlag([]string{"apps", "--no-color", "list", "--", "--no-color"})
	if !set || strings.Join(rest, " ") != "apps list -- --no-color" {
		t.Fatalf("rest=%v set=%v", rest, set)
	}
	var out, errb bytes.Buffer
	t.Cleanup(func() { noColorFlag = false })
	run("levelrail", []string{"--no-color", "--help"}, &out, &errb, func(string) (string, bool) { return "", false })
	if !noColorFlag || strings.Contains(out.String()+errb.String(), "unknown") {
		t.Fatalf("dispatch did not strip the flag: out=%q err=%q", out.String(), errb.String())
	}
	found := false
	for _, f := range globalFlags {
		found = found || f == "--no-color"
	}
	if !found {
		t.Fatal("completion lacks --no-color")
	}
}
