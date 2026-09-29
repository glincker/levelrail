package main

import (
	"bytes"
	"net/http"
	"strings"
	"testing"
)

func TestRun_ModelsSwapGroup_Set(t *testing.T) {
	var gotBody string
	srv, gotPath, gotMethod := newNoContentEchoServer(t)
	defer srv.Close()
	srv.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		buf := make([]byte, 256)
		n, _ := r.Body.Read(buf)
		gotBody = string(buf[:n])
		*gotPath, *gotMethod = r.URL.Path, r.Method
		w.WriteHeader(http.StatusNoContent)
	})

	stdout, _ := runCLIExpectOK(t, []string{"models", "swap-group", "chat", "--group", "gpu0", "--api-url", srv.URL})
	if *gotMethod != http.MethodPut || *gotPath != "/api/v1/models/chat/swap-group" {
		t.Errorf("request = %s %s", *gotMethod, *gotPath)
	}
	if !strings.Contains(gotBody, `"swap_group":"gpu0"`) {
		t.Errorf("body = %q, want swap_group=gpu0", gotBody)
	}
	if !strings.Contains(stdout, `"chat"`) || !strings.Contains(stdout, "gpu0") {
		t.Errorf("stdout = %q", stdout)
	}
}

func TestRun_ModelsSwapGroup_Clear(t *testing.T) {
	var gotBody string
	srv, gotPath, gotMethod := newNoContentEchoServer(t)
	defer srv.Close()
	srv.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		buf := make([]byte, 256)
		n, _ := r.Body.Read(buf)
		gotBody = string(buf[:n])
		*gotPath, *gotMethod = r.URL.Path, r.Method
		w.WriteHeader(http.StatusNoContent)
	})

	stdout, _ := runCLIExpectOK(t, []string{"models", "swap-group", "chat", "--clear", "--api-url", srv.URL})
	if *gotMethod != http.MethodPut || *gotPath != "/api/v1/models/chat/swap-group" {
		t.Errorf("request = %s %s", *gotMethod, *gotPath)
	}
	if !strings.Contains(gotBody, `"swap_group":""`) {
		t.Errorf("body = %q, want an empty swap_group", gotBody)
	}
	if !strings.Contains(stdout, "no longer in a swap group") {
		t.Errorf("stdout = %q", stdout)
	}
}

func TestRun_ModelsSwapGroup_RequiresGroupOrClear(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run("levelrail-cli-test", []string{"models", "swap-group", "chat"}, &stdout, &stderr, envMap()); code != exitValidation {
		t.Fatalf("exit = %d, want %d", code, exitValidation)
	}
	if !strings.Contains(stderr.String(), "--group") {
		t.Errorf("stderr = %q", stderr.String())
	}
}
