package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestBootArgsExit(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		wantCode   int
		wantHandle bool
		wantOut    string
		wantErr    string
	}{
		{name: "no args starts server", args: nil},
		{name: "known subcommand falls through", args: []string{"healthcheck", "--ready"}},
		{name: "help word", args: []string{"help"}, wantHandle: true, wantOut: "Usage:"},
		{name: "short help", args: []string{"-h"}, wantHandle: true, wantOut: "Usage:"},
		{name: "long help", args: []string{"--help"}, wantHandle: true, wantOut: "Usage:"},
		{name: "version flag", args: []string{"--version"}, wantHandle: true, wantOut: "dev"},
		{name: "unknown flag", args: []string{"--bogus"}, wantCode: 2, wantHandle: true, wantErr: "unknown command or flag"},
		{name: "unknown word", args: []string{"serve"}, wantCode: 2, wantHandle: true, wantErr: "Usage:"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var out, errOut bytes.Buffer
			code, handled := bootArgsExit(tc.args, "/usr/local/bin/tool", &out, &errOut)
			if code != tc.wantCode || handled != tc.wantHandle {
				t.Fatalf("got (%d,%v), want (%d,%v)", code, handled, tc.wantCode, tc.wantHandle)
			}
			if !strings.Contains(out.String(), tc.wantOut) || !strings.Contains(errOut.String(), tc.wantErr) {
				t.Fatalf("stdout %q stderr %q", out.String(), errOut.String())
			}
		})
	}
}
