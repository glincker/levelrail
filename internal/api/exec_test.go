package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/store"
)

// fakeExecAppRuntime is a hand-written fake docker.Runtime, the same
// convention internal/backup/dump_test.go's own fakeExecRuntime already
// establishes for this exact interface: every method is a compile-
// satisfying stub except the two handleExecApp actually calls
// (InspectByName, Exec), which are configurable per test.
type fakeExecAppRuntime struct {
	inspectState *docker.ContainerState
	inspectErr   error

	execReader io.ReadCloser
	execErr    error

	gotContainerID string
	gotCmd         []string
	execCalls      int
}

func (f *fakeExecAppRuntime) InspectByName(_ context.Context, _ string) (*docker.ContainerState, error) {
	return f.inspectState, f.inspectErr
}

func (f *fakeExecAppRuntime) Exec(_ context.Context, containerID string, cmd []string) (io.ReadCloser, error) {
	f.execCalls++
	f.gotContainerID = containerID
	f.gotCmd = cmd
	return f.execReader, f.execErr
}

func (f *fakeExecAppRuntime) ExecWithInput(context.Context, string, []string, io.Reader) (io.ReadCloser, error) {
	return nil, nil
}
func (f *fakeExecAppRuntime) Create(context.Context, docker.ContainerSpec) (string, error) {
	return "", nil
}
func (f *fakeExecAppRuntime) Start(context.Context, string) error { return nil }
func (f *fakeExecAppRuntime) Events(context.Context) (<-chan docker.Event, <-chan error) {
	return nil, nil
}
func (f *fakeExecAppRuntime) ListImages(context.Context, string) ([]docker.ImageInfo, error) {
	return nil, nil
}
func (f *fakeExecAppRuntime) ListByPrefix(context.Context, string) ([]docker.ContainerState, error) {
	return nil, nil
}
func (f *fakeExecAppRuntime) Stop(context.Context, string, time.Duration) error { return nil }
func (f *fakeExecAppRuntime) Remove(context.Context, string, bool) error        { return nil }
func (f *fakeExecAppRuntime) EnsureVolume(context.Context, string) error        { return nil }
func (f *fakeExecAppRuntime) EnsureNetwork(context.Context, string) (string, error) {
	return "", nil
}
func (f *fakeExecAppRuntime) RemoveNetwork(context.Context, string) error { return nil }
func (f *fakeExecAppRuntime) ListNetworksByPrefix(context.Context, string) ([]docker.NetworkInfo, error) {
	return nil, nil
}

var _ docker.Runtime = (*fakeExecAppRuntime)(nil)

// blockingReadCloser never returns from Read until Close is called (or
// the test times the whole thing out some other way), simulating a
// command whose output stream a caller stops waiting on before it ever
// naturally ends. Used by the timeout test to prove handleExecApp
// itself never blocks past its own deadline, regardless of whether the
// underlying stream ever resolves; see handleExecApp's own doc comment
// on the timeout branch for the real, narrower guarantee this models
// (the HTTP handler returns on time; the goroutine reading the stream
// unblocks once Close runs, not necessarily any sooner).
type blockingReadCloser struct {
	closed chan struct{}
}

func newBlockingReadCloser() *blockingReadCloser {
	return &blockingReadCloser{closed: make(chan struct{})}
}

func (b *blockingReadCloser) Read(_ []byte) (int, error) {
	<-b.closed
	return 0, io.EOF
}

func (b *blockingReadCloser) Close() error {
	select {
	case <-b.closed:
	default:
		close(b.closed)
	}
	return nil
}

// newTestRouterWithExecRuntime wires fake as the single node this test
// router's NodeRuntimeResolver ever resolves to, regardless of nodeID:
// none of this file's tests place an app on a named node (NodeID stays
// "", the store's own "local node" convention), so a resolver that
// ignores its argument is a faithful enough stand-in for
// resolveNodeTransport's real "" branch (cmd/levelrail/main.go) without
// this package needing to depend on internal/agent.Registry just to
// build one for a test.
func newTestRouterWithExecRuntime(t *testing.T, fake *fakeExecAppRuntime) (*Router, *store.DB) {
	t.Helper()
	db := openTestDB(t)
	resolver := func(string) (docker.Runtime, error) { return fake, nil }
	return NewRouter(discardLogger(), testBrand(), db, WithExecRuntime(resolver)), db
}

func seedExecApp(t *testing.T, db *store.DB) {
	t.Helper()
	svc := store.DesiredService{Name: "web", Image: "levelrail/web:1", Port: 3000}
	if err := db.SaveDesiredService(context.Background(), svc); err != nil {
		t.Fatalf("seed app: %v", err)
	}
}

