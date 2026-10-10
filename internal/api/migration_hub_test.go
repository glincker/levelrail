package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/store"
)

const hubTestPassword = "s3cr3t-pw"

// hubRuntime fakes a source server with databases a, b, c where b's dump fails.
type hubRuntime struct {
	docker.Runtime
	mu      sync.Mutex
	scripts []string
}

func (h *hubRuntime) InspectByName(_ context.Context, name string) (*docker.ContainerState, error) {
	return &docker.ContainerState{ID: name, Name: name, Running: true}, nil
}

func (h *hubRuntime) Create(_ context.Context, spec docker.ContainerSpec) (string, error) {
	return "h-" + spec.Env["SRC_DB"], nil
}
func (h *hubRuntime) Start(context.Context, string) error        { return nil }
func (h *hubRuntime) Remove(context.Context, string, bool) error { return nil }

type errReader struct{ err error }

func (e errReader) Read([]byte) (int, error) { return 0, e.err }
func (e errReader) Close() error             { return nil }

func (h *hubRuntime) Exec(_ context.Context, container string, cmd []string) (io.ReadCloser, error) {
	script := cmd[2]
	h.mu.Lock()
	h.scripts = append(h.scripts, script)
	h.mu.Unlock()
	ok := func(s string) (io.ReadCloser, error) { return io.NopCloser(strings.NewReader(s)), nil }
	switch {
	case strings.Contains(script, "pg_database_size"):
		return ok("version|16.4\ndb|a|1048576\ndb|b|2097152\ndb|c|3145728\n")
	case strings.Contains(script, "'tables|'"):
		return ok("tables|1\next|pg_trgm\n")
	case strings.Contains(script, "df -Pk"):
		return ok("free|107374182400\n")
	case strings.Contains(script, "pg_dump"):
		if container == "h-b" {
			return errReader{err: errors.New("pg_dump: error: connection to server failed for " + hubTestPassword)}, nil
		}
		return ok("CREATE TABLE t();")
	case strings.Contains(script, "xpath"):
		return ok("public.t|3\n")
	}
	return ok("")
}

func (h *hubRuntime) ExecWithInput(_ context.Context, _ string, _ []string, in io.Reader) (io.ReadCloser, error) {
	_, _ = io.Copy(io.Discard, in)
	return io.NopCloser(strings.NewReader("")), nil
}

func TestMigrationHubHalfFailure(t *testing.T) {
	rt, db, cookie := newSafetyRouter(t, stubResolver{digest: "sha256:abc"})
	runtime := &hubRuntime{}
	rt.execRuntime = func(string) (docker.Runtime, error) { return runtime, nil }
	call := func(method, path, body string) *httptest.ResponseRecorder {
		return serve(rt, authedRequest(t, cookie, method, path, body))
	}

	rec := call(http.MethodPost, "/api/v1/migration/hub/sessions", `{"engine":"postgres","host":"old.example.com","user":"u","password":"`+hubTestPassword+`"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create = %d: %s", rec.Code, rec.Body)
	}
	var sess hubSessionResource
	if err := json.Unmarshal(rec.Body.Bytes(), &sess); err != nil {
		t.Fatal(err)
	}
	if len(sess.Items) != 3 || sess.ServerVersion != "16.4" || !sess.PasswordHeld || sess.Summary.Selected != 3 {
		t.Fatalf("unexpected session %+v", sess)
	}
	if strings.Contains(rec.Body.String(), hubTestPassword) {
		t.Fatal("session response leaks the password")
	}

	rec = call(http.MethodPost, "/api/v1/migration/hub/sessions/"+sess.ID+"/apply", `{}`)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("apply = %d: %s", rec.Code, rec.Body)
	}
	var final hubSessionResource
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		get := call(http.MethodGet, "/api/v1/migration/hub/sessions/"+sess.ID, "")
		_ = json.Unmarshal(get.Body.Bytes(), &final)
		if !final.Running && final.Summary.Verified+final.Summary.Failed == 3 {
			break
		}
		time.Sleep(25 * time.Millisecond)
	}
	got := map[string]string{}
	for _, it := range final.Items {
		got[it.SourceDB] = it.Status
		if strings.Contains(it.Reason, hubTestPassword) {
			t.Fatalf("reason leaks password: %q", it.Reason)
		}
	}
	if got["a"] != store.HubItemVerified || got["c"] != store.HubItemVerified || got["b"] != store.HubItemFailed {
		t.Fatalf("statuses = %v", got)
	}
	if final.Step != hubStepVerify {
		t.Errorf("step = %q, want verify", final.Step)
	}
	if _, err := db.GetDesiredDatabase(context.Background(), "a"); err != nil {
		t.Errorf("managed database a was not created: %v", err)
	}

	runtime.mu.Lock()
	for _, s := range runtime.scripts {
		if strings.Contains(s, "SRC_HOST") && (strings.Contains(s, "pg_dump") || strings.Contains(s, "psql")) && !strings.Contains(s, "default_transaction_read_only=on") {
			t.Errorf("source command without read-only session:\n%s", s)
		}
	}
	runtime.mu.Unlock()

	receipt := call(http.MethodGet, "/api/v1/migration/hub/sessions/"+sess.ID+"/receipt", "")
	var rc hubReceipt
	if err := json.Unmarshal(receipt.Body.Bytes(), &rc); err != nil || len(rc.Databases) != 2 {
		t.Fatalf("receipt = %s (%v)", receipt.Body, err)
	}
	if strings.Contains(receipt.Body.String(), hubTestPassword) || strings.Contains(receipt.Body.String(), secretMask) {
		t.Error("receipt carries a secret or mask value")
	}

	if conn := call(http.MethodGet, "/api/v1/migration/hub/sessions/"+sess.ID+"/items/b/connection", ""); conn.Code != http.StatusNotFound {
		t.Errorf("connection of a failed copy = %d, want 404", conn.Code)
	}
}

func TestMigrationHubBlocksUnsupportedExtension(t *testing.T) {
	rt, _, cookie := newSafetyRouter(t, stubResolver{digest: "sha256:abc"})
	rt.execRuntime = func(string) (docker.Runtime, error) { return &extRuntime{}, nil }
	rec := serve(rt, authedRequest(t, cookie, http.MethodPost, "/api/v1/migration/hub/sessions", `{"engine":"postgres","host":"old.example.com","user":"u","password":"x"}`))
	var sess hubSessionResource
	_ = json.Unmarshal(rec.Body.Bytes(), &sess)
	if sess.Summary.Blocked != 1 {
		t.Fatalf("blocked = %d, want 1: %s", sess.Summary.Blocked, rec.Body)
	}
	rec = serve(rt, authedRequest(t, cookie, http.MethodPost, "/api/v1/migration/hub/sessions/"+sess.ID+"/apply", `{}`))
	if rec.Code != http.StatusConflict {
		t.Fatalf("apply with a blocked database = %d, want 409", rec.Code)
	}
}

type extRuntime struct{ hubRuntime }

func (e *extRuntime) Exec(_ context.Context, _ string, cmd []string) (io.ReadCloser, error) {
	script := cmd[2]
	switch {
	case strings.Contains(script, "pg_database_size"):
		return io.NopCloser(strings.NewReader("version|16.4\ndb|geo|1024\n")), nil
	case strings.Contains(script, "'tables|'"):
		return io.NopCloser(strings.NewReader("tables|2\next|postgis\n")), nil
	}
	return io.NopCloser(strings.NewReader("free|107374182400\n")), nil
}
