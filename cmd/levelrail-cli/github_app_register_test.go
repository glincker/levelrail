package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestRun_GitHubApp_RegisterURL(t *testing.T) {
	const base = "https://cp.example.com/api/v1/github-app/register/start"
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "personal", args: nil, want: base},
		{name: "org", args: []string{"--owner", "acme"}, want: base + "?owner=acme"},
		{name: "org public", args: []string{"--owner", "acme", "--public"}, want: base + "?owner=acme&public=true"},
		{name: "ghe", args: []string{"--instance-url", "https://ghe.example.com"}, want: base + "?instance_url=https%3A%2F%2Fghe.example.com"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := append([]string{"github-app", "register-url", "--api-url", "https://cp.example.com"}, tt.args...)
			stdout, _ := runCLIExpectOK(t, args)
			if got := strings.TrimSpace(stdout); got != tt.want {
				t.Errorf("stdout = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRun_GitHubApp_RegisterURL_InvalidOwner(t *testing.T) {
	var outBuf, errBuf bytes.Buffer
	got := run("levelrail-cli-test", []string{"github-app", "register-url", "--api-url", "https://cp.example.com", "--owner", "a/b"}, &outBuf, &errBuf, envMap())
	if got != exitUsage {
		t.Errorf("exit = %d, want %d", got, exitUsage)
	}
}