func TestHandleExecApp_NotConfigured(t *testing.T) {
	rt, db := newTestRouter(t) // no WithExecRuntime
	cookie := loginTestSession(t, rt, db)
	seedExecApp(t, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/exec", `{"command":"echo"}`))
	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusNotImplemented, rec.Body.String())
	}
}

func TestExecAppRoute_RequiresAuth(t *testing.T) {
	rt, _ := newTestRouter(t)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps/web/exec", strings.NewReader(`{"command":"echo"}`))
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

// TestHandleExecApp_PlainWriteToken_Forbidden proves POST
// /apps/{name}/exec sits behind AbilityRoot, not AbilityWrite.
func TestHandleExecApp_PlainWriteToken_Forbidden(t *testing.T) {
	fake := &fakeExecAppRuntime{inspectState: &docker.ContainerState{ID: "c1", Running: true}}
	rt, db := newTestRouterWithExecRuntime(t, fake)
	ctx := context.Background()
	seedExecApp(t, db)

	const plaintext = "write-scoped-token" //nolint:gosec // fake fixture, not a real credential
	if err := db.SaveAPIToken(ctx, store.APIToken{
		ID: "tok_write", Name: "writer", TokenHash: hashToken(plaintext), Abilities: []string{AbilityWrite}, CreatedAt: time.Now(),
	}); err != nil {
		t.Fatalf("seed token: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps/web/exec", strings.NewReader(`{"command":"echo"}`))
	req.Header.Set("Authorization", "Bearer "+plaintext)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d: a plain write token must not be able to run commands inside a container", rec.Code, http.StatusForbidden)
	}
	if fake.execCalls != 0 {
		t.Errorf("Exec called %d times, want 0: the ability check must reject before any container work happens", fake.execCalls)
	}
}

// TestHandleExecApp_PlainDeployToken_Forbidden proves a token scoped to
// nothing but AbilityDeploy (the CI/MCP-token shape abilities.go
// documents as its own use case) cannot run exec either, even though
// that same token can already trigger a deploy or a restart. Exec sits
// one tier above those because it is the one route that can read a
// secret's plaintext back out of the container's own environment
// (secrets are injected as plaintext env vars at create time, CLAUDE.md
// 4.10, and this package otherwise never decrypts one into a response
// body, see the secrets route's own doc comment above its registration
// in router.go). A plain deploy token running `env` inside the
// container would otherwise exfiltrate every secret the app holds.
func TestHandleExecApp_PlainDeployToken_Forbidden(t *testing.T) {
	fake := &fakeExecAppRuntime{inspectState: &docker.ContainerState{ID: "c1", Running: true}}
	rt, db := newTestRouterWithExecRuntime(t, fake)
	ctx := context.Background()
	seedExecApp(t, db)

	const plaintext = "deploy-scoped-token" //nolint:gosec // fake fixture, not a real credential
	if err := db.SaveAPIToken(ctx, store.APIToken{
		ID: "tok_deploy", Name: "deployer", TokenHash: hashToken(plaintext), Abilities: []string{AbilityDeploy}, CreatedAt: time.Now(),
	}); err != nil {
		t.Fatalf("seed token: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps/web/exec", strings.NewReader(`{"command":"env"}`))
	req.Header.Set("Authorization", "Bearer "+plaintext)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d: a plain deploy token must not be able to read secrets back out of a container via exec", rec.Code, http.StatusForbidden)
	}
	if fake.execCalls != 0 {
		t.Errorf("Exec called %d times, want 0: the ability check must reject before any container work happens", fake.execCalls)
	}
}

func TestHandleExecApp_UnknownApp_NotFound(t *testing.T) {
	fake := &fakeExecAppRuntime{}
	rt, db := newTestRouterWithExecRuntime(t, fake)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/nonexistent/exec", `{"command":"echo"}`))
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d, body = %s", rec.Code, http.StatusNotFound, rec.Body.String())
	}
}

