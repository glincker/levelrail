package nodemove

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

type fakeExec struct {
	mu        sync.Mutex
	placement map[string]string
	running   map[string]bool
	calls     []string
	fail      map[string]int
	permanent map[string]bool
	readyAt   map[string]int
	readyN    map[string]int
}

func newFakeExec() *fakeExec {
	return &fakeExec{placement: map[string]string{}, running: map[string]bool{}, fail: map[string]int{},
		permanent: map[string]bool{}, readyAt: map[string]int{}, readyN: map[string]int{}}
}

func key(kind, name string) string { return kind + "/" + name }

func (f *fakeExec) call(op string) error {
	f.calls = append(f.calls, op)
	name := strings.SplitN(op, " ", 2)[0]
	if f.fail[name] > 0 {
		f.fail[name]--
		if f.permanent[name] {
			return Permanent(fmt.Errorf("%s refused", name))
		}
		return fmt.Errorf("%s failed", name)
	}
	return nil
}

func (f *fakeExec) Preflight(_ context.Context, m store.NodeMove) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.call("preflight " + m.Resource)
}

func (f *fakeExec) Stop(_ context.Context, kind, name, node string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.call("stop " + name + "@" + node); err != nil {
		return err
	}
	delete(f.running, key(kind, name)+"@"+node)
	return nil
}

func (f *fakeExec) CopyVolume(_ context.Context, from, to, v string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.call("copy " + v + " " + from + ">" + to)
}

func (f *fakeExec) Place(_ context.Context, kind, name, node string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.call("place " + name + "@" + node); err != nil {
		return err
	}
	f.placement[key(kind, name)] = node
	return nil
}

func (f *fakeExec) Resume(_ context.Context, _, name string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.call("resume " + name)
}

func (f *fakeExec) Ready(_ context.Context, kind, name, node string, _ time.Time) (bool, string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.call("ready " + name + "@" + node); err != nil {
		return false, "", err
	}
	k := key(kind, name)
	f.readyN[k]++
	if need, ok := f.readyAt[k]; ok && (need < 0 || f.readyN[k] < need) {
		return false, "probe failing", nil
	}
	return true, "", nil
}

func (f *fakeExec) RemoveFrom(_ context.Context, _, name, node string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.call("remove " + name + "@" + node)
}

func (f *fakeExec) Finalize(_ context.Context, m store.NodeMove) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.call("finalize " + m.Resource)
}

func (f *fakeExec) ops() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]string, 0, len(f.calls))
	for _, c := range f.calls {
		if !strings.HasPrefix(c, "ready ") {
			out = append(out, c)
		}
	}
	return out
}

type clock struct{ t time.Time }

func (c *clock) now() time.Time { return c.t }

