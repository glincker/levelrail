package reconcile

import (
	"context"
	"testing"
)

func TestNudgeController_ClosesBreakerAndQueuesPass(t *testing.T) {
	e := NewEngine(testLogger())
	for i := 0; i < 50; i++ {
		e.guard.Before("application/web")
		e.guard.After("application/web", context.DeadlineExceeded, false)
	}
	e.NudgeController("application/web")
	if !e.guard.Before("application/web") {
		t.Fatal("breaker still open after NudgeController")
	}
	if len(e.nudge) != 1 {
		t.Fatal("no pass queued")
	}
}