func TestHandleExecApp_MalformedBody_BadRequest(t *testing.T) {
	fake := &fakeExecAppRuntime{}
	rt, db := newTestRouterWithExecRuntime(t, fake)
	cookie := loginTestSession(t, rt, db)
	seedExecApp(t, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/exec", `{not json`))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d, body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestHandleExecApp_EmptyCommand_BadRequest(t *testing.T) {
	fake := &fakeExecAppRuntime{}
	rt, db := newTestRouterWithExecRuntime(t, fake)
	cookie := loginTestSession(t, rt, db)
	seedExecApp(t, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/exec", `{"command":"   "}`))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d, body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestHandleExecApp_NoRunningContainer_Conflict(t *testing.T) {
	tests := []struct {
		name  string
		state *docker.ContainerState
	}{
		{name: "never deployed", state: nil},
		{name: "exists but stopped", state: &docker.ContainerState{ID: "c1", Running: false}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fake := &fakeExecAppRuntime{inspectState: tc.state}
			rt, db := newTestRouterWithExecRuntime(t, fake)
			cookie := loginTestSession(t, rt, db)
			seedExecApp(t, db)

			rec := httptest.NewRecorder()
			rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/exec", `{"command":"echo"}`))
			if rec.Code != http.StatusConflict {
				t.Errorf("status = %d, want %d, body = %s", rec.Code, http.StatusConflict, rec.Body.String())
			}
			if fake.execCalls != 0 {
				t.Errorf("Exec called %d times, want 0: no running container to exec into", fake.execCalls)
			}
		})
	}
}

func TestHandleExecApp_Success_Stdout(t *testing.T) {
	fake := &fakeExecAppRuntime{
		inspectState: &docker.ContainerState{ID: "c1", Running: true},
		execReader:   io.NopCloser(strings.NewReader("hello from inside\n")),
	}
	rt, db := newTestRouterWithExecRuntime(t, fake)
	cookie := loginTestSession(t, rt, db)
	seedExecApp(t, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/exec", `{"command":"echo","args":["hello","from","inside"]}`))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got execResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Stdout != "hello from inside\n" {
		t.Errorf("Stdout = %q, want %q", got.Stdout, "hello from inside\n")
	}
	if got.ExitCode != 0 {
		t.Errorf("ExitCode = %d, want 0", got.ExitCode)
	}
	if got.Stderr != "" {
		t.Errorf("Stderr = %q, want empty", got.Stderr)
	}
	if got.Truncated {
		t.Error("Truncated = true, want false")
	}

	if fake.gotContainerID != "c1" {
		t.Errorf("Exec called with container %q, want %q", fake.gotContainerID, "c1")
	}
	wantCmd := []string{"echo", "hello", "from", "inside"}
	if len(fake.gotCmd) != len(wantCmd) {
		t.Fatalf("Exec cmd = %v, want %v", fake.gotCmd, wantCmd)
	}
	for i := range wantCmd {
		if fake.gotCmd[i] != wantCmd[i] {
			t.Errorf("Exec cmd = %v, want %v", fake.gotCmd, wantCmd)
			break
		}
	}
}

// TestHandleExecApp_Success_NonzeroExit proves a command that runs but
// fails inside the container (a missing binary, a real application
// error) is reported as a normal 200 response with the real exit code
// and stderr, not as an HTTP-level failure: see execResponse's own doc
// comment for why. This is the one case docker.Runtime.Exec's own
// contract folds exit code and stderr into the trailing error
// (docker.ExecExitError), so this test is also the proof that
// writeExecOutcome's errors.As branch actually recovers both fields.
func TestHandleExecApp_Success_NonzeroExit(t *testing.T) {
	execErr := &docker.ExecExitError{
		Cmd:       []string{"false"},
		Container: "c1",
		ExitCode:  17,
		Stderr:    "boom: no such file",
	}
	fake := &fakeExecAppRuntime{
		inspectState: &docker.ContainerState{ID: "c1", Running: true},
		execReader:   io.NopCloser(strings.NewReader("partial output\n")),
	}
	// A real docker.Client's Exec surfaces the exit error as the last
	// Read's error, not as Exec's own return error (see Runtime.Exec's
	// doc comment): errReadCloser below reproduces that exact shape,
	// some bytes then a trailing error, rather than Exec itself failing.
	fake.execReader = &errReadCloser{r: strings.NewReader("partial output\n"), err: execErr}

	rt, db := newTestRouterWithExecRuntime(t, fake)
	cookie := loginTestSession(t, rt, db)
	seedExecApp(t, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/exec", `{"command":"false"}`))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got execResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.ExitCode != 17 {
		t.Errorf("ExitCode = %d, want 17", got.ExitCode)
	}
	if got.Stderr != "boom: no such file" {
		t.Errorf("Stderr = %q, want %q", got.Stderr, "boom: no such file")
	}
	if got.Stdout != "partial output\n" {
		t.Errorf("Stdout = %q, want %q", got.Stdout, "partial output\n")
	}
}

// errReadCloser reads r to completion and then returns err instead of
// io.EOF, the same "some bytes, then a trailing non-EOF error" shape
// docker.Client.Exec's real returned io.ReadCloser has once a command
// exits non-zero (see streamExecOutput, internal/docker/client.go).
type errReadCloser struct {
	r   io.Reader
	err error
}

func (e *errReadCloser) Read(p []byte) (int, error) {
	n, err := e.r.Read(p)
	if err == io.EOF {
		return n, e.err
	}
	return n, err
}

func (e *errReadCloser) Close() error { return nil }

// TestHandleExecApp_GenuineExecTransportFailure proves an error reading
// Exec's stream that is *not* a docker.ExecExitError (a real transport
// failure: lost connection, daemon restarted mid-command) is reported
// as a 500, not folded into a 200 the way a normal nonzero exit is.
func TestHandleExecApp_GenuineExecTransportFailure(t *testing.T) {
	fake := &fakeExecAppRuntime{
		inspectState: &docker.ContainerState{ID: "c1", Running: true},
		execReader:   &errReadCloser{r: strings.NewReader(""), err: errors.New("connection reset by peer")},
	}
	rt, db := newTestRouterWithExecRuntime(t, fake)
	cookie := loginTestSession(t, rt, db)
	seedExecApp(t, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/exec", `{"command":"echo"}`))
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d, body = %s", rec.Code, http.StatusInternalServerError, rec.Body.String())
	}
}

// TestHandleExecApp_Timeout proves a command whose output stream never
// resolves does not hold this handler open past its own timeout: the
// request supplies a 1-second override (well under defaultExecTimeout)
// specifically so this test finishes quickly rather than waiting out
// the real 30-second default.
func TestHandleExecApp_Timeout(t *testing.T) {
	blocking := newBlockingReadCloser()
	t.Cleanup(func() { _ = blocking.Close() }) // avoid leaking the fake's own goroutine-less-but-still-open channel past this test
	fake := &fakeExecAppRuntime{
		inspectState: &docker.ContainerState{ID: "c1", Running: true},
		execReader:   blocking,
	}
	rt, db := newTestRouterWithExecRuntime(t, fake)
	cookie := loginTestSession(t, rt, db)
	seedExecApp(t, db)

	start := time.Now()
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/exec", `{"command":"sleep","args":["999"],"timeout_seconds":1}`))
	elapsed := time.Since(start)

	if rec.Code != http.StatusGatewayTimeout {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusGatewayTimeout, rec.Body.String())
	}
	if elapsed > 5*time.Second {
		t.Errorf("handler took %s to respond, want well under its own requested 1s timeout (some slack for scheduling)", elapsed)
	}
}

