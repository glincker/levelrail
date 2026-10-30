package main

import (
	"context"
	"errors"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestAssistantModeSourceFailsClosed(t *testing.T) {
	src := func(s store.AIControlSettings, err error) func(context.Context) string {
		return assistantModeSource(func(context.Context) (store.AIControlSettings, error) { return s, err })
	}
	ctx := context.Background()
	if got := src(store.AIControlSettings{Mode: store.AIControlModeObserve}, nil)(ctx); got != "observe" {
		t.Fatalf("observe = %q", got)
	}
	if got := src(store.AIControlSettings{Mode: store.AIControlModeOperate}, nil)(ctx); got != "operate" {
		t.Fatalf("operate = %q", got)
	}
	if got := src(store.AIControlSettings{}, errors.New("db down"))(ctx); got != store.AIControlModeOff {
		t.Fatalf("read error = %q, want off", got)
	}
	if got := src(store.AIControlSettings{Mode: "bogus"}, nil)(ctx); got != store.AIControlModeOff {
		t.Fatalf("unknown mode = %q, want off", got)
	}
}
