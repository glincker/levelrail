package alerting

import (
	"context"
	"testing"

	"github.com/glincker/levelrail/internal/store"
)

type sleepyPlacement struct {
	fakePlacement
	sleeping bool
}

func (s *sleepyPlacement) GetAppSleep(_ context.Context, name string) (store.AppSleep, error) {
	return store.AppSleep{ServiceName: name, Sleeping: s.sleeping}, nil
}

func TestRoute_SleepingAppIsQuietByDefault(t *testing.T) {
	pl := &sleepyPlacement{sleeping: true}
	nc, db, spy := newRouteFixture(t, NoiseConfig{}, pl)
	nc.Route(context.Background(), Event{Rule: webRule("r1")}, t0, spy.send)
	if len(spy.events) != 0 {
		t.Fatal("a sleeping app must not notify by default")
	}
	h := history(t, db)
	if len(h) != 1 || h[0].Outcome != OutcomeInhibited {
		t.Fatalf("history = %+v", h)
	}
}

func TestRoute_SleepingAppNotifiesWhenSettingOn(t *testing.T) {
	pl := &sleepyPlacement{sleeping: true}
	cfg := NoiseConfig{NotifyOnSleepFn: func(context.Context) bool { return true }}
	nc, _, spy := newRouteFixture(t, cfg, pl)
	nc.Route(context.Background(), Event{Rule: webRule("r1")}, t0, spy.send)
	if len(spy.events) != 1 {
		t.Fatalf("sent %d, want 1", len(spy.events))
	}
}

func TestRoute_AwakeAppNotifies(t *testing.T) {
	pl := &sleepyPlacement{}
	nc, _, spy := newRouteFixture(t, NoiseConfig{}, pl)
	nc.Route(context.Background(), Event{Rule: webRule("r1")}, t0, spy.send)
	if len(spy.events) != 1 {
		t.Fatalf("sent %d, want 1", len(spy.events))
	}
}
