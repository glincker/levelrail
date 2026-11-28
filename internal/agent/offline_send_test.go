package agent

import (
	"errors"
	"testing"

	"github.com/glincker/levelrail/internal/agent/agentpb"
	"github.com/glincker/levelrail/internal/nodeinsight"
)

func TestBufferedSendKeepsReportsAndReplaysInOrder(t *testing.T) {
	buf := nodeinsight.NewOfflineBuffer(10, 1<<20)
	down := true
	var delivered []string
	raw := func(m *agentpb.AgentMessage) error {
		if down {
			return errors.New("stream closed")
		}
		delivered = append(delivered, m.GetEvent().GetWatchId())
		return nil
	}
	send, replay := bufferedSend(buf, raw, testLogger())
	for _, id := range []string{"a", "b", "c"} {
		send(&agentpb.AgentMessage{Payload: &agentpb.AgentMessage_Event{Event: &agentpb.ProxiedEvent{WatchId: id}}})
	}
	// A heartbeat is only meaningful live and must not be queued.
	send(&agentpb.AgentMessage{Payload: &agentpb.AgentMessage_Heartbeat{Heartbeat: &agentpb.Heartbeat{}}})
	if n, _, _ := buf.Stats(); n != 3 {
		t.Fatalf("queued %d, want 3", n)
	}
	if replay() != 0 {
		t.Fatal("replay delivered while down")
	}
	down = false
	if got := replay(); got != 3 || len(delivered) != 3 || delivered[0] != "a" || delivered[2] != "c" {
		t.Fatalf("replayed %d %v", got, delivered)
	}
	if n, _, _ := buf.Stats(); n != 0 {
		t.Fatalf("left %d", n)
	}
}
