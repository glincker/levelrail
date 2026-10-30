package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func TestPublishedPortErrorHints(t *testing.T) {
	tests := []struct {
		code, msg, hint string
	}{
		{"published_port_invalid", `published port 51820/udp: a host port between 1 and 65535 is required`, `"HOST:CONTAINER/udp"`},
		{"published_port_conflict", `host port 53/udp for service "dns" is already published by "other"`, "choose another host port"},
		{"published_port_remote_node", `host port 53/udp can only be published on the control plane's own node, service "dns" is placed on node "n2"`, "control plane node"},
		{"published_port_in_use", `host port 53/udp is already in use on this server by "systemd-resolve" (pid 812)`, "another process on this server already uses that port"},
		{"other_code", "boom", ""},
	}
	for _, tc := range tests {
		t.Run(tc.code, func(t *testing.T) {
			var out, errb bytes.Buffer
			reportError(&out, &errb, false, &apiclient.APIError{StatusCode: 400, Message: tc.msg, Code: tc.code})
			got := errb.String()
			if !strings.Contains(got, tc.msg) {
				t.Fatalf("server message missing: %q", got)
			}
			if tc.hint == "" && strings.Contains(got, "hint:") {
				t.Fatalf("unexpected hint: %q", got)
			}
			if tc.hint != "" && !strings.Contains(got, "hint: ") || !strings.Contains(got, tc.hint) {
				t.Fatalf("hint %q missing: %q", tc.hint, got)
			}
		})
	}
}
