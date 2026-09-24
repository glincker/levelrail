package alerting

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

type errNodeSource struct{}

func (errNodeSource) ListNodes(context.Context) ([]store.Node, error) { return nil, errors.New("boom") }

func TestEvaluateNodeOffline(t *testing.T) {
	now := time.Now()
	rule := Rule{ID: "r1", Kind: KindNodeOffline, Enabled: true}
	tests := []struct {
		name        string
		nodes       []store.Node
		prevFiring  bool
		wantFiring  bool
		wantNotices int
	}{
		{"no nodes", nil, false, false, 0},
		{"all online", []store.Node{{ID: "a", Status: store.NodeStatusOnline}}, false, false, 0},
		{"one offline", []store.Node{{ID: "a", Name: "web-1", Status: store.NodeStatusOffline}, {ID: "b", Status: store.NodeStatusOnline}}, false, true, 1},
		{"still offline stays firing", []store.Node{{ID: "a", Status: store.NodeStatusOffline}}, true, true, 1},
		{"recovered resolves", []store.Node{{ID: "a", Status: store.NodeStatusOnline}}, true, false, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := rule
			r.Firing = tt.prevFiring
			got, notices, err := EvaluateNodeOffline(context.Background(), &fakeNodeSource{nodes: tt.nodes}, r, now)
			if err != nil {
				t.Fatal(err)
			}
			if got.Firing != tt.wantFiring || len(notices) != tt.wantNotices {
				t.Fatalf("firing=%v notices=%v, want firing=%v n=%d", got.Firing, notices, tt.wantFiring, tt.wantNotices)
			}
		})
	}
}

func TestEvaluateNodeOffline_ListError(t *testing.T) {
	if _, _, err := EvaluateNodeOffline(context.Background(), errNodeSource{}, Rule{ID: "r"}, time.Now()); err == nil {
		t.Fatal("want error")
	}
}
