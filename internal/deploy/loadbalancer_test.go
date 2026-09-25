package deploy

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/build"
	"github.com/GLINCKER/levelrail/internal/loadbalancer"
)

type fakeLBStore struct {
	saved map[string]string
	err   error
}

func (f *fakeLBStore) SetServiceLoadBalancer(_ context.Context, service, cfg string) error {
	if f.err != nil {
		return f.err
	}
	if f.saved == nil {
		f.saved = map[string]string{}
	}
	f.saved[service] = cfg
	return nil
}

func TestPipeline_Deploy_PersistsLoadBalancerBlock(t *testing.T) {
	svc := imageService()
	svc.LoadBalancer = &loadbalancer.Config{Algorithm: loadbalancer.AlgoLeastConn}

	tests := []struct {
		name      string
		lbStore   *fakeLBStore
		wantSaved string
		wantErr   string
	}{
		{name: "block persisted", lbStore: &fakeLBStore{}, wantSaved: `{"algorithm":"least_conn"}`},
		{name: "store failure surfaces", lbStore: &fakeLBStore{err: errors.New("disk full")}, wantErr: "save loadbalancer"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := New(&fakeBuilder{result: &build.Result{Tag: "x:1"}}, &fakeServiceStore{}, WithLoadBalancerStore(tt.lbStore))
			_, err := p.Deploy(context.Background(), Request{ServiceName: "web", Service: svc}, nil)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("Deploy() error = %v, want %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Deploy() error = %v", err)
			}
			if got := tt.lbStore.saved["web"]; got != tt.wantSaved {
				t.Errorf("saved config = %q, want %q", got, tt.wantSaved)
			}
		})
	}
}

func TestPipeline_Deploy_NoLoadBalancerBlockLeavesStoreAlone(t *testing.T) {
	lbStore := &fakeLBStore{}
	p := New(&fakeBuilder{result: &build.Result{Tag: "x:1"}}, &fakeServiceStore{}, WithLoadBalancerStore(lbStore))
	if _, err := p.Deploy(context.Background(), Request{ServiceName: "web", Service: imageService()}, nil); err != nil {
		t.Fatal(err)
	}
	if len(lbStore.saved) != 0 {
		t.Errorf("saved = %v, want nothing", lbStore.saved)
	}
}
