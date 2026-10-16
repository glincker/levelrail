package appsleep

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

// EventStore persists sleep history and mirrors slept and woke into the app timeline.
type EventStore interface {
	AddSleepEvent(ctx context.Context, e store.SleepEvent) (int64, error)
	AddAppEvent(ctx context.Context, e store.AppEvent) error
}

// Probe and bot labels for request wakes that look automated.
const (
	SourceHealthProbe = "health probe"
	SourceBot         = "bot"
)

// Record stores e, and for slept and woke also an app timeline entry. A
// failure is logged and never returned: the state change already happened.
// It returns the history row's ID, 0 when nothing was stored.
func Record(ctx context.Context, es EventStore, logger *slog.Logger, e store.SleepEvent) int64 {
	if es == nil {
		return 0
	}
	if logger == nil {
		logger = slog.Default()
	}
	id, err := es.AddSleepEvent(ctx, e)
	if err != nil {
		logger.Warn("appsleep: record sleep event failed", slog.String("app", e.ServiceName), slog.String("kind", e.Kind), slog.String("error", err.Error()))
		return 0
	}
	kind, title := timelineEntry(e)
	if kind == "" {
		return id
	}
	err = es.AddAppEvent(ctx, store.AppEvent{AppName: e.ServiceName, Kind: kind, Actor: e.Actor, Title: title, Detail: e.Detail, CreatedAt: e.At})
	if err != nil {
		logger.Warn("appsleep: record timeline entry failed", slog.String("app", e.ServiceName), slog.String("error", err.Error()))
	}
	return id
}

func timelineEntry(e store.SleepEvent) (kind, title string) {
	switch e.Kind {
	case store.SleepEventSlept:
		if e.Reason == store.SleepReasonIdleTimeout {
			return store.AppEventSlept, fmt.Sprintf("Went to sleep after %d min idle", e.IdleMinutes)
		}
		return store.AppEventSlept, "Put to sleep"
	case store.SleepEventWoke:
		return store.AppEventWoke, "Woke up (" + e.Reason + ")"
	}
	return "", ""
}

// WokeEvent builds the history row for a wake. slept is when the app went to sleep.
func WokeEvent(name, reason, actor string, slept, now time.Time) store.SleepEvent {
	e := store.SleepEvent{ServiceName: name, Kind: store.SleepEventWoke, At: now, Reason: reason, Actor: actor}
	if !slept.IsZero() && now.After(slept) {
		d := int64(now.Sub(slept) / time.Second)
		e.DurationSeconds = &d
	}
	return e
}
