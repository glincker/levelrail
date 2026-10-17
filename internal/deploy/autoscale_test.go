package deploy

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/autoscale"
	"github.com/GLINCKER/levelrail/internal/spec"
	"github.com/GLINCKER/levelrail/internal/store"
)

type fakeAutoscaleStore struct {
	policy   *autoscale.Policy
	replicas int
	saved    *autoscale.Policy
}

func (f *fakeAutoscaleStore) GetAutoscalePolicy(context.Context, string) (autoscale.Policy, error) {
	if f.policy == nil {
		return autoscale.Policy{}, autoscale.ErrNotFound
	}
	return *f.policy, nil
}

func (f *fakeAutoscaleStore) SaveAutoscalePolicy(_ context.Context, p autoscale.Policy, _ autoscale.Event, _ time.Time) error {
	f.saved = &p
	return nil
}

func (f *fakeAutoscaleStore) AutoscaleReplicas(context.Context, string) (int, bool, error) {
	if f.replicas == 0 {
		return 0, false, autoscale.ErrNotFound
	}
	return f.replicas, false, nil
}

func TestApplyAutoscaleReplicas(t *testing.T) {
	stored := autoscale.Policy{App: "web", MinReplicas: 2, MaxReplicas: 6, Metric: autoscale.MetricCPUPercent, Target: 60}.Normalize()
	block := &spec.Autoscale{Min: 3, Max: 5, Metric: "cpu_percent", Target: 50}
	tests := []struct {
		name     string
		policy   *autoscale.Policy
		replicas int
		svc      spec.Service
		want     int
		wantErr  string
	}{
		{"no policy leaves the spec count", nil, 4, spec.Service{Replicas: 2}, 2, ""},
		{"stored policy keeps the autoscaler's count", &stored, 4, spec.Service{}, 4, ""},
		{"stored policy clamps a count above its max", &stored, 9, spec.Service{}, 6, ""},
		{"stored policy with no row yet starts at the minimum", &stored, 0, spec.Service{}, 2, ""},
		{"stored policy refuses a spec replicas value", &stored, 4, spec.Service{Replicas: 2}, 0, "managed by autoscale"},
		{"spec block sets the bounds", nil, 1, spec.Service{Autoscale: block}, 3, ""},
		{"spec block keeps a count inside its bounds", nil, 4, spec.Service{Autoscale: block}, 4, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := &Pipeline{autoscale: &fakeAutoscaleStore{policy: tc.policy, replicas: tc.replicas}, autoscaleCeiling: 20}
			desired := store.DesiredService{Name: "web", Replicas: tc.svc.Replicas}
			err := p.applyAutoscaleReplicas(context.Background(), Request{ServiceName: "web", Service: tc.svc}, &desired)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("err = %v, want %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if desired.Replicas != tc.want {
				t.Errorf("replicas = %d, want %d", desired.Replicas, tc.want)
			}
		})
	}
}

func TestSaveAutoscalePersistsTheSpecBlock(t *testing.T) {
	fake := &fakeAutoscaleStore{}
	p := &Pipeline{autoscale: fake, autoscaleCeiling: 20}
	req := Request{ServiceName: "web", Service: spec.Service{Autoscale: &spec.Autoscale{Max: 4, Metric: "memory_percent", Target: 70}}}
	if err := p.saveAutoscale(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	if fake.saved == nil || fake.saved.App != "web" || fake.saved.MaxReplicas != 4 {
		t.Fatalf("saved = %+v", fake.saved)
	}
	fake.saved = nil
	if err := p.saveAutoscale(context.Background(), Request{ServiceName: "web"}); err != nil || fake.saved != nil {
		t.Fatalf("a spec with no block must leave the policy alone: %v %+v", err, fake.saved)
	}
	req.Service.Autoscale.Max = 99
	if err := p.saveAutoscale(context.Background(), req); err == nil {
		t.Fatal("a block above the global ceiling must fail the deploy")
	}
}
