package reconcile

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/chaos"
	"github.com/GLINCKER/levelrail/internal/chaos/live"
	"github.com/GLINCKER/levelrail/test/e2e/testenv"
)

// TestChaos_Live runs each scenario against real local containers and
// asserts the platform's documented behaviour; the measured numbers are logged.
func TestChaos_Live(t *testing.T) {
	if testing.Short() {
		t.Skip("live Docker test")
	}
	testenv.RequireFullLive(t)
	for _, info := range chaos.Catalog() {
		t.Run(info.Name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 6*time.Minute)
			defer cancel()
			res := live.Run(ctx, info.Name, "")
			b, _ := json.Marshal(res)
			t.Logf("%s", b)
			if res.Status == chaos.StatusSkipped {
				t.Skip(res.Summary)
			}
			for _, a := range res.Assertions {
				if !a.Passed {
					t.Errorf("%s: %s (%s)", info.Name, a.Name, a.Detail)
				}
			}
		})
	}
}
