package failure

import (
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/deploysafety"
)

func TestIsRolloutCrashLoop(t *testing.T) {
	msg := func(n int) string { return deploysafety.CrashLoopMessage("web-abc", n, 2*time.Second) }
	tests := []struct {
		name    string
		reason  string
		message string
		limit   int
		want    bool
	}{
		{"at the limit", deploysafety.ReasonCrashLoopBackOff, msg(2), 2, true},
		{"past the limit", deploysafety.ReasonCrashLoopBackOff, msg(7), 2, true},
		{"below the limit", deploysafety.ReasonCrashLoopBackOff, msg(1), 2, false},
		{"unparsable message", deploysafety.ReasonCrashLoopBackOff, "keeps exiting", 2, false},
		{"other reason", "Deployed", msg(9), 2, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsRolloutCrashLoop(tt.reason, tt.message, tt.limit); got != tt.want {
				t.Errorf("IsRolloutCrashLoop = %t, want %t", got, tt.want)
			}
		})
	}
	env := map[string]string{EnvRolloutCrashLoopRestarts: "4"}
	if got := RolloutCrashLoopRestarts(func(k string) (string, bool) { v, ok := env[k]; return v, ok }); got != 4 {
		t.Errorf("RolloutCrashLoopRestarts = %d, want 4", got)
	}
}
