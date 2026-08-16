package api

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/build"
	"github.com/GLINCKER/levelrail/internal/deploy"
	"github.com/GLINCKER/levelrail/internal/deploylog"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/telemetry"
)

func TestHandleListDeployAttempts(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()

	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "web", Image: "levelrail/web:1", Port: 3000}); err != nil {
		t.Fatalf("seed app: %v", err)
	}

	// No attempts yet: empty array, not null, not an error.
	recEmpty := httptest.NewRecorder()
	rt.Handler().ServeHTTP(recEmpty, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/deploy-attempts", ""))
	if recEmpty.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recEmpty.Code, http.StatusOK)
	}
	if strings.TrimSpace(recEmpty.Body.String()) != "[]" {
		t.Errorf("body = %q, want an empty JSON array", recEmpty.Body.String())
	}

	base := time.Now().UTC().Truncate(time.Millisecond)
	if err := db.SaveDeployAttempt(ctx, store.DeployAttempt{
		ID: "dep_1", ServiceName: "web", Image: "levelrail/web:1",
		CommitSHA: "sha1", Source: store.DeployAttemptSourceWebhook,
		Status: store.DeployAttemptStatusSucceeded, StartedAt: base,
	}); err != nil {
		t.Fatalf("seed attempt: %v", err)
	}
	if err := db.SaveDeployAttempt(ctx, store.DeployAttempt{
		ID: "dep_2", ServiceName: "web", Image: "levelrail/web:2",
		Source: store.DeployAttemptSourceImage,
		Status: store.DeployAttemptStatusFailed, StartedAt: base.Add(time.Minute),
	}); err != nil {
		t.Fatalf("seed attempt: %v", err)
	}
	if err := db.FinishDeployAttempt(ctx, "dep_2", store.DeployAttemptStatusFailed, base.Add(2*time.Minute), "build failed"); err != nil {
		t.Fatalf("finish attempt: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/deploy-attempts", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var got []deployAttemptResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d attempts, want 2", len(got))
	}
	// Newest first.
	if got[0].ID != "dep_2" || got[1].ID != "dep_1" {
		t.Errorf("got IDs [%s, %s], want [dep_2, dep_1]", got[0].ID, got[1].ID)
	}
	if got[0].Status != store.DeployAttemptStatusFailed || got[0].Error != "build failed" {
		t.Errorf("got[0] = %+v, want status=failed error='build failed'", got[0])
	}
	if got[0].Source != store.DeployAttemptSourceImage || got[0].CommitSHA != "" {
		t.Errorf("got[0] source/commit = %q/%q, want %q/empty", got[0].Source, got[0].CommitSHA, store.DeployAttemptSourceImage)
	}
	if got[1].Source != store.DeployAttemptSourceWebhook || got[1].CommitSHA != "sha1" {
		t.Errorf("got[1] source/commit = %q/%q, want %q/sha1", got[1].Source, got[1].CommitSHA, store.DeployAttemptSourceWebhook)
	}
	if got[0].FinishedAt == nil {
		t.Error("got[0].FinishedAt = nil, want set")
	}
	if got[1].FinishedAt != nil {
		t.Error("got[1].FinishedAt != nil, want nil for a never-finished row")
	}
}

