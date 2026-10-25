package build

import (
	"context"
	"errors"
	"testing"

	"github.com/docker/docker/api/types"
)

type fakeEngine struct {
	v   types.Version
	err error
}

func (f fakeEngine) ServerVersion(context.Context) (types.Version, error) { return f.v, f.err }

func TestCheckEngine(t *testing.T) {
	podman := types.Version{Components: []types.ComponentVersion{{Name: "Podman Engine"}}}
	docker := types.Version{Components: []types.ComponentVersion{{Name: "Engine"}}}
	tests := []struct {
		name string
		eng  fakeEngine
		want error
	}{
		{"podman component", fakeEngine{v: podman}, ErrBuildEngineUnsupported},
		{"docker", fakeEngine{v: docker}, nil},
		{"lookup error passes through", fakeEngine{err: errors.New("down")}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &Client{engine: tt.eng}
			if got := c.checkEngine(context.Background()); !errors.Is(got, tt.want) {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
		})
	}
}