// TestHandleExecApp_StdoutTruncatedButExitCodeStillReal proves
// cappedWriter's whole reason for existing: a command whose stdout
// exceeds execMaxOutputBytes still reports its real exit code, because
// the handler keeps draining the stream to completion (discarding bytes
// past the cap) instead of stopping early the moment the cap is hit,
// which would mean never reaching Exec's trailing error at all.
func TestHandleExecApp_StdoutTruncatedButExitCodeStillReal(t *testing.T) {
	huge := strings.Repeat("x", execMaxOutputBytes+1024)
	execErr := &docker.ExecExitError{Cmd: []string{"yes"}, Container: "c1", ExitCode: 3, Stderr: "too much output"}
	fake := &fakeExecAppRuntime{
		inspectState: &docker.ContainerState{ID: "c1", Running: true},
		execReader:   &errReadCloser{r: strings.NewReader(huge), err: execErr},
	}
	rt, db := newTestRouterWithExecRuntime(t, fake)
	cookie := loginTestSession(t, rt, db)
	seedExecApp(t, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/exec", `{"command":"yes"}`))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got execResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !got.Truncated {
		t.Error("Truncated = false, want true")
	}
	if len(got.Stdout) != execMaxOutputBytes {
		t.Errorf("len(Stdout) = %d, want %d", len(got.Stdout), execMaxOutputBytes)
	}
	if got.ExitCode != 3 {
		t.Errorf("ExitCode = %d, want 3: the real exit code must still surface even though stdout was capped", got.ExitCode)
	}
}

// TestHandleExecApp_NodeResolverError proves a NodeRuntimeResolver
// failure (the app's node is not currently connected, agent.Registry's
// own ErrNodeNotRegistered shape) is reported as a clear 502, not a
// panic or a 500 that looks like this control plane's own fault.
func TestHandleExecApp_NodeResolverError(t *testing.T) {
	db := openTestDB(t)
	resolver := func(string) (docker.Runtime, error) {
		return nil, errors.New("node not registered")
	}
	rt := NewRouter(discardLogger(), testBrand(), db, WithExecRuntime(resolver))
	cookie := loginTestSession(t, rt, db)
	seedExecApp(t, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/exec", `{"command":"echo"}`))
	if rec.Code != http.StatusBadGateway {
		t.Errorf("status = %d, want %d, body = %s", rec.Code, http.StatusBadGateway, rec.Body.String())
	}
}
