package nodemove

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

type fakePlanner struct {
	exec      *fakeExec
	resources []Resource
	cordoned  map[string]bool
	blockApp  string
	models    []store.NodeDrainBlocked
}

func (p *fakePlanner) Cordon(_ context.Context, node string) error {
	p.cordoned[node] = true
	return nil
}

func (p *fakePlanner) Remaining(_ context.Context, node string) ([]Resource, error) {
	p.exec.mu.Lock()
	defer p.exec.mu.Unlock()
	var out []Resource
	for _, r := range p.resources {
		if p.exec.placement[key(r.Kind, r.Name)] == node {
			out = append(out, r)
		}
	}
	return out, nil
}

func (p *fakePlanner) PickTarget(_ context.Context, res Resource, _, explicit string) (string, error) {
	if res.Name == p.blockApp {
		return "", &BlockedError{Reason: "needs a GPU node"}
	}
	if explicit != "" {
		return explicit, nil
	}
	return "b", nil
}

func (p *fakePlanner) Blocked(context.Context, string) ([]store.NodeDrainBlocked, error) {
	return p.models, nil
}

func TestDrain(t *testing.T) {
	tests := []struct {
		name       string
		setup      func(*fakePlanner)
		wantStatus string
		wantErr    string
		wantLeft   int
	}{
		{name: "moves database before its app", wantStatus: store.NodeMoveStatusSucceeded},
		{
			name:       "blocked resource fails the drain but moves the rest",
			setup:      func(p *fakePlanner) { p.blockApp = "web" },
			wantStatus: store.NodeMoveStatusFailed,
			wantErr:    "could not move: app web",
			wantLeft:   1,
		},
		{
			name: "model on the node is reported",
			setup: func(p *fakePlanner) {
				p.models = []store.NodeDrainBlocked{{Kind: "model", Name: "llm", Reason: "models cannot be moved"}}
			},
			wantStatus: store.NodeMoveStatusFailed,
			wantErr:    "model llm",
		},
		{
			name:       "a failed move stops the drain with the node cordoned",
			setup:      func(p *fakePlanner) { p.exec.readyAt["database/main"] = -1 },
			wantStatus: store.NodeMoveStatusFailed,
			wantErr:    "moving database main failed",
			wantLeft:   2,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := openStore(t)
			f := newFakeExec()
			f.placement["database/main"] = "a"
			f.placement["app/web"] = "a"
			p := &fakePlanner{exec: f, cordoned: map[string]bool{}, resources: []Resource{
				{Kind: "database", Name: "main", Volumes: []string{"db-main-data"}},
				{Kind: "app", Name: "web"},
			}}
			if tt.setup != nil {
				tt.setup(p)
			}
			c := &clock{t: time.Date(2026, 10, 10, 0, 0, 0, 0, time.UTC)}
			r := newRunner(s, f, c, WithDrainPlanner(p))
			d, err := r.StartDrain(context.Background(), "a", "")
			if err != nil {
				t.Fatalf("StartDrain: %v", err)
			}
			if !p.cordoned["a"] {
				t.Fatal("drain did not cordon the node first")
			}
			if _, err := r.StartDrain(context.Background(), "a", ""); !errors.Is(err, store.ErrNodeMoveInProgress) {
				t.Fatalf("second StartDrain error = %v", err)
			}
			for i := 0; i < 60; i++ {
				r.Tick(context.Background())
				c.t = c.t.Add(10 * time.Second)
				got, _ := s.GetNodeDrain(context.Background(), d.ID)
				if got.Status != store.NodeMoveStatusRunning {
					break
				}
			}
			got, _ := s.GetNodeDrain(context.Background(), d.ID)
			if got.Status != tt.wantStatus || !strings.Contains(got.Error, tt.wantErr) {
				t.Fatalf("drain = %s %q, want %s containing %q", got.Status, got.Error, tt.wantStatus, tt.wantErr)
			}
			left, _ := p.Remaining(context.Background(), "a")
			if len(left) != tt.wantLeft {
				t.Fatalf("left on node = %+v, want %d", left, tt.wantLeft)
			}
			if tt.wantStatus == store.NodeMoveStatusSucceeded {
				ops := strings.Join(f.ops(), "|")
				if strings.Index(ops, "place main@b") > strings.Index(ops, "place web@b") {
					t.Fatalf("app moved before its database: %s", ops)
				}
			}
		})
	}
}
