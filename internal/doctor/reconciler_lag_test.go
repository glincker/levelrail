package doctor

import (
	"testing"
	"time"
)

func TestReconcilerLag(t *testing.T) {
	lag := map[string]time.Duration{"app": 5 * time.Second, "database": 3 * time.Minute, "mesh": 2 * time.Minute}
	c := ReconcilerLag(lag, 2*time.Minute)
	if c.Status != StatusWarn || len(c.Lagging) != 2 || c.Lagging[0].Reconciler != "database" {
		t.Fatalf("got %+v", c)
	}
	if c := ReconcilerLag(lag, 10*time.Minute); c.Status != StatusOK || len(c.Lagging) != 0 {
		t.Fatalf("under threshold: %+v", c)
	}
	if c := ReconcilerLag(lag, 0); c.Status != StatusOK {
		t.Fatalf("disabled: %+v", c)
	}
}
