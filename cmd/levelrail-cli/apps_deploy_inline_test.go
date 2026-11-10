package main

import (
	"os"
	"strings"
	"testing"
)

func TestRun_AppsDeployDockerfileFileSendsInlineSource(t *testing.T) {
	var built map[string]any
	srv := sourceDeployServer(t, &built)
	t.Chdir(t.TempDir())
	if err := os.WriteFile("Dockerfile.prod", []byte("FROM scratch\nCOPY conf/a.conf /\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir("conf", 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("conf/a.conf", []byte("x=1"), 0o600); err != nil {
		t.Fatal(err)
	}
	stdout, _ := runCLIExpectOK(t, []string{"apps", "deploy", "web", "--dockerfile-file", "Dockerfile.prod", "--context-file", "conf/a.conf", "--api-url", srv.URL})
	in, _ := built["inline_source"].(map[string]any)
	if in["dockerfile"] != "FROM scratch\nCOPY conf/a.conf /\n" || built["repo_url"] != "" {
		t.Fatalf("build body = %v", built)
	}
	files, _ := in["files"].([]any)
	f0, _ := files[0].(map[string]any)
	if len(files) != 1 || f0["path"] != "conf/a.conf" || f0["content"] != "x=1" {
		t.Errorf("files = %v", files)
	}
	if !strings.Contains(stdout, "dep_1") {
		t.Errorf("stdout = %q", stdout)
	}
}

func TestRun_AppsDeployDockerfileRefusals(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := os.WriteFile("Dockerfile", []byte("FROM scratch"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("/etc/hosts", "linked"); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	for _, args := range [][]string{
		{"--dockerfile-file", "Dockerfile", "--context-file", "linked"},
		{"--dockerfile-file", "linked"},
		{"--dockerfile-file", "Dockerfile", "--context-file", "../outside"},
		{"--dockerfile-file", "Dockerfile", "--image", "nginx:1"},
		{"--dockerfile", "Dockerfile"},
		{"--context-file", "Dockerfile", "--commit", "abc"},
	} {
		runCLIExpectValidationError(t, append([]string{"apps", "deploy", "web", "--api-url", "http://127.0.0.1:1"}, args...))
	}
}

func TestReadInlineSource_Stdin(t *testing.T) {
	src, err := readInlineSource(sourceDeployFlags{dockerfile: "-"}, strings.NewReader("FROM alpine\n"))
	if err != nil || src.Dockerfile != "FROM alpine\n" {
		t.Fatalf("src = %+v, %v", src, err)
	}
	if _, err := readInlineSource(sourceDeployFlags{dockerfile: "-"}, strings.NewReader("  \n")); err == nil {
		t.Fatal("empty stdin accepted")
	}
}
