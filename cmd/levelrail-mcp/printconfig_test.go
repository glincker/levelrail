package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func noEnv(string) (string, bool) { return "", false }

func captureConfig(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	old := configOut
	configOut = &buf
	t.Cleanup(func() { configOut = old })
	return &buf
}

func TestPrintConfigShapes(t *testing.T) {
	tests := []struct {
		client string
		path   []string
	}{
		{"claude", []string{"mcpServers", "levelrail-mcp"}},
		{"cursor", []string{"mcpServers", "levelrail-mcp"}},
		{"vscode", []string{"servers", "levelrail-mcp"}},
		{"zed", []string{"context_servers", "levelrail-mcp", "command"}},
	}
	for _, tt := range tests {
		t.Run(tt.client, func(t *testing.T) {
			buf := captureConfig(t)
			f := cliFlags{printConfig: tt.client, apiURL: "http://cp.example:8080"}
			if err := runPrintConfig("levelrail-mcp", f, noEnv); err != nil {
				t.Fatal(err)
			}
			var doc map[string]any
			if err := json.Unmarshal(buf.Bytes(), &doc); err != nil {
				t.Fatalf("not json: %v\n%s", err, buf.String())
			}
			var cur any = doc
			for _, k := range tt.path {
				cur = cur.(map[string]any)[k]
				if cur == nil {
					t.Fatalf("missing %v in %s", tt.path, buf.String())
				}
			}
			out := buf.String()
			for _, want := range []string{`"read-only"`, "<your API token>", "http://cp.example:8080"} {
				if !strings.Contains(out, want) {
					t.Errorf("output lacks %s:\n%s", want, out)
				}
			}
			if strings.Contains(out, "--allow-write") {
				t.Errorf("write must not be on by default:\n%s", out)
			}
		})
	}
}

func TestPrintConfigOptionsAndErrors(t *testing.T) {
	buf := captureConfig(t)
	f := cliFlags{printConfig: "claude", allowWrite: true, mode: "standard", team: "t1"}
	if err := runPrintConfig("levelrail-mcp", f, noEnv); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"--allow-write", `"standard"`, "--team", "t1"} {
		if !strings.Contains(buf.String(), want) {
			t.Errorf("missing %s:\n%s", want, buf.String())
		}
	}
	if err := runPrintConfig("levelrail-mcp", cliFlags{printConfig: "emacs"}, noEnv); err == nil {
		t.Error("unknown client must fail")
	}
}

func TestPrintConfigNeverEmitsRealToken(t *testing.T) {
	buf := captureConfig(t)
	env := func(k string) (string, bool) {
		if k == "APP_API_TOKEN" {
			return "lr_realsecret", true
		}
		return "", false
	}
	if err := runPrintConfig("levelrail-mcp", cliFlags{printConfig: "claude", token: "lr_flagsecret"}, env); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(buf.String(), "realsecret") || strings.Contains(buf.String(), "flagsecret") {
		t.Errorf("token leaked:\n%s", buf.String())
	}
}
