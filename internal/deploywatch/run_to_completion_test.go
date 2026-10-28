package deploywatch

import (
	"context"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/deploysafety"
	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/store"
)

func TestObserve_OneShotExitZeroIsNotExited(t *testing.T) {
	tests := []struct {
		name       string
		restart    string
		exitCode   int
		wantExited bool
	}{
		{name: "one-shot exit 0 is done", restart: "no", exitCode: 0, wantExited: false},
		{name: "one-shot exit 1 still counts", restart: "on-failure", exitCode: 1, wantExited: true},
		{name: "keep-running exit 0 still counts", restart: "", exitCode: 0, wantExited: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := &store.DesiredService{Name: "web", Image: "app:v2", Health: &store.ServiceHealth{Restart: tt.restart}}
			w := &Watcher{Inspectors: func(string) (docker.ExitStateInspector, error) {
				return fakeInspector{currentContainer(): {ExitCode: tt.exitCode}}, nil
			}}
			obs, err := w.observe(context.Background(), svc, "dep", deploysafety.Effective{}, time.Now().Add(-time.Minute), time.Now())
			if err != nil {
				t.Fatal(err)
			}
			if obs.Exited != tt.wantExited {
				t.Fatalf("Exited = %v, want %v", obs.Exited, tt.wantExited)
			}
		})
	}
}
