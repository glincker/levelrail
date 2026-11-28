package main

import (
	"testing"
	"time"

	"github.com/glincker/levelrail/internal/nodepreflight"
)

func TestLocalDoctorChecksDropsRemoteOnes(t *testing.T) {
	checks := localDoctorChecks(nodepreflight.Evaluate(nodepreflight.Facts{}, timeNowForTest(), nodepreflight.Config{}))
	for _, c := range checks {
		if c.ID == nodepreflight.CheckControlPlane || c.ID == nodepreflight.CheckClock {
			t.Fatalf("check %s needs the control plane and must not run locally", c.ID)
		}
	}
	if len(checks) == 0 {
		t.Fatal("no local checks")
	}
}

func timeNowForTest() time.Time { return time.Now() }
