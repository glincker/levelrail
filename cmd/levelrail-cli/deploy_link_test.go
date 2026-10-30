package main

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestValidateDeployLink(t *testing.T) {
	good := deployLinkParams{Image: "ghcr.io/you/app:1.2", Name: "my-app", Port: 3000, Env: []string{"API_KEY"}, Health: "/healthz"}
	tests := []struct {
		name    string
		mutate  func(p *deployLinkParams)
		wantErr string
	}{
		{"valid", func(*deployLinkParams) {}, ""},
		{"digest image", func(p *deployLinkParams) { p.Image = "nginx@sha256:" + strings.Repeat("a", 64) }, ""},
		{"uppercase image", func(p *deployLinkParams) { p.Image = "Nginx" }, "--image"},
		{"url image", func(p *deployLinkParams) { p.Image = "http://evil/app" }, "--image"},
		{"bad name", func(p *deployLinkParams) { p.Name = "My_App" }, "--name"},
		{"port zero", func(p *deployLinkParams) { p.Port = 0 }, "--port"},
		{"port high", func(p *deployLinkParams) { p.Port = 70000 }, "--port"},
		{"lowercase env", func(p *deployLinkParams) { p.Env = []string{"api_key"} }, "--env"},
		{"env with value", func(p *deployLinkParams) { p.Env = []string{"KEY=secret"} }, "--env"},
		{"health no slash", func(p *deployLinkParams) { p.Health = "healthz" }, "--health"},
		{"health double slash", func(p *deployLinkParams) { p.Health = "//evil" }, "--health"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := good
			tc.mutate(&p)
			err := validateDeployLink(p)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("err = %v, want containing %q", err, tc.wantErr)
			}
		})
	}
}

func TestRun_DeployLink(t *testing.T) {
	stdout, _ := runCLIExpectOK(t, []string{
		"deploy-link", "--image", "traefik/whoami:latest", "--name", "whoami", "--port", "80",
		"--env", "TOKEN,REGION", "--health", "/health",
		"--server", "https://lr.example.com", "--launcher", "https://docs.example.com/deploy.html",
	})
	for _, want := range []string{
		"https://lr.example.com/apps/new?deploy=1&image=traefik%2Fwhoami%3Alatest&name=whoami&port=80&env=TOKEN%2CREGION&health=%2Fhealth",
		"https://docs.example.com/deploy.html?image=traefik%2Fwhoami%3Alatest&name=whoami&port=80&env=TOKEN%2CREGION&health=%2Fhealth",
		"[![Deploy](https://docs.example.com/badges/deploy.svg)](",
	} {
		if !strings.Contains(stdout, want) {
			t.Errorf("stdout missing %q:\n%s", want, stdout)
		}
	}
}

func TestRun_DeployLink_Errors(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		wantCode int
	}{
		{"invalid image", []string{"deploy-link", "--image", "Bad Image", "--name", "a", "--port", "80", "--launcher", "https://x.example/deploy.html"}, exitValidation},
		{"no destination", []string{"deploy-link", "--image", "nginx", "--name", "a", "--port", "80"}, exitUsage},
		{"server with userinfo", []string{"deploy-link", "--image", "nginx", "--name", "a", "--port", "80", "--server", "https://u:p@x.example"}, exitValidation},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var out, errb bytes.Buffer
			if got := run("levelrail-cli-test", tc.args, &out, &errb, envMap()); got != tc.wantCode {
				t.Fatalf("exit = %d, want %d (stderr=%q)", got, tc.wantCode, errb.String())
			}
		})
	}
}

func TestLauncherBase(t *testing.T) {
	cases := map[string]string{
		"https://docs.levelrail.com/":  "https://levelrail.com",
		"https://levelrail.com":        "https://levelrail.com",
		"https://help.example.com/x/":  "https://help.example.com/x",
		"https://docs.example.com/app": "https://example.com/app",
	}
	for in, want := range cases {
		if got := launcherBase(in); got != want {
			t.Errorf("launcherBase(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestRun_DeployLinkRepo(t *testing.T) {
	var opened string
	openBrowserFunc = func(_ context.Context, u string) error { opened = u; return nil }
	t.Cleanup(func() { openBrowserFunc = openBrowser })
	stdout, _ := runCLIExpectOK(t, []string{
		"deploy-link", "--repo", "https://github.com/you/app.git", "--branch", "main", "--name", "app", "--port", "3000",
		"--api-url", "https://lr.example.com", "--open",
	})
	want := "https://lr.example.com/deploy?repo=https%3A%2F%2Fgithub.com%2Fyou%2Fapp.git&branch=main&name=app&port=3000"
	for _, s := range []string{want, "README Markdown:", "README HTML:", "docs/assets/deploy-to-levelrail.svg"} {
		if !strings.Contains(stdout, s) {
			t.Errorf("stdout missing %q:\n%s", s, stdout)
		}
	}
	if opened != want {
		t.Errorf("opened %q, want %q", opened, want)
	}
}

func TestRun_DeployLinkRepo_Errors(t *testing.T) {
	base := []string{"deploy-link", "--api-url", "https://lr.example.com"}
	tests := []struct {
		name     string
		args     []string
		wantCode int
	}{
		{"credentials", []string{"--repo", "https://u:p@github.com/a/b"}, exitValidation},
		{"file scheme", []string{"--repo", "file:///etc/passwd"}, exitValidation},
		{"bad template", []string{"--template", "Bad Id"}, exitValidation},
		{"branch with template", []string{"--template", "postgres", "--branch", "main"}, exitValidation},
		{"repo and template", []string{"--repo", "https://github.com/a/b", "--template", "postgres"}, exitValidation},
		{"repo with image", []string{"--repo", "https://github.com/a/b", "--image", "nginx"}, exitUsage},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var out, errb bytes.Buffer
			if got := run("levelrail-cli-test", append(append([]string{}, base...), tc.args...), &out, &errb, envMap()); got != tc.wantCode {
				t.Fatalf("exit = %d, want %d (stderr=%q)", got, tc.wantCode, errb.String())
			}
			if out.Len() != 0 {
				t.Errorf("unexpected stdout %q", out.String())
			}
		})
	}
}
