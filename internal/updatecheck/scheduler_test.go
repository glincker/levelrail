package updatecheck

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/upgrade"
	"github.com/GLINCKER/levelrail/internal/version"
)

// fakeStore is a Store that can be told to fail, matching this
// codebase's own rule for testing a periodic background check: "get a
// test for the case where the operation half-succeeded," applied here
// to a settings read instead of a reconciler op.
type fakeStore struct {
	settings store.UpdateSettings
	err      error
}

func (f *fakeStore) GetUpdateSettings(context.Context) (store.UpdateSettings, error) {
	return f.settings, f.err
}

func fakeFetchers(release *upgrade.Release, err error) upgrade.Fetchers {
	fn := func(context.Context) (*upgrade.Release, error) { return release, err }
	return upgrade.Fetchers{Stable: fn, Beta: fn, Edge: fn}
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestScheduler_Tick_SkipsWhenDisabled(t *testing.T) {
	st := &fakeStore{settings: store.UpdateSettings{Channel: "stable", AutoUpdateEnabled: false}}
	called := false
	sched := NewScheduler(st, discardLogger())
	sched.Fetchers = upgrade.Fetchers{
		Stable: func(context.Context) (*upgrade.Release, error) { called = true; return nil, nil },
		Beta:   func(context.Context) (*upgrade.Release, error) { called = true; return nil, nil },
		Edge:   func(context.Context) (*upgrade.Release, error) { called = true; return nil, nil },
	}

	if err := sched.Tick(context.Background()); err != nil {
		t.Fatalf("Tick() error = %v, want nil", err)
	}
	if called {
		t.Error("a channel fetcher was called while auto_update_enabled is false, want no GitHub lookup at all")
	}
	if got := sched.Result(); got != (Result{}) {
		t.Errorf("Result() = %+v, want zero value when disabled", got)
	}
}

func TestScheduler_Tick_StoreErrorPropagates(t *testing.T) {
	st := &fakeStore{err: errors.New("db unavailable")}
	sched := NewScheduler(st, discardLogger())

	if err := sched.Tick(context.Background()); err == nil {
		t.Fatal("Tick() error = nil, want a wrapped store error")
	}
}

func TestScheduler_Tick_FetchErrorPropagates(t *testing.T) {
	st := &fakeStore{settings: store.UpdateSettings{Channel: "stable", AutoUpdateEnabled: true}}
	sched := NewScheduler(st, discardLogger())
	sched.Fetchers = fakeFetchers(nil, errors.New("github unreachable"))

	if err := sched.Tick(context.Background()); err == nil {
		t.Fatal("Tick() error = nil, want the fetch failure surfaced")
	}
	if got := sched.Result(); got != (Result{}) {
		t.Errorf("Result() = %+v, want zero value after a failed fetch", got)
	}
}

func TestScheduler_Tick_RecordsUpdateAvailable(t *testing.T) {
	orig := version.Version
	version.Version = "v1.0.0"
	t.Cleanup(func() { version.Version = orig })

	st := &fakeStore{settings: store.UpdateSettings{Channel: "beta", AutoUpdateEnabled: true}}
	sched := NewScheduler(st, discardLogger())
	sched.Fetchers = fakeFetchers(&upgrade.Release{Tag: "v1.1.0-beta.1", URL: "https://example.com/beta"}, nil)

	if err := sched.Tick(context.Background()); err != nil {
		t.Fatalf("Tick() error = %v, want nil", err)
	}

	got := sched.Result()
	want := Result{Channel: "beta", CurrentVersion: "v1.0.0", LatestVersion: "v1.1.0-beta.1", UpdateAvailable: true, ReleaseURL: "https://example.com/beta"}
	if got.Channel != want.Channel || got.CurrentVersion != want.CurrentVersion || got.LatestVersion != want.LatestVersion ||
		got.UpdateAvailable != want.UpdateAvailable || got.ReleaseURL != want.ReleaseURL {
		t.Errorf("Result() = %+v, want %+v (ignoring CheckedAt)", got, want)
	}
	if got.CheckedAt.IsZero() {
		t.Error("CheckedAt is zero, want it set")
	}
}

func TestScheduler_Tick_InvalidChannelFallsBackToStable(t *testing.T) {
	st := &fakeStore{settings: store.UpdateSettings{Channel: "nightly", AutoUpdateEnabled: true}}
	sched := NewScheduler(st, discardLogger())
	var calledChannel string
	sched.Fetchers = upgrade.Fetchers{
		Stable: func(context.Context) (*upgrade.Release, error) {
			calledChannel = "stable"
			return &upgrade.Release{Tag: "v2.0.0"}, nil
		},
		Beta: func(context.Context) (*upgrade.Release, error) { calledChannel = "beta"; return nil, nil },
		Edge: func(context.Context) (*upgrade.Release, error) { calledChannel = "edge"; return nil, nil },
	}

	if err := sched.Tick(context.Background()); err != nil {
		t.Fatalf("Tick() error = %v, want nil", err)
	}
	if calledChannel != "stable" {
		t.Errorf("fetcher called for channel %q, want fallback to stable on an invalid stored channel", calledChannel)
	}
}

func TestScheduler_Run_StopsOnContextCancel(t *testing.T) {
	st := &fakeStore{settings: store.UpdateSettings{AutoUpdateEnabled: false}}
	sched := NewScheduler(st, discardLogger())

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- sched.Run(ctx, time.Millisecond) }()

	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Errorf("Run() error = %v, want context.Canceled", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run() did not return after context cancellation")
	}
}
