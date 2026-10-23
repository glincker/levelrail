package main

import (
	"context"
	"errors"
	"testing"

	"github.com/GLINCKER/levelrail/internal/docker"
)

type listCountingRuntime struct {
	docker.Runtime
	states []docker.ContainerState
	calls  int
	err    error
}

func (r *listCountingRuntime) ListByPrefix(_ context.Context, prefix string) ([]docker.ContainerState, error) {
	r.calls++
	if r.err != nil {
		return nil, r.err
	}
	var out []docker.ContainerState
	for _, cs := range r.states {
		if len(cs.Name) >= len(prefix) && cs.Name[:len(prefix)] == prefix {
			out = append(out, cs)
		}
	}
	return out, nil
}

func TestTickListingServesManyPrefixesFromOneCall(t *testing.T) {
	rt := &listCountingRuntime{states: []docker.ContainerState{
		{ID: "1", Name: "web-abc"},
		{ID: "2", Name: "web-abc-r1"},
		{ID: "3", Name: "api-def"},
		{ID: "4", Name: "db-main"},
	}}
	l := &tickListing{rt: rt}
	ctx := context.Background()

	web, err := l.withPrefix(ctx, "web-")
	if err != nil || len(web) != 2 {
		t.Fatalf("web- = %v, %v, want 2 containers", web, err)
	}
	api, _ := l.withPrefix(ctx, "api-")
	if len(api) != 1 || api[0].ID != "3" {
		t.Errorf("api- = %v, want container 3", api)
	}
	if none, _ := l.withPrefix(ctx, "cache-"); len(none) != 0 {
		t.Errorf("cache- = %v, want none", none)
	}
	if rt.calls != 1 {
		t.Errorf("daemon listings = %d, want 1", rt.calls)
	}
}

func TestTickListingMatchesPerPrefixQueries(t *testing.T) {
	rt := &listCountingRuntime{states: []docker.ContainerState{
		{ID: "1", Name: "web-abc"}, {ID: "2", Name: "web-worker-abc"}, {ID: "3", Name: "webby-1"},
	}}
	l := &tickListing{rt: rt}
	got, _ := l.withPrefix(context.Background(), "web-")
	want, _ := rt.ListByPrefix(context.Background(), "web-")
	if len(got) != len(want) {
		t.Fatalf("shared listing returned %d for web-, a direct query returned %d", len(got), len(want))
	}
	for i := range got {
		if got[i].ID != want[i].ID {
			t.Errorf("entry %d: %s vs %s", i, got[i].ID, want[i].ID)
		}
	}
}

func TestTickListingErrorIsNotCached(t *testing.T) {
	rt := &listCountingRuntime{err: errors.New("daemon down")}
	l := &tickListing{rt: rt}
	if _, err := l.withPrefix(context.Background(), "web-"); err == nil {
		t.Fatal("want the listing error")
	}
	rt.err = nil
	rt.states = []docker.ContainerState{{ID: "1", Name: "web-abc"}}
	if got, err := l.withPrefix(context.Background(), "web-"); err != nil || len(got) != 1 {
		t.Errorf("after recovery = %v, %v, want 1 container", got, err)
	}
}
