package dockerguard

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
)

type blockingDocker struct{}

func (blockingDocker) Runtime() docker.RuntimeInfo { return docker.RuntimeInfo{} }

func (blockingDocker) DaemonSecurity(ctx context.Context) (docker.DaemonSecurity, error) {
	<-ctx.Done()
	return docker.DaemonSecurity{}, ctx.Err()
}

func TestLogPrivilegeReturnsWhenDockerHangs(t *testing.T) {
	t.Setenv(bootTimeoutEnv, "100ms")
	done := make(chan bool, 1)
	go func() {
		done <- LogPrivilege(context.Background(), slog.New(slog.NewTextHandler(io.Discard, nil)), blockingDocker{})
	}()
	select {
	case ok := <-done:
		if ok {
			t.Fatal("want degraded result when docker never answers")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("LogPrivilege hung on an unresponsive daemon")
	}
}
