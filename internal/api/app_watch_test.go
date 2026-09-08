package api

import (
	"bufio"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/reconcile"
	"github.com/GLINCKER/levelrail/internal/store"
)

// fakeConditionStore is a hand-written DeployStore fake whose
// GetConditions returns the next entry of a fixed sequence on each call
// (clamped to the last entry once exhausted), so a test can control
// exactly when the "observed" conditions change from one poll to the
// next.
type fakeConditionStore struct {
	mu       sync.Mutex
	sequence [][]reconcile.Condition
	calls    int
}

func (f *fakeConditionStore) GetConditions(_ context.Context, _ string) ([]reconcile.Condition, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	idx := f.calls
	if idx >= len(f.sequence) {
		idx = len(f.sequence) - 1
	}
	f.calls++
	return f.sequence[idx], nil
}

func (f *fakeConditionStore) GetConditionsForControllers(_ context.Context, _ []string) (map[string][]reconcile.Condition, error) {
	return nil, nil
}

func TestHandleAppWatch_AppNotFound(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/ghost/watch", ""))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestHandleDatabaseWatch_DatabaseNotFound(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/databases/ghost/watch", ""))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

// TestHandleAppWatch_EmitsOnlyOnChange is the one genuinely concurrent
// test in this file, following TestHandleDeployLogStream_LiveTail's own
// real-HTTP-connection pattern (httptest.NewServer, not
// httptest.NewRecorder): the handler and this test both read/write the
// response concurrently, which a shared bytes.Buffer cannot do without a
// data race.
//
// It asserts all three behaviors this endpoint promises: an unchanged
// condition set produces no extra event across several polls, a changed
// one produces a new event with the right payload, and cancelling the
// client's request context makes the server observe r.Context().Done()
// and stop.
func TestHandleAppWatch_EmitsOnlyOnChange(t *testing.T) {
	db := openTestDB(t)
	rt := NewRouter(nil, testBrand(), db, WithWatchPollInterval(10*time.Millisecond))

	if err := db.SaveDesiredService(context.Background(), store.DesiredService{Name: "web", Image: "levelrail/web:1", Port: 3000}); err != nil {
		t.Fatalf("seed app: %v", err)
	}

	ready := reconcile.Condition{Type: "Ready", Status: reconcile.ConditionFalse, Reason: "Deploying", Message: "rolling out"}
	readyTrue := reconcile.Condition{Type: "Ready", Status: reconcile.ConditionTrue, Reason: "Deployed", Message: "up"}
	fake := &fakeConditionStore{sequence: [][]reconcile.Condition{
		{ready}, {ready}, {ready}, // three unchanged polls
		{readyTrue}, {readyTrue}, // then a change, held steady
	}}
	rt.deploys = fake

	srv := httptest.NewServer(rt.Handler())
	defer srv.Close()
	cookie := loginViaServer(t, srv, db)

	reqCtx, reqCancel := context.WithCancel(context.Background())
	defer reqCancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, srv.URL+"/api/v1/apps/web/watch", nil)
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
	if !strings.Contains(first, `"Reason":"Deploying"`) {
		t.Fatalf("first event = %q, want it to contain the initial Deploying condition", first)
	}

	// The second real event must be the changed condition, not a repeat
	// of the first: however many unchanged polls happened in between,
	// exactly one event should have been buffered for this read.
	second, err := nextSSEData(reader)
	if err != nil {
		t.Fatalf("read second event: %v", err)
	}
	if !strings.Contains(second, `"Reason":"Deployed"`) || !strings.Contains(second, `"Status":"True"`) {
		t.Fatalf("second event = %q, want the Deployed/True condition", second)
	}

	// No further changes are queued, so no third event should ever
	// arrive; confirmed with a bounded wait since there is no event to
	// synchronize on for a thing that's supposed to *not* happen (same
	// reasoning TestHandleDeployLogStream_LiveTail's own post-Finish
	// check documents).
	readErrCh := make(chan error, 1)
	go func() {
		_, readErr := nextSSEData(reader)
		readErrCh <- readErr
	}()
	select {
	case readErr := <-readErrCh:
		t.Errorf("read returned (err=%v) with no further condition change queued, want the connection to stay open with no more data", readErr)
	case <-time.After(150 * time.Millisecond):
		// Expected: still open, no more data, no error.
	}

	reqCancel()
	select {
	case <-readErrCh:
		// The background reader's blocked Read finally unblocked with an
		// error once the connection was torn down, as expected.
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for the connection to close after the client cancelled its request context")
	}
}
