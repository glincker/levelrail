package testenv

import (
	"testing"

	"github.com/GLINCKER/levelrail/internal/docker"
)

// The reconciler reads optional capabilities by type assertion, so the
// wrapper must not hide what the real client supports.
func TestWithPullRetry_KeepsOptionalInterfaces(t *testing.T) {
	c, err := docker.NewClient()
	if err != nil {
		t.Skipf("docker.NewClient: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })
	rt := withPullRetry(t, c)
	if _, ok := rt.(docker.ExitStateInspector); !ok {
		t.Error("wrapped runtime hides docker.ExitStateInspector")
	}
	if _, ok := rt.(docker.NetworkAdmin); !ok {
		t.Error("wrapped runtime hides docker.NetworkAdmin")
	}
	r, ok := rt.(interface{ ReportsExitTimes() bool })
	if !ok || !r.ReportsExitTimes() {
		t.Error("wrapped local client does not report exit times")
	}
}
