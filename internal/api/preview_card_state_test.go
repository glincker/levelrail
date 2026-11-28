package api

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/glincker/levelrail/internal/preview"
	"github.com/glincker/levelrail/internal/store"
)

// stateStub answers CardStates and records which apps were asked about.
type stateStub struct {
	*fakePreview
	states map[string]preview.CardState
	asked  []string
}

func (s *stateStub) CardStates(_ context.Context, apps []string) (map[string]preview.CardState, error) {
	s.asked = append(s.asked, apps...)
	out := map[string]preview.CardState{}
	for _, a := range apps {
		out[a] = s.states[a]
	}
	return out, nil
}

func TestListPreviewCards_ReportsRenderStateForWebAndStaticOnly(t *testing.T) {
	stub := &stateStub{fakePreview: newFakePreview(t), states: map[string]preview.CardState{
		"site":   {State: preview.StateRendering},
		"docs":   {State: preview.StateFailed, Reason: preview.ReasonTimeout},
		"shop":   {State: preview.StateNone},
		"worker": {State: preview.StateQueued},
	}}
	rt := &Router{preview: stub, logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
	svcs := []store.DesiredService{
		{Name: "site", Image: "web:1", Port: 3000, Domains: []string{"site.example.com"}},
		{Name: "docs", Image: "web:1", Port: 80, Domains: []string{"docs.example.com"}},
		{Name: "shop", Image: "web:1", Port: 80},
		{Name: "worker", Image: "worker:1"},
		{Name: "db", Image: "postgres:16", Port: 5432},
	}
	got := rt.listPreviewCards(context.Background(), svcs)
	for _, app := range stub.asked {
		if app == "worker" || app == "db" {
			t.Errorf("%s asked for a card state, only web and static kinds render", app)
		}
	}
	if got["site"].State != preview.StateRendering {
		t.Errorf("site = %+v, want rendering", got["site"])
	}
	if got["docs"].State != preview.StateFailed || got["docs"].Reason != preview.ReasonTimeout {
		t.Errorf("docs = %+v, want failed with reason", got["docs"])
	}
	if _, ok := got["shop"]; ok {
		t.Error("an app with state none and nothing to draw must have no preview entry")
	}
	if _, ok := got["worker"]; ok {
		t.Error("worker must have no preview entry")
	}
}
