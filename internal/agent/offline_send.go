package agent

import (
	"context"
	"log/slog"
	"strconv"

	"google.golang.org/protobuf/proto"

	"github.com/GLINCKER/levelrail/internal/agent/agentpb"
	"github.com/GLINCKER/levelrail/internal/nodeinsight"
)

type offlineBufferKey struct{}

// WithOfflineBuffer makes sessions served under ctx keep metrics and events
// that could not be sent, and replay them after the next connect.
func WithOfflineBuffer(ctx context.Context, b *nodeinsight.OfflineBuffer) context.Context {
	return context.WithValue(ctx, offlineBufferKey{}, b)
}

func offlineBufferFrom(ctx context.Context) *nodeinsight.OfflineBuffer {
	b, _ := ctx.Value(offlineBufferKey{}).(*nodeinsight.OfflineBuffer)
	return b
}

// OfflineBufferFromEnv builds the buffer from the seed limits. A value of 0
// turns buffering off. The control plane has no channel to push new limits
// to a running agent, so a change needs an agent restart.
func OfflineBufferFromEnv(lookup func(string) (string, bool)) *nodeinsight.OfflineBuffer {
	num := func(key string, def int) int {
		if v, ok := lookup(key); ok {
			if n, err := strconv.Atoi(v); err == nil && n >= 0 {
				return n
			}
		}
		return def
	}
	return nodeinsight.NewOfflineBuffer(num("APP_NODE_AGENT_BUFFER_MAX_ITEMS", 2000), num("APP_NODE_AGENT_BUFFER_MAX_BYTES", 4<<20))
}

// bufferKind names the report kinds worth keeping across an outage. Command
// responses, exec output and heartbeats are only meaningful live.
func bufferKind(msg *agentpb.AgentMessage) string {
	switch msg.GetPayload().(type) {
	case *agentpb.AgentMessage_Event:
		return "event"
	case *agentpb.AgentMessage_HostReport:
		return "host"
	case *agentpb.AgentMessage_GpuReport:
		return "gpu"
	}
	return ""
}

// bufferedSend wraps a raw stream send: a failed send of a buffered kind is
// queued, and replay re-sends the queue oldest first.
func bufferedSend(buf *nodeinsight.OfflineBuffer, raw func(*agentpb.AgentMessage) error, logger *slog.Logger) (send func(*agentpb.AgentMessage), replay func() int) {
	send = func(msg *agentpb.AgentMessage) {
		err := raw(msg)
		if err == nil {
			return
		}
		logger.Warn("agent: session: send failed", slog.String("error", err.Error()))
		if kind := bufferKind(msg); buf != nil && kind != "" {
			if b, merr := proto.Marshal(msg); merr == nil {
				buf.Add(nodeinsight.Sample{Kind: kind, Payload: b})
			}
		}
	}
	replay = func() int {
		if buf == nil {
			return 0
		}
		return buf.Replay(func(s nodeinsight.Sample) error {
			var msg agentpb.AgentMessage
			if proto.Unmarshal(s.Payload, &msg) != nil {
				return nil // unreadable sample: drop it rather than block the queue
			}
			return raw(&msg)
		})
	}
	return send, replay
}

// StableSessionSeconds is how long a session must last to count as healthy.
func StableSessionSeconds(lookup func(string) (string, bool)) int {
	if v, ok := lookup("APP_AGENT_STABLE_SESSION_SECONDS"); ok {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return 120
}
