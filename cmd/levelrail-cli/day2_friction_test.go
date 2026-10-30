package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDoctorPathsOfflineJSON(t *testing.T) {
	var out, errb bytes.Buffer
	env := map[string]string{"LEVELRAIL_INSTALL_DIR": "/opt/lr", "APP_DATA_DIR": "/data/lr", "APP_HTTP_ADDR": "10.0.0.1:18080"}
	code := run("levelrail-cli-test", []string{"doctor", "paths", "--json"}, &out, &errb, func(k string) (string, bool) { v, ok := env[k]; return v, ok })
	if code != exitOK {
		t.Fatalf("exit %d stderr=%q", code, errb.String())
	}
	for _, want := range []string{"/opt/lr/levelrail.releases", "/data/lr", "10.0.0.1:18080"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("output missing %q: %s", want, out.String())
		}
	}
}

func TestUnknownFlagKeepsErrorShort(t *testing.T) {
	var out, errb bytes.Buffer
	code := run("levelrail-cli-test", []string{"doctor", "--yess"}, &out, &errb, envMap())
	if code != exitUsage {
		t.Fatalf("exit %d, want %d", code, exitUsage)
	}
	got := errb.String()
	if !strings.Contains(got, "yess") || !strings.Contains(got, "--help") {
		t.Errorf("stderr should name the flag and point at --help: %q", got)
	}
	if strings.Contains(got, "Runs the control plane's health checks") {
		t.Errorf("full usage leaked after a bad flag: %q", got)
	}
}

func TestOpenLocalDestRemovesPartialFileOnFailure(t *testing.T) {
	dst := filepath.Join(t.TempDir(), "out.txt")
	w, finish, err := openLocalDest(dst, nil)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = w.Write([]byte("partial"))
	_ = finish(errors.New("download failed"))
	if _, err := os.Stat(dst); !os.IsNotExist(err) {
		t.Errorf("destination exists after failed download: %v", err)
	}
	entries, _ := os.ReadDir(filepath.Dir(dst))
	if len(entries) != 0 {
		t.Errorf("temp file left behind: %v", entries)
	}

	w, finish, _ = openLocalDest(dst, nil)
	_, _ = w.Write([]byte("ok"))
	if err := finish(nil); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(dst) //nolint:gosec // test temp path
	if string(b) != "ok" {
		t.Errorf("content = %q", b)
	}
}
