package application

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/store"
)

func TestHeldFilterKeepsOnlyNewestPreviousRelease(t *testing.T) {
	now := time.Now()
	c := New("web", &fakeStore{}, newFakeRuntime(0), WithPreviousReleaseHold(time.Minute))
	cur := ContainerName("web", "img:v3", "")
	v2 := ContainerName("web", "img:v2", "")
	v1 := ContainerName("web", "img:v1", "")
	tests := []struct {
		name      string
		all       []docker.ContainerState
		targets   []string
		wantDue   []string
		wantHeld  bool
		heldAfter time.Duration
	}{
		{
			name: "older running release is removed as soon as a newer previous exists",
			all: []docker.ContainerState{
				{ID: "c", Name: cur, Running: true, Created: now.Add(-5 * time.Second)},
				{ID: "2", Name: v2, Running: true, Created: now.Add(-20 * time.Second)},
				{ID: "1", Name: v1, Running: true, Created: now.Add(-40 * time.Second)},
			},
			wantDue: []string{"1"}, wantHeld: true, heldAfter: 55 * time.Second,
		},
		{
			name: "scaling up the current release does not extend the window",
			all: []docker.ContainerState{
				{ID: "c", Name: cur, Running: true, Created: now.Add(-50 * time.Second)},
				{ID: "c2", Name: cur + "-r1", Running: true, Created: now.Add(-1 * time.Second)},
				{ID: "2", Name: v2, Running: true, Created: now.Add(-90 * time.Second)},
			},
			targets: []string{cur, cur + "-r1"},
			wantDue: nil, wantHeld: true, heldAfter: 10 * time.Second,
		},
		{
			name: "replicas of the held release stay together",
			all: []docker.ContainerState{
				{ID: "c", Name: cur, Running: true, Created: now.Add(-5 * time.Second)},
				{ID: "2", Name: v2, Running: true, Created: now.Add(-20 * time.Second)},
				{ID: "2b", Name: v2 + "-r1", Running: true, Created: now.Add(-20 * time.Second)},
			},
			wantDue: nil, wantHeld: true, heldAfter: 55 * time.Second,
		},
		{
			name: "nothing held once the window passed",
			all: []docker.ContainerState{
				{ID: "c", Name: cur, Running: true, Created: now.Add(-2 * time.Minute)},
				{ID: "2", Name: v2, Running: true, Created: now.Add(-3 * time.Minute)},
			},
			wantDue: []string{"2"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			targets := tt.targets
			if targets == nil {
				targets = []string{cur}
			}
			due, until := c.heldFilter(tt.all, targets, now)
			var got []string
			for _, cs := range due {
				got = append(got, cs.ID)
			}
			if fmt.Sprint(got) != fmt.Sprint(tt.wantDue) {
				t.Fatalf("due = %v, want %v", got, tt.wantDue)
			}
			if (!until.IsZero()) != tt.wantHeld {
				t.Fatalf("heldUntil = %v, want held=%v", until, tt.wantHeld)
			}
			if tt.wantHeld {
				if d := until.Sub(now) - tt.heldAfter; d < -time.Second || d > time.Second {
					t.Fatalf("heldUntil in %v, want about %v", until.Sub(now), tt.heldAfter)
				}
			}
		})
	}
}

func TestReconcile_RapidIterationKeepsAtMostTwoContainers(t *testing.T) {
	rt := newDigestRuntime(map[string]string{})
	desired := &store.DesiredService{Name: "web", Image: "img:v0", Port: 80}
	rec := &fakeApplied{}
	c := New("web", &fakeStore{svc: desired}, rt, WithPreviousReleaseHold(3*time.Minute), WithAppliedConfigRecorder(rec))
	start := time.Now()
	for i := 1; i <= 5; i++ {
		desired.Image = fmt.Sprintf("img:v%d", i)
		rt.now = start.Add(time.Duration(i*10) * time.Second)
		if _, err := c.Reconcile(context.Background()); err != nil {
			t.Fatal(err)
		}
		if rt.count() > 2 {
			t.Fatalf("after deploy %d: containers = %v, want at most 2", i, rt.names())
		}
	}
	if rt.count() != 2 {
		t.Fatalf("containers = %v, want current plus previous", rt.names())
	}
	if rec.heldUntil().IsZero() {
		t.Fatal("hold not published for the status payload")
	}
}

func TestReconcile_RecordsAppliedConfigOnCreate(t *testing.T) {
	rt := newDigestRuntime(map[string]string{})
	desired := &store.DesiredService{Name: "web", Image: "img:v1", Port: 80, Env: map[string]string{"A": "1"}}
	rec := &fakeApplied{}
	c := New("web", &fakeStore{svc: desired}, rt, WithAppliedConfigRecorder(rec))
	if _, err := c.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	got := rec.last()
	if got.Release != ContainerName("web", "img:v1", "") {
		t.Fatalf("release = %q", got.Release)
	}
	if got.EnvHashes["A"] != store.HashEnvValue("web", "A", "1") || got.Fields[store.AppliedFieldPort] != "80" {
		t.Fatalf("snapshot = %+v", got)
	}
	// No-op reconcile records nothing new.
	calls := rec.calls()
	if _, err := c.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	if rec.calls() != calls {
		t.Fatal("steady state re-recorded the snapshot")
	}
}

type fakeApplied struct {
	mu    sync.Mutex
	saved []store.AppliedConfig
	held  time.Time
}

func (f *fakeApplied) SaveAppliedConfig(_ context.Context, c store.AppliedConfig) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.saved = append(f.saved, c)
	return nil
}

func (f *fakeApplied) SetPreviousReleaseHeldUntil(_ context.Context, _ string, until time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.held = until
	return nil
}

func (f *fakeApplied) heldUntil() time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.held
}

func (f *fakeApplied) calls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.saved)
}

func (f *fakeApplied) last() store.AppliedConfig {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.saved) == 0 {
		return store.AppliedConfig{}
	}
	return f.saved[len(f.saved)-1]
}
