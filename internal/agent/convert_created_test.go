package agent

import (
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	"github.com/GLINCKER/levelrail/internal/agent/agentpb"
	"github.com/GLINCKER/levelrail/internal/docker"
)

// Holds and retirement order on remote nodes depend on Created surviving the wire.
func TestContainerStateCreatedSurvivesTheWire(t *testing.T) {
	created := time.Date(2026, 10, 20, 8, 30, 15, 123456789, time.UTC)
	raw, err := proto.Marshal(containerStateToPB(&docker.ContainerState{ID: "c1", Name: "web-1", Created: created}))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var pb agentpb.ContainerState
	if err := proto.Unmarshal(raw, &pb); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got := containerStateFromPB(&pb).Created; !got.Equal(created) {
		t.Fatalf("Created = %v, want %v", got, created)
	}
}

// An older agent never sets the field; that must read as unknown, not as a real time.
func TestContainerStateCreatedUnknownStaysZero(t *testing.T) {
	if pb := containerStateToPB(&docker.ContainerState{ID: "c1"}); pb.Created != nil {
		t.Fatalf("zero Created sent as %v, want unset", pb.Created)
	}
	if got := containerStateFromPB(&agentpb.ContainerState{Id: "c1"}).Created; !got.IsZero() {
		t.Fatalf("missing Created decoded as %v, want zero", got)
	}
}
