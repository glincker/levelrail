package statuspage

import (
	"context"
	"testing"
)

type fixedMonitorState string

func (f fixedMonitorState) MonitorState(context.Context, string) string { return string(f) }

func TestMonitorStateMapsDegraded(t *testing.T) {
	s := &Service{monitors: fixedMonitorState("degraded")}
	if got := s.monitorState(context.Background(), "mon_1"); got != Degraded {
		t.Fatalf("degraded monitor mapped to %q", got)
	}
}
