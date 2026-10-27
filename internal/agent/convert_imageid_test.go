package agent

import (
	"testing"

	"google.golang.org/protobuf/proto"

	"github.com/GLINCKER/levelrail/internal/agent/agentpb"
	"github.com/GLINCKER/levelrail/internal/docker"
)

// Image truth and drain holds on remote nodes compare the running image ID, so it must survive the wire.
func TestContainerStateImageIDSurvivesTheWire(t *testing.T) {
	const id = "sha256:3f1c0b0e9a7d4c2e8b6a5f4d3c2b1a0f9e8d7c6b5a4f3e2d1c0b9a8f7e6d5c4b"
	raw, err := proto.Marshal(containerStateToPB(&docker.ContainerState{ID: "c1", Name: "web-1", Image: "web:1", ImageID: id}))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var pb agentpb.ContainerState
	if err := proto.Unmarshal(raw, &pb); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got := containerStateFromPB(&pb).ImageID; got != id {
		t.Fatalf("ImageID = %q, want %q", got, id)
	}
}

// An older agent never sends the field; the control plane must read that as unknown (empty).
func TestContainerStateImageIDFromOldAgentStaysEmpty(t *testing.T) {
	raw, err := proto.Marshal(&agentpb.ContainerState{Id: "c1", Name: "web-1", Image: "web:1", Running: true})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var pb agentpb.ContainerState
	if err := proto.Unmarshal(raw, &pb); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got := containerStateFromPB(&pb).ImageID; got != "" {
		t.Fatalf("missing ImageID decoded as %q, want empty", got)
	}
}