func openStore(t *testing.T) *store.DB {
	t.Helper()
	db, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func newRunner(s Store, exec Executor, c *clock, opts ...Option) *Runner {
	n := 0
	ids := func(p string) (string, error) { n++; return fmt.Sprintf("%s_%d", p, n), nil }
	cfg := Config{VerifyTimeout: time.Minute, MaxAttempts: 3, Interval: time.Second}
	return NewRunner(s, exec, cfg, nil, append([]Option{WithClock(c.now), WithIDs(ids)}, opts...)...)
}

func runUntilDone(t *testing.T, r *Runner, s Store, id string, c *clock) store.NodeMove {
	t.Helper()
	for i := 0; i < 50; i++ {
		m, err := r.Advance(context.Background(), id)
		if err != nil {
			t.Fatalf("Advance: %v", err)
		}
		if m.Status != store.NodeMoveStatusRunning {
			return m
		}
		c.t = c.t.Add(10 * time.Second)
	}
	m, _ := s.GetNodeMove(context.Background(), id)
	t.Fatalf("move never finished: %+v", m)
	return m
}

func TestMove_StateMachine(t *testing.T) {
	tests := []struct {
		name       string
		req        Request
		setup      func(*fakeExec)
		wantStatus string
		wantOps    []string
		wantErr    string
	}{
		{
			name:       "stateless app stages on target before stopping the source",
			req:        Request{Kind: "app", Resource: "web", FromNodeID: "a", ToNodeID: "b"},
			setup:      func(f *fakeExec) { f.readyAt["app/web"] = 2 },
			wantStatus: store.NodeMoveStatusSucceeded,
			wantOps:    []string{"preflight web", "place web@b", "remove web@a", "finalize web"},
		},
		{
			name:       "stateful app stops, copies, places, resumes",
			req:        Request{Kind: "app", Resource: "api", FromNodeID: "a", ToNodeID: "b", Volumes: []string{"data", "uploads"}},
			wantStatus: store.NodeMoveStatusSucceeded,
			wantOps:    []string{"preflight api", "stop api@a", "copy data a>b", "copy uploads a>b", "place api@b", "resume api", "finalize api"},
		},
		{
			name:       "database is always stateful",
			req:        Request{Kind: "database", Resource: "main", FromNodeID: "a", ToNodeID: "b", Volumes: []string{"db-main-data"}},
			wantStatus: store.NodeMoveStatusSucceeded,
			wantOps:    []string{"preflight main", "stop main@a", "copy db-main-data a>b", "place main@b", "resume main", "finalize main"},
		},
		{
			name:       "preflight refusal changes nothing",
			req:        Request{Kind: "app", Resource: "web", FromNodeID: "a", ToNodeID: "b"},
			setup:      func(f *fakeExec) { f.fail["preflight"] = 1; f.permanent["preflight"] = true },
			wantStatus: store.NodeMoveStatusFailed,
			wantOps:    []string{"preflight web"},
			wantErr:    "preflight refused",
		},
		{
			name:       "stateless never ready rolls back to the still running source",
			req:        Request{Kind: "app", Resource: "web", FromNodeID: "a", ToNodeID: "b"},
			setup:      func(f *fakeExec) { f.readyAt["app/web"] = -1 },
			wantStatus: store.NodeMoveStatusRolledBack,
			wantOps:    []string{"preflight web", "place web@b", "place web@a", "remove web@b"},
			wantErr:    "not ready on the target node",
		},
		{
			name:       "half succeeded copy rolls back by resuming the untouched source",
			req:        Request{Kind: "app", Resource: "api", FromNodeID: "a", ToNodeID: "b", Volumes: []string{"data"}},
			setup:      func(f *fakeExec) { f.fail["copy"] = 3 },
			wantStatus: store.NodeMoveStatusRolledBack,
			wantOps:    []string{"preflight api", "stop api@a", "copy data a>b", "copy data a>b", "copy data a>b", "resume api"},
			wantErr:    "copy_volumes failed",
		},
		{
			name:       "transient copy failure retries then succeeds",
			req:        Request{Kind: "app", Resource: "api", FromNodeID: "a", ToNodeID: "b", Volumes: []string{"data"}},
			setup:      func(f *fakeExec) { f.fail["copy"] = 1 },
			wantStatus: store.NodeMoveStatusSucceeded,
			wantOps:    []string{"preflight api", "stop api@a", "copy data a>b", "copy data a>b", "place api@b", "resume api", "finalize api"},
		},
		{
			name: "stateful target never ready stops target and resumes on source",
			req:  Request{Kind: "database", Resource: "main", FromNodeID: "a", ToNodeID: "b", Volumes: []string{"v"}},
			setup: func(f *fakeExec) {
				f.readyAt["database/main"] = -1
			},
			wantStatus: store.NodeMoveStatusRolledBack,
			wantOps:    []string{"preflight main", "stop main@a", "copy v a>b", "place main@b", "resume main", "stop main@b", "place main@a", "resume main"},
		},
		{
			name: "rollback failing is reported, not hidden",
			req:  Request{Kind: "app", Resource: "web", FromNodeID: "a", ToNodeID: "b"},
			setup: func(f *fakeExec) {
				f.readyAt["app/web"] = -1
				f.fail["remove"] = 3
			},
			wantStatus: store.NodeMoveStatusFailed,
			wantErr:    "rollback also failed",
		},
		{
			name:       "finalize failure keeps a verified move",
			req:        Request{Kind: "database", Resource: "main", FromNodeID: "a", ToNodeID: "b"},
			setup:      func(f *fakeExec) { f.fail["finalize"] = 1 },
			wantStatus: store.NodeMoveStatusSucceeded,
			wantErr:    "follow-up failed",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := openStore(t)
			f := newFakeExec()
			if tt.setup != nil {
				tt.setup(f)
			}
			c := &clock{t: time.Date(2026, 10, 10, 0, 0, 0, 0, time.UTC)}
			r := newRunner(s, f, c)
			m, err := r.Start(context.Background(), tt.req)
			if err != nil {
				t.Fatalf("Start: %v", err)
			}
			got := runUntilDone(t, r, s, m.ID, c)
			if got.Status != tt.wantStatus {
				t.Fatalf("status = %s (%s), want %s; steps %+v", got.Status, got.Error, tt.wantStatus, got.Steps)
			}
			if tt.wantErr != "" && !strings.Contains(got.Error, tt.wantErr) {
				t.Fatalf("error = %q, want it to contain %q", got.Error, tt.wantErr)
			}
			if tt.wantOps != nil && strings.Join(f.ops(), "|") != strings.Join(tt.wantOps, "|") {
				t.Fatalf("ops = %v\nwant  %v", f.ops(), tt.wantOps)
			}
		})
	}
}

