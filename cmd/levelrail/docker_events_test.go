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

func TestResilientEventsQueuesBurstFromUnbufferedUpstream(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	up := make(chan docker.Event)
	subscribe := func(context.Context) (<-chan docker.Event, <-chan error) {
		return up, make(chan error)
	}
	out := resilientEvents(ctx, subscribe, slog.New(slog.NewTextHandler(io.Discard, nil)))
	const burst = 10
	sent := make(chan struct{})
	go func() {
		for i := 0; i < burst; i++ {
			up <- docker.Event{ContainerName: "app"}
		}
		close(sent)
	}()
	select {
	case <-sent:
	case <-time.After(5 * time.Second):
		t.Fatal("upstream blocked: burst was not queued while nothing consumed")
	}
	deadline := time.Now().Add(2 * time.Second)
	for len(out) < burst && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if len(out) != burst {
		t.Fatalf("queued %d of %d events", len(out), burst)
	}
}
