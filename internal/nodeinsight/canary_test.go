package nodeinsight

import "testing"

func TestCanaryWait(t *testing.T) {
	behind := []CanaryNode{{ID: "c1", Online: true, Version: "1.0.0"}}
	if CanaryWait(false, behind, "1.1.0") == "" {
		t.Error("non-canary must wait while the canary is behind")
	}
	if CanaryWait(true, behind, "1.1.0") != "" {
		t.Error("canary itself never waits")
	}
	done := []CanaryNode{{ID: "c1", Online: true, Version: "1.1.0"}}
	if got := CanaryWait(false, done, "1.1.0"); got != "" {
		t.Errorf("wait = %q, want none once the canary is current", got)
	}
	if CanaryWait(false, []CanaryNode{{ID: "c1", Version: "1.1.0"}}, "1.1.0") == "" {
		t.Error("offline canary must hold the fleet")
	}
}
