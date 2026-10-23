package deploy

import (
	"context"
	"errors"
	"testing"

	"github.com/GLINCKER/levelrail/internal/build"
)

type fakeSpendGuard struct {
	err  error
	apps []string
}

func (f *fakeSpendGuard) BeforeDeploy(_ context.Context, app string) error {
	f.apps = append(f.apps, app)
	return f.err
}

func TestPipeline_SpendGuard(t *testing.T) {
	refused := errors.New("deploys are blocked by a spend cap")
	tests := []struct {
		name      string
		guard     *fakeSpendGuard
		wantErr   bool
		wantSaves int
	}{
		{name: "allowed deploy proceeds", guard: &fakeSpendGuard{}, wantSaves: 1},
		{name: "blocked deploy stops before the build", guard: &fakeSpendGuard{err: refused}, wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			builder := &fakeBuilder{result: &build.Result{Tag: "levelrail/web:abc1234"}}
			svcStore := &fakeServiceStore{}
			p := New(builder, svcStore, WithSpendGuard(tc.guard))
			_, err := p.Deploy(context.Background(), supplyChainRequest(), nil)
			if tc.wantErr != errors.Is(err, refused) {
				t.Fatalf("err = %v", err)
			}
			if len(tc.guard.apps) != 1 || svcStore.saveCalls != tc.wantSaves {
				t.Fatalf("guard calls=%v saves=%d, want saves=%d", tc.guard.apps, svcStore.saveCalls, tc.wantSaves)
			}
			if tc.wantErr && builder.lastReq.Tag != "" {
				t.Errorf("a refused deploy must not build, built %q", builder.lastReq.Tag)
			}
		})
	}
}
