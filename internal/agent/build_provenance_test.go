package agent

import (
	"testing"

	"github.com/GLINCKER/levelrail/internal/agent/agentpb"
)

func doneFrame(id string) *agentpb.BuildOutput {
	return &agentpb.BuildOutput{BuildId: id, Frame: &agentpb.BuildOutput_Done{Done: &agentpb.BuildDone{Digest: "sha256:" + id}}}
}

func TestMux_BuildResultFromOtherNodeIsRejected(t *testing.T) {
	assigned := newMux(newFakeSessionStream())
	other := newMux(newFakeSessionStream())
	sub := assigned.subscribeBuild("b1")

	other.deliverBuildOutput(doneFrame("b1"))
	select {
	case <-sub.out:
		t.Fatal("a result reported by a node that was not assigned the build was delivered")
	default:
	}

	assigned.deliverBuildOutput(doneFrame("b1"))
	if len(sub.out) != 1 {
		t.Fatalf("the assigned node's result was not delivered, queued=%d", len(sub.out))
	}
	// A second terminal frame for a finished build is a replay.
	assigned.deliverBuildOutput(doneFrame("b1"))
	if len(sub.out) != 1 {
		t.Fatalf("a replayed result was delivered, queued=%d", len(sub.out))
	}
}
