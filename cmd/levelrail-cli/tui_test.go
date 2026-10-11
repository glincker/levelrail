package main

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/tui"
)

func noEnv(string) (string, bool) { return "", false }

func TestRunTUINonTTYPrintsClearError(t *testing.T) {
	prev := tuiInteractive
	tuiInteractive = func() bool { return false }
	t.Cleanup(func() { tuiInteractive = prev })

	var out, errBuf bytes.Buffer
	code := run("lr", []string{"tui"}, &out, &errBuf, noEnv)
	if code != exitUsage {
		t.Fatalf("exit = %d, want %d", code, exitUsage)
	}
	if !strings.Contains(errBuf.String(), "interactive terminal") || out.Len() != 0 {
		t.Fatalf("stderr=%q stdout=%q", errBuf.String(), out.String())
	}
}

func TestRunTUIPassesOptions(t *testing.T) {
	prevI, prevR := tuiInteractive, tuiRun
	tuiInteractive = func() bool { return true }
	var got tui.Options
	tuiRun = func(_ context.Context, _ tui.API, o tui.Options) error {
		got = o
		return nil
	}
	t.Cleanup(func() { tuiInteractive, tuiRun = prevI, prevR })

	env := map[string]string{"NO_COLOR": "1", envTUILogBuffer: "50", envAPIURL: "https://" + "operator" + "@cp.example.com:9100/x"}
	lookup := func(k string) (string, bool) { v, ok := env[k]; return v, ok }
	var out, errBuf bytes.Buffer
	code := run("lr", []string{"tui", "--read-only", "--refresh", "500ms"}, &out, &errBuf, lookup)
	if code != exitOK {
		t.Fatalf("exit = %d stderr=%q", code, errBuf.String())
	}
	switch {
	case !got.ReadOnly:
		t.Error("read-only not forwarded")
	case got.Color:
		t.Error("NO_COLOR ignored")
	case got.Refresh != minTUIRefresh:
		t.Errorf("refresh = %v, want clamped to %v", got.Refresh, minTUIRefresh)
	case got.LogBuffer != 50:
		t.Errorf("log buffer = %d", got.LogBuffer)
	case got.Host != "cp.example.com:9100":
		t.Errorf("host = %q, must not carry userinfo or path", got.Host)
	}
}

func TestTUIRefreshPrecedence(t *testing.T) {
	env := func(v string) func(string) (string, bool) {
		return func(k string) (string, bool) {
			if k == envTUIRefresh && v != "" {
				return v, true
			}
			return "", false
		}
	}
	tests := []struct {
		name string
		flag time.Duration
		env  string
		want time.Duration
	}{
		{"default", 0, "", minTUIRefresh},
		{"flag wins", 5 * time.Second, "9s", 5 * time.Second},
		{"env used", 0, "7s", 7 * time.Second},
		{"bad env ignored", 0, "soon", minTUIRefresh},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tuiRefresh(tc.flag, env(tc.env)); got != tc.want {
				t.Fatalf("got %v want %v", got, tc.want)
			}
		})
	}
}
