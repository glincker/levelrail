package agent

import (
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	"github.com/GLINCKER/levelrail/internal/agent/agentpb"
	"github.com/GLINCKER/levelrail/internal/docker"
)

// A remote one-shot is only Completed once it started and finished; both times must survive the wire.
func TestExitStateTimesSurviveTheWire(t *testing.T) {
	started := time.Date(2026, 10, 20, 8, 30, 15, 123456789, time.UTC)
	finished := started.Add(42 * time.Second)
	raw, err := proto.Marshal(exitStateToPB(&docker.ExitState{StartedAt: started, FinishedAt: finished}))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var pb agentpb.ExitState
	if err := proto.Unmarshal(raw, &pb); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	got := exitStateFromPB(&pb)
	if !got.StartedAt.Equal(started) || !got.FinishedAt.Equal(finished) {
		t.Fatalf("times = %v / %v, want %v / %v", got.StartedAt, got.FinishedAt, started, finished)
	}
}

// A created-never-started container has zero times; they go out unset, like an older agent's.
func TestExitStateUnknownTimesStayZero(t *testing.T) {
	if pb := exitStateToPB(&docker.ExitState{}); pb.StartedAt != nil || pb.FinishedAt != nil {
		t.Fatalf("zero times sent as %v / %v, want unset", pb.StartedAt, pb.FinishedAt)
	}
	got := exitStateFromPB(&agentpb.ExitState{})
	if !got.StartedAt.IsZero() || !got.FinishedAt.IsZero() {
		t.Fatalf("missing times decoded as %v / %v, want zero", got.StartedAt, got.FinishedAt)
	}
}

// Only an agent that advertises exit times may have zero times read as never started.
func TestGRPCTransportReportsExitTimesOnlyWhenAdvertised(t *testing.T) {
	old := &capSet{}
	old.store([]string{CapContainerSpecSecurity})
	if (&GRPCTransport{caps: old}).ReportsExitTimes() {
		t.Fatal("an agent without the capability must fall back to the old completion rule")
	}
	cur := &capSet{}
	cur.store(LocalAgentInfo().GetCapabilities())
	if !(&GRPCTransport{caps: cur}).ReportsExitTimes() {
		t.Fatal("this agent build must advertise exit times")
	}
}