func TestHandleListDeployAttempts_AppNotFound(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/ghost/deploy-attempts", ""))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestHandleListDeployAttempts_RequiresAuth(t *testing.T) {
	rt, _ := newTestRouter(t)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/apps/web/deploy-attempts", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestHandleDeployLogStream_AppNotFound(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/ghost/deploys/dep_1/logs", ""))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestHandleDeployLogStream_AttemptNotFound(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	if err := db.SaveDesiredService(context.Background(), store.DesiredService{Name: "web", Image: "levelrail/web:1", Port: 3000}); err != nil {
		t.Fatalf("seed app: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/deploys/dep_ghost/logs", ""))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestHandleDeployLogStream_AttemptBelongsToDifferentApp(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()
	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "web", Image: "levelrail/web:1", Port: 3000}); err != nil {
		t.Fatalf("seed app: %v", err)
	}
	if err := db.SaveDeployAttempt(ctx, store.DeployAttempt{
		ID: "dep_other", ServiceName: "worker", Image: "worker:1",
		Status: store.DeployAttemptStatusSucceeded, StartedAt: time.Now(),
	}); err != nil {
		t.Fatalf("seed attempt: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/deploys/dep_other/logs", ""))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestHandleDeployLogStream_FinishedAttempt_ReplaysPersistedLog(t *testing.T) {
	db := openTestDB(t)
	fakeLogStore := &fakeDeployLogStore{
		entries: map[string][]sseTestEntry{
			"dep_done": {
				{Message: "line one", Stream: "stdout"},
				{Message: "line two", Stream: "stderr"},
			},
		},
	}
	rt := NewRouter(nil, testBrand(), db, WithDeployLogStore(fakeLogStore))
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()

	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "web", Image: "levelrail/web:1", Port: 3000}); err != nil {
		t.Fatalf("seed app: %v", err)
	}
	if err := db.SaveDeployAttempt(ctx, store.DeployAttempt{
		ID: "dep_done", ServiceName: "web", Image: "levelrail/web:2",
		Status: store.DeployAttemptStatusRunning, StartedAt: time.Now(),
	}); err != nil {
		t.Fatalf("seed attempt: %v", err)
	}
	if err := db.FinishDeployAttempt(ctx, "dep_done", store.DeployAttemptStatusSucceeded, time.Now(), ""); err != nil {
		t.Fatalf("finish attempt: %v", err)
	}

	// serveFinishedDeployLog holds the connection open (writing nothing
	// further) after its one real burst, rather than returning
	// immediately: see that function's own doc comment for why (a real
	// EventSource client reconnects on a server-initiated close, and
	// this task's own live browser verification caught that reconnect
	// replaying, and re-appending, the entire persisted log on a loop).
	// So ServeHTTP itself only returns once the request's context is
	// done; a short timeout here plays the client-disconnect role a
	// real browser navigating away would, single-goroutine and
	// synchronous (no separate goroutine/race concern: ServeHTTP simply
	// blocks this call for up to the timeout, then returns).
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	req := authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/deploys/dep_done/logs", "").WithContext(ctx)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); ct != "text/event-stream" {
		t.Errorf("Content-Type = %q, want %q", ct, "text/event-stream")
	}
	body := rec.Body.String()
	if !strings.Contains(body, `"line":"line one"`) || !strings.Contains(body, `"stream":"stdout"`) {
		t.Errorf("body missing line one: %q", body)
	}
	if !strings.Contains(body, `"line":"line two"`) || !strings.Contains(body, `"stream":"stderr"`) {
		t.Errorf("body missing line two: %q", body)
	}
}

func TestHandleDeployLogStream_FinishedAttempt_NoLogStoreConfigured(t *testing.T) {
	rt, db := newTestRouter(t) // no WithDeployLogStore
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()

	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "web", Image: "levelrail/web:1", Port: 3000}); err != nil {
		t.Fatalf("seed app: %v", err)
	}
	if err := db.SaveDeployAttempt(ctx, store.DeployAttempt{
		ID: "dep_done", ServiceName: "web", Image: "levelrail/web:2",
		Status: store.DeployAttemptStatusSucceeded, StartedAt: time.Now(),
	}); err != nil {
		t.Fatalf("seed attempt: %v", err)
	}
	if err := db.FinishDeployAttempt(ctx, "dep_done", store.DeployAttemptStatusSucceeded, time.Now(), ""); err != nil {
		t.Fatalf("finish attempt: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/deploys/dep_done/logs", ""))
	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotImplemented)
	}
}

func TestHandleDeployLogStream_InProgressAttempt_NoRecorderConfigured(t *testing.T) {
	rt, db := newTestRouter(t) // no WithDeployRecorder
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()

	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "web", Image: "levelrail/web:1", Port: 3000}); err != nil {
		t.Fatalf("seed app: %v", err)
	}
	if err := db.SaveDeployAttempt(ctx, store.DeployAttempt{
		ID: "dep_running", ServiceName: "web", Image: "levelrail/web:2",
		Status: store.DeployAttemptStatusRunning, StartedAt: time.Now(),
	}); err != nil {
		t.Fatalf("seed attempt: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/deploys/dep_running/logs", ""))
	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotImplemented)
	}
}

