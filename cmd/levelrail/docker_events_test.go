package main

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
)

func TestResilientEventsResubscribes(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	calls := 0
	subscribe := func(context.Context) (<-chan docker.Event, <-chan error) {
		calls++
		ev := make(chan docker.Event, 1)
		errs := make(chan error, 1)
		if calls == 1 {
			errs <- errors.New("daemon gone")
			close(ev)
			close(errs)
			return ev, errs
		}
		ev <- docker.Event{ContainerName: "app"}
		return ev, errs
	}
	out := resilientEvents(ctx, subscribe, slog.New(slog.NewTextHandler(io.Discard, nil)))
	select {
	case got := <-out:
		if got.ContainerName != "app" || calls != 2 {
			t.Fatalf("got %+v after %d subscribes", got, calls)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no event after the stream ended and was resubscribed")
	}
}
