package imageupdate

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

type fakeStore struct{ apps []store.ImageAutoUpdate }

func (f fakeStore) ListEnabledImageAutoUpdates(context.Context) ([]store.ImageAutoUpdate, error) {
	return f.apps, nil
}

type fakeChecker struct {
	seen []string
	fail map[string]bool
}

func (f *fakeChecker) CheckImageUpdate(_ context.Context, name string) (string, error) {
	f.seen = append(f.seen, name)
	if f.fail[name] {
		return "", errors.New("registry down")
	}
	return "up to date", nil
}

func TestTick_OneFailureDoesNotStopTheRest(t *testing.T) {
	c := &fakeChecker{fail: map[string]bool{"a": true}}
	s := &Scheduler{
		Store:   fakeStore{apps: []store.ImageAutoUpdate{{ServiceName: "a"}, {ServiceName: "b"}}},
		Checker: c,
		Logger:  slog.Default(),
	}
	err := s.Tick(t.Context())
	if err == nil || len(c.seen) != 2 {
		t.Fatalf("err = %v, checked = %v; want an error and both apps checked", err, c.seen)
	}
}

func TestIntervalFromEnv(t *testing.T) {
	for raw, want := range map[string]time.Duration{"": DefaultInterval, "30m": 30 * time.Minute, "nope": DefaultInterval, "-5m": DefaultInterval} {
		t.Setenv(EnvInterval, raw)
		if got := IntervalFromEnv(slog.Default()); got != want {
			t.Errorf("%q: got %v, want %v", raw, got, want)
		}
	}
}