// TestHandleDeployLogStream_LiveTail is the one genuinely concurrent
// test in this file: it exercises the actual live fan-out path end to
// end (Snapshot's lines-so-far replay, then a new event published after
// the client subscribed, then Finish closing the stream), over a real
// HTTP connection (httptest.NewServer, not httptest.NewRecorder) because
// the handler and this test both read/write the response concurrently,
// which a shared bytes.Buffer (what NewRecorder backs onto) cannot do
// without a data race.
func TestHandleDeployLogStream_LiveTail(t *testing.T) {
	db := openTestDB(t)
	recorder := deploylog.NewRecorder(nil, discardLogger())
	rt := NewRouter(nil, testBrand(), db, WithDeployRecorder(recorder))
	ctx := context.Background()

	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "web", Image: "levelrail/web:1", Port: 3000}); err != nil {
		t.Fatalf("seed app: %v", err)
	}
	if err := db.SaveDeployAttempt(ctx, store.DeployAttempt{
		ID: "dep_live", ServiceName: "web", Image: "levelrail/web:2",
		Status: store.DeployAttemptStatusRunning, StartedAt: time.Now(),
	}); err != nil {
		t.Fatalf("seed attempt: %v", err)
	}

	recorder.Start("dep_live")
	progress := recorder.Progress("dep_live")
	progress(build.ProgressEvent{Log: "line one", Stream: "stdout"})

	srv := httptest.NewServer(rt.Handler())
	defer srv.Close()
	cookie := loginViaServer(t, srv, db)

	// A cancellable request context, not http.NewRequest's default: this
	// test needs to explicitly end the client side of the connection
	// once it's done reading, playing the role a real browser tab
	// closing/navigating away would. That's required now that
	// serveLiveDeployLog holds the connection open after the live
	// channel closes (see that function's own doc comment) instead of
	// returning immediately: without an explicit client-side cancel,
	// the server would keep waiting on this request's context forever
	// and this test would hang.
	reqCtx, reqCancel := context.WithCancel(context.Background())
	defer reqCancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, srv.URL+"/api/v1/apps/web/deploys/dep_live/logs", nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.AddCookie(cookie)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	reader := bufio.NewReader(resp.Body)

	first, err := nextSSEData(reader)
	if err != nil {
		t.Fatalf("read first event: %v", err)
	}
	if !strings.Contains(first, `"line":"line one"`) {
		t.Fatalf("first event = %q, want it to contain line one", first)
	}

	progress(build.ProgressEvent{Log: "line two", Stream: "stderr"})

	second, err := nextSSEData(reader)
	if err != nil {
		t.Fatalf("read second (live) event: %v", err)
	}
	if !strings.Contains(second, `"line":"line two"`) || !strings.Contains(second, `"stream":"stderr"`) {
		t.Fatalf("second event = %q, want line two/stderr", second)
	}

	recorder.Finish(ctx, "dep_live")

	// The connection must NOT close (and so the client must NOT see an
	// EOF or get a chance to reconnect) just because the attempt
	// finished: see serveLiveDeployLog's own doc comment for why a
	// server-initiated close here is exactly the bug this task's live
	// browser verification caught (a reconnecting EventSource replaying,
	// and re-appending, the whole persisted log on a loop). So the
	// correct behavior is "no more data arrives, but the read does not
	// return an error either," checked with a bounded wait since there's
	// no event to synchronize on for a thing that's supposed to *not*
	// happen.
	readErrCh := make(chan error, 1)
	go func() {
		_, readErr := nextSSEData(reader)
		readErrCh <- readErr
	}()
	select {
	case readErr := <-readErrCh:
		t.Errorf("read returned (err=%v) shortly after Finish, want the connection to stay open with no more data", readErr)
	case <-time.After(300 * time.Millisecond):
		// Expected: still open, no more data, no error.
	}

	// Now play the client-disconnect role explicitly: cancelling the
	// request context should make the server observe r.Context().Done()
	// and return, closing the connection from the server's side too.
	reqCancel()
	select {
	case <-readErrCh:
		// The background reader's blocked Read finally unblocked with an
		// error once the connection was torn down, as expected.
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for the connection to close after the client cancelled its request context")
	}
}

// nextSSEData reads lines until it finds a non-empty "data: " line,
// skipping blank lines (SSE's own event separator).
func nextSSEData(r *bufio.Reader) (string, error) {
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return "", err
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			continue
		}
		if data, ok := strings.CutPrefix(line, "data: "); ok {
			return data, nil
		}
	}
}

