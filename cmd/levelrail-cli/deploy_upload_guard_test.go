package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

var railpackDirFiles = map[string]string{
	"package.json": `{"dependencies":{"next":"15"},"scripts":{"start":"next start"}}`,
}

func TestDeployUpload_BuildEnvRefusedBeforeAnySideEffect(t *testing.T) {
	dir := writeDeployDir(t, railpackDirFiles)
	var mu sync.Mutex
	var calls []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		calls = append(calls, r.Method+" "+r.URL.Path)
		mu.Unlock()
		http.Error(w, `{"error":"unexpected"}`, http.StatusNotFound)
	}))
	defer srv.Close()

	code, _, errOut := runDeployCLI([]string{"deploy", dir, "--yes", "--build-env", "NEXT_PUBLIC_X=1", "--api-url", srv.URL, "--token", "tok"}, nil)
	if code == exitOK || !strings.Contains(errOut, "--env") {
		t.Fatalf("exit = %d, stderr = %s, want a refusal that points at --env", code, errOut)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(calls) != 0 {
		t.Fatalf("server was called before the flag was refused: %v", calls)
	}
}

func TestDeployUpload_BuildFailureAfterCreateNamesTheApp(t *testing.T) {
	dir := writeDeployDir(t, appDirFiles)
	app := filepath.Base(dir)
	var created map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/apps/"+app:
			http.Error(w, `{"error":"app not found"}`, http.StatusNotFound)
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/apps":
			_ = json.NewDecoder(r.Body).Decode(&created)
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(created)
		case strings.HasSuffix(r.URL.Path, "/uploads"):
			http.Error(w, `{"error":"disk full"}`, http.StatusInsufficientStorage)
		default:
			http.Error(w, `{"error":"unexpected"}`, http.StatusNotFound)
		}
	}))
	defer srv.Close()

	code, _, errOut := runDeployCLI([]string{"deploy", dir, "--yes", "--api-url", srv.URL, "--token", "tok"}, nil)
	if code == exitOK {
		t.Fatalf("exit = 0, want a failure; stderr %s", errOut)
	}
	for _, want := range []string{"was created by this run", "apps delete " + app} {
		if !strings.Contains(errOut, want) {
			t.Errorf("stderr missing %q:\n%s", want, errOut)
		}
	}
}

func TestDeployUpload_FailureRemovesTheAppItCreated(t *testing.T) {
	dir := writeDeployDir(t, appDirFiles)
	app := filepath.Base(dir)
	var deleted bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/apps/"+app:
			http.Error(w, `{"error":"app not found"}`, http.StatusNotFound)
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/apps":
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"name":"` + app + `"}`))
		case r.Method == http.MethodDelete && r.URL.Path == "/api/v1/apps/"+app:
			deleted = true
			w.WriteHeader(http.StatusNoContent)
		case strings.HasSuffix(r.URL.Path, "/uploads"):
			http.Error(w, `{"error":"disk full"}`, http.StatusInsufficientStorage)
		default:
			http.Error(w, `{"error":"unexpected"}`, http.StatusNotFound)
		}
	}))
	defer srv.Close()

	code, _, errOut := runDeployCLI([]string{"deploy", dir, "--yes", "--api-url", srv.URL, "--token", "tok"}, nil)
	if code == exitOK || !deleted || !strings.Contains(errOut, "removed app") {
		t.Fatalf("exit = %d, deleted = %v, stderr = %s", code, deleted, errOut)
	}
}

func TestDeployUpload_UploadHashPrefix(t *testing.T) {
	dir := writeDeployDir(t, appDirFiles)
	app := filepath.Base(dir)
	full := strings.Repeat("ab", 32)
	other := "abcd" + strings.Repeat("0", 60)
	var built map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/apps/"+app:
			_ = json.NewEncoder(w).Encode(map[string]any{"name": app})
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/uploads"):
			_ = json.NewEncoder(w).Encode(map[string]any{"items": []map[string]any{{"hash": full}, {"hash": other}}})
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/builds"):
			_ = json.NewDecoder(r.Body).Decode(&built)
			w.WriteHeader(http.StatusAccepted)
			_, _ = w.Write([]byte(`{"id":"dep_1"}`))
		case strings.HasSuffix(r.URL.Path, "/deploys/dep_1/logs"):
			w.Header().Set("Content-Type", "text/event-stream")
		case strings.HasSuffix(r.URL.Path, "/deploys/dep_1"):
			_, _ = w.Write([]byte(`{"id":"dep_1","outcome":"healthy"}`))
		default:
			http.Error(w, `{"error":"unexpected"}`, http.StatusNotFound)
		}
	}))
	defer srv.Close()
	base := []string{"deploy", dir, "--yes", "--api-url", srv.URL, "--token", "tok", "--upload-hash"}

	if code, _, errOut := runDeployCLI(append(append([]string{}, base...), "abab"), nil); code == exitOK || !strings.Contains(errOut, "at least 8") {
		t.Errorf("short prefix: exit = %d, stderr = %s", code, errOut)
	}
	if code, _, errOut := runDeployCLI(append(append([]string{}, base...), "abcd0000"), nil); code != exitOK {
		t.Fatalf("unique prefix: exit = %d, stderr = %s", code, errOut)
	}
	if built["upload_hash"] != other {
		t.Errorf("build used %v, want the full digest %s", built["upload_hash"], other)
	}
	if code, _, errOut := runDeployCLI(append(append([]string{}, base...), "deadbeef"), nil); code == exitOK || !strings.Contains(errOut, "no stored upload") {
		t.Errorf("unknown prefix: exit = %d, stderr = %s", code, errOut)
	}
}

func TestStampTrace(t *testing.T) {
	if got := stampTrace("[trace] cutover: ok", 62e9); got != "[+1m2s] [trace] cutover: ok" {
		t.Errorf("got %q", got)
	}
	if got := stampTrace("plain line", 5e9); got != "plain line" {
		t.Errorf("got %q", got)
	}
}
