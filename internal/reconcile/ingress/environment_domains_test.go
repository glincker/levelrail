package ingress

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/GLINCKER/levelrail/internal/reconcile/application"
	"github.com/GLINCKER/levelrail/internal/store"
)

type envDomainStore struct {
	*fakeStore
	sets map[string]map[string][]string
	err  error
}

func (s *envDomainStore) ListAllServiceEnvironmentDomains(context.Context) (map[string]map[string][]string, error) {
	return s.sets, s.err
}

func TestController_Reconcile_RoutesActiveEnvironmentDomains(t *testing.T) {
	tests := []struct {
		name    string
		envID   string
		sets    map[string]map[string][]string
		listErr error
		want    []string
	}{
		{"dev set routes in dev", "env_dev", map[string]map[string][]string{"web": {"env_dev": {"dev.example.com"}, "env_uat": {"uat.example.com"}}}, nil, []string{"dev.example.com"}},
		{"uat set routes in uat", "env_uat", map[string]map[string][]string{"web": {"env_dev": {"dev.example.com"}, "env_uat": {"uat.example.com"}}}, nil, []string{"uat.example.com"}},
		{"environment without a set keeps default domains", "env_production", map[string]map[string][]string{"web": {"env_dev": {"dev.example.com"}}}, nil, []string{"example.com"}},
		{"untagged app keeps default domains", "", map[string]map[string][]string{"web": {"env_dev": {"dev.example.com"}}}, nil, []string{"example.com"}},
		{"list failure degrades to default domains", "env_dev", nil, errors.New("db down"), []string{"example.com"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			desired := store.DesiredService{Name: "web", Image: "img:v1", Port: 80, Domains: []string{"example.com"}, EnvironmentID: tt.envID}
			rt := newFakeRuntime()
			rt.seedRunning(application.ContainerName(desired.Name, desired.Image, ""), 34567)
			st := &envDomainStore{fakeStore: &fakeStore{services: []store.DesiredService{desired}}, sets: tt.sets, err: tt.listErr}
			applier := &fakeApplier{}
			c := New(st, rt, applier, WithLogger(discardLogger()))

			if _, err := c.Reconcile(context.Background()); err != nil {
				t.Fatalf("Reconcile() error = %v", err)
			}
			var got []string
			for _, r := range applier.routes(t) {
				for _, m := range r.Match {
					got = append(got, m.Host...)
				}
			}
			if !slices.Equal(got, tt.want) {
				t.Fatalf("routed hosts = %v, want %v", got, tt.want)
			}
		})
	}
}