// loginViaServer bootstraps the admin account and logs in through a real
// HTTP round trip against srv, returning the session cookie: needed for
// TestHandleDeployLogStream_LiveTail since that test talks to the router
// over a real listener, not httptest.NewRecorder.
func loginViaServer(t *testing.T, srv *httptest.Server, db *store.DB) *http.Cookie {
	t.Helper()
	bootstrapTestAdmin(t, db)

	body := strings.NewReader(`{"username":"` + testAdminUsername + `","password":"` + testAdminPassword + `"}`)
	resp, err := http.Post(srv.URL+"/api/v1/auth/login", "application/json", body)
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("login status = %d", resp.StatusCode)
	}
	for _, c := range resp.Cookies() {
		if c.Name == sessionCookieName {
			return c
		}
	}
	t.Fatal("login: no session cookie returned")
	return nil
}

// fakeDeployLogStore is a hand-written fake for DeployLogStore.
type fakeDeployLogStore struct {
	entries map[string][]sseTestEntry
}

type sseTestEntry struct {
	Message string
	Stream  string
}

func (f *fakeDeployLogStore) QueryDeployLog(_ context.Context, attemptID string) ([]telemetry.DeployLogEntry, error) {
	var out []telemetry.DeployLogEntry
	for _, e := range f.entries[attemptID] {
		out = append(out, telemetry.DeployLogEntry{AttemptID: attemptID, Stream: e.Stream, Message: e.Message})
	}
	return out, nil
}

// orderCheckAttemptStore wraps a real DeployAttemptStore and, during
// SaveDeployAttempt, snapshots whether recorder already knows the
// attempt being saved. A GET can only ever discover an attempt ID after
// SaveDeployAttempt returns, so this proves beginBuildDeployAttempt calls
// Recorder.Start strictly before that, not just usually.
type orderCheckAttemptStore struct {
	DeployAttemptStore
	recorder          *deploylog.Recorder
	saveErr           error
	lastID            string
	startedBeforeSave bool
}

func (f *orderCheckAttemptStore) SaveDeployAttempt(ctx context.Context, a store.DeployAttempt) error {
	f.lastID = a.ID
	if _, _, unsubscribe, ok := f.recorder.Snapshot(a.ID); ok {
		f.startedBeforeSave = true
		unsubscribe()
	}
	if f.saveErr != nil {
		return f.saveErr
	}
	return f.DeployAttemptStore.SaveDeployAttempt(ctx, a)
}

func TestBeginBuildDeployAttempt_StartRunsBeforeAttemptIsSaveable(t *testing.T) {
	rt, db := newTestRouterWithBuilder(t, newFakeBuilder("web:1", nil), nil)
	recorder := deploylog.NewRecorder(nil, discardLogger())
	rt.deployRecorder = recorder
	fake := &orderCheckAttemptStore{DeployAttemptStore: rt.deployAttempts, recorder: recorder}
	rt.deployAttempts = fake

	ctx := context.Background()
	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "web", Image: "levelrail/web:1", Port: 3000}); err != nil {
		t.Fatalf("seed app: %v", err)
	}

	_, _, finish := rt.beginBuildDeployAttempt(ctx, deploy.Request{ServiceName: "web", ImageRepo: "web", CommitSHA: "sha1"}, store.DeployAttemptSourceManual)
	finish(nil)

	if !fake.startedBeforeSave {
		t.Error("recorder.Snapshot(id) was not yet ok during SaveDeployAttempt: Start ran after the row became visible to a GET, reopening the race a client could hit right after the save")
	}
}

func TestBeginBuildDeployAttempt_SaveFails_RecorderDoesNotLeak(t *testing.T) {
	rt, db := newTestRouterWithBuilder(t, newFakeBuilder("web:1", nil), nil)
	recorder := deploylog.NewRecorder(nil, discardLogger())
	rt.deployRecorder = recorder
	fake := &orderCheckAttemptStore{DeployAttemptStore: rt.deployAttempts, recorder: recorder, saveErr: errors.New("db write failed")}
	rt.deployAttempts = fake

	ctx := context.Background()
	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "web", Image: "levelrail/web:1", Port: 3000}); err != nil {
		t.Fatalf("seed app: %v", err)
	}

	rt.beginBuildDeployAttempt(ctx, deploy.Request{ServiceName: "web", ImageRepo: "web", CommitSHA: "sha1"}, store.DeployAttemptSourceManual)

	if fake.lastID == "" {
		t.Fatal("SaveDeployAttempt was never called")
	}
	if _, _, unsubscribe, ok := recorder.Snapshot(fake.lastID); ok {
		unsubscribe()
		t.Error("recorder still has an active entry for id after SaveDeployAttempt failed: Start's registration leaked instead of being cleaned up by Finish")
	}
}
