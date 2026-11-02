package selfupgrade

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func stallServer(t *testing.T, trickle bool) *httptest.Server {
	t.Helper()
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "1000000")
		_, _ = w.Write([]byte("partial"))
		w.(http.Flusher).Flush()
		for {
			select {
			case <-r.Context().Done():
				return
			case <-release:
				return
			case <-time.After(20 * time.Millisecond):
				if trickle {
					_, _ = w.Write([]byte("x"))
					w.(http.Flusher).Flush()
				}
			}
		}
	}))
	t.Cleanup(func() { close(release); srv.Close() })
	return srv
}

func TestFetchStallFailsAndCleansUp(t *testing.T) {
	srv := stallServer(t, false)
	var seen atomic.Int64
	ctx := WithProgress(context.Background(), func(p Progress) { seen.Store(p.Bytes) })
	dir := filepath.Join(t.TempDir(), "w")
	start := time.Now()
	_, err := Fetcher{BaseURL: srv.URL + "/dl", AllowHTTP: true, Stall: 200 * time.Millisecond, Deadline: time.Minute}.
		Fetch(ctx, "v1.2.3", "levelrail-linux-amd64", dir)
	if !errors.Is(err, ErrStalled) {
		t.Fatalf("err = %v, want ErrStalled", err)
	}
	if time.Since(start) > 5*time.Second {
		t.Fatalf("stall took %s to detect", time.Since(start))
	}
	if seen.Load() != int64(len("partial")) {
		t.Fatalf("progress bytes = %d", seen.Load())
	}
	if left, _ := filepath.Glob(filepath.Join(dir, "*")); len(left) != 0 {
		t.Fatalf("partial files left behind: %v", left)
	}
}

func TestFetchDeadlineCapsASlowTrickle(t *testing.T) {
	srv := stallServer(t, true)
	_, err := Fetcher{BaseURL: srv.URL + "/dl", AllowHTTP: true, Stall: 5 * time.Second, Deadline: 300 * time.Millisecond}.
		Fetch(context.Background(), "v1.2.3", "levelrail-linux-amd64", t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "time limit") {
		t.Fatalf("err = %v, want the overall time limit", err)
	}
}

func TestApplyCancelDuringDownload(t *testing.T) {
	h := newFakeHost(t)
	d := h.deps()
	d.Limits = Limits{Stall: time.Minute, Deadline: time.Minute, Heartbeat: 10 * time.Millisecond, OrphanAfter: time.Minute}
	started := make(chan struct{})
	d.Fetch = func(ctx context.Context, _, asset, dir string) (Fetched, error) {
		_ = os.MkdirAll(dir, 0o700)
		h.write(filepath.Join(dir, asset+".part"), "half")
		close(started)
		<-ctx.Done()
		return Fetched{}, context.Cause(ctx)
	}
	go func() {
		<-started
		_ = d.Journal.RequestCancel(req().ID)
	}()
	a, err := Apply(context.Background(), d, req())
	if !errors.Is(err, ErrCancelled) {
		t.Fatalf("err = %v", err)
	}
	if a.Outcome != OutcomeFailed || a.FailedStep != StepDownload {
		t.Fatalf("outcome %s at %s", a.Outcome, a.FailedStep)
	}
	if h.stops != 0 || h.read(h.bin) != "OLD" {
		t.Fatalf("host touched: stops=%d bin=%q", h.stops, h.read(h.bin))
	}
	if _, serr := os.Stat(d.WorkDir); !os.IsNotExist(serr) {
		t.Fatalf("work directory kept: %v", serr)
	}
	if d.Journal.CancelRequested(req().ID) {
		t.Fatal("cancel marker not cleared")
	}
}

func TestCancelRefusedOnceApplying(t *testing.T) {
	for _, step := range []string{StepStop, StepSwap, StepStart, StepHealth, StepRollback} {
		if CanCancel(Attempt{Outcome: OutcomeRunning, Current: step}) {
			t.Fatalf("%s must not be cancellable", step)
		}
	}
	if CanCancel(Attempt{Outcome: OutcomeRunning, Steps: []StepRecord{{Name: StepStop, Status: StatusOK}}}) {
		t.Fatal("an attempt that already stopped the service must not be cancellable")
	}
	if !CanCancel(Attempt{Outcome: OutcomeRunning, Current: StepDownload}) {
		t.Fatal("download must be cancellable")
	}
	r := &runner{}
	_, r.cancel = context.WithCancelCause(context.Background())
	if err := r.commit(); err != nil {
		t.Fatal(err)
	}
	if r.tryCancel() {
		t.Fatal("cancel must lose once the host is being changed")
	}
}

func TestReapOrphansAtStartup(t *testing.T) {
	dir := t.TempDir()
	j := NewJournal(dir)
	work := filepath.Join(dir, WorkDirName)
	if err := os.MkdirAll(work, 0o700); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	l := Limits{Stall: time.Minute, Deadline: time.Minute, Heartbeat: 10 * time.Second, OrphanAfter: 90 * time.Second}
	old := now.Add(-time.Hour)
	save := func(a Attempt) {
		t.Helper()
		if err := j.Save(a); err != nil {
			t.Fatal(err)
		}
	}
	save(Attempt{ID: "dead-download", Outcome: OutcomeRunning, Current: StepDownload, StartedAt: old, Steps: []StepRecord{{Name: StepGuard, Status: StatusOK}}})
	save(Attempt{ID: "dead-after-stop", Outcome: OutcomeRunning, Current: StepSwap, StartedAt: old, Steps: []StepRecord{{Name: StepStop, Status: StatusOK}}})
	save(Attempt{ID: "alive", Outcome: OutcomeRunning, Current: StepDownload, StartedAt: old})
	if err := j.WriteHeartbeat("alive", Heartbeat{At: now}); err != nil {
		t.Fatal(err)
	}

	closed, err := ReapOrphans(context.Background(), j, work, now, l)
	if err != nil {
		t.Fatal(err)
	}
	if len(closed) != 1 || closed[0].ID != "dead-download" {
		t.Fatalf("closed = %+v", closed)
	}
	got, _ := j.List()
	state := map[string]string{}
	for _, a := range got {
		state[a.ID] = a.Outcome
	}
	if state["dead-download"] != OutcomeFailed || state["dead-after-stop"] != OutcomeRunning || state["alive"] != OutcomeRunning {
		t.Fatalf("state = %v", state)
	}
	for _, a := range got {
		if a.ID == "dead-download" && (a.FailedStep != StepDownload || !strings.Contains(a.Error, "Retry")) {
			t.Fatalf("failed attempt = %+v", a)
		}
	}
	if _, serr := os.Stat(work); !os.IsNotExist(serr) {
		t.Fatal("partial download directory kept")
	}
}