// flakyStore fails the save after a chosen phase, simulating a crash
// between an action and its phase being recorded.
type flakyStore struct {
	*store.DB
	failAfter string
	failed    bool
}

func (s *flakyStore) SaveNodeMove(ctx context.Context, m store.NodeMove) error {
	if !s.failed && len(m.Steps) > 0 && m.Steps[len(m.Steps)-1].Phase == s.failAfter {
		s.failed = true
		return errors.New("disk vanished")
	}
	return s.DB.SaveNodeMove(ctx, m)
}

func TestMove_ResumesAfterRestartMidMove(t *testing.T) {
	db := openStore(t)
	flaky := &flakyStore{DB: db, failAfter: PhaseCopyVolumes}
	f := newFakeExec()
	c := &clock{t: time.Date(2026, 10, 10, 0, 0, 0, 0, time.UTC)}
	first := newRunner(flaky, f, c)
	m, err := first.Start(context.Background(), Request{Kind: "app", Resource: "api", FromNodeID: "a", ToNodeID: "b", Volumes: []string{"data"}})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if _, err := first.Advance(context.Background(), m.ID); err == nil {
		t.Fatal("Advance succeeded, want the injected save failure")
	}

	persisted, _ := db.GetNodeMove(context.Background(), m.ID)
	if persisted.Phase != PhaseCopyVolumes || persisted.Status != store.NodeMoveStatusRunning {
		t.Fatalf("persisted = %s/%s, want copy_volumes/running", persisted.Phase, persisted.Status)
	}

	second := newRunner(db, f, c)
	second.Tick(context.Background())
	got, _ := db.GetNodeMove(context.Background(), m.ID)
	if got.Status != store.NodeMoveStatusSucceeded {
		t.Fatalf("after restart status = %s (%s)", got.Status, got.Error)
	}
	want := "preflight api|stop api@a|copy data a>b|copy data a>b|place api@b|resume api|finalize api"
	if strings.Join(f.ops(), "|") != want {
		t.Fatalf("ops = %v, want the copy replayed once: %s", f.ops(), want)
	}
}

func TestMove_OnePerResource(t *testing.T) {
	s := openStore(t)
	c := &clock{t: time.Now()}
	r := newRunner(s, newFakeExec(), c)
	req := Request{Kind: "app", Resource: "web", FromNodeID: "a", ToNodeID: "b"}
	if _, err := r.Start(context.Background(), req); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if _, err := r.Start(context.Background(), req); !errors.Is(err, store.ErrNodeMoveInProgress) {
		t.Fatalf("second Start error = %v, want ErrNodeMoveInProgress", err)
	}
	if _, err := r.Start(context.Background(), Request{Kind: "app", Resource: "x", FromNodeID: "a", ToNodeID: "a"}); !IsPermanent(err) {
		t.Fatalf("same-node Start error = %v, want permanent", err)
	}
}

func TestConfigFromEnv(t *testing.T) {
	env := map[string]string{"APP_NODE_MOVE_VERIFY_TIMEOUT": "90s", "APP_NODE_MOVE_MAX_ATTEMPTS": "nope", "APP_NODE_MOVE_INTERVAL": "500ms"}
	cfg := ConfigFromEnv(func(k string) (string, bool) { v, ok := env[k]; return v, ok })
	if cfg.VerifyTimeout != 90*time.Second || cfg.MaxAttempts != 5 || cfg.Interval != 500*time.Millisecond {
		t.Fatalf("cfg = %+v", cfg)
	}
}
