package agent

import (
	"context"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/agent/agentpb"
)

// EnvSilencePushEvery is how often a session re-checks the silence URL, so a dashboard edit reaches live agents.
const EnvSilencePushEvery = "APP_AGENT_CP_SILENCE_PUSH_EVERY"

const defaultSilencePushEvery = 30 * time.Second

func silencePushEveryFromEnv() time.Duration {
	d, err := time.ParseDuration(os.Getenv(EnvSilencePushEvery))
	if err != nil || d <= 0 {
		return defaultSilencePushEvery
	}
	return d
}

// SilenceSource returns the off-host URL agents ping when they lose the control plane, or "" for none.
type SilenceSource func(ctx context.Context) string

// SetSilenceSource wires the URL provider; safe to call after the server starts.
func (s *Server) SetSilenceSource(src SilenceSource) {
	s.silenceSrc.Store(&src)
}

func (s *Server) silenceURL(ctx context.Context) string {
	if p := s.silenceSrc.Load(); p != nil && *p != nil {
		return (*p)(ctx)
	}
	return ""
}

// silenceLoop sends the URL on session start and again whenever it changes, until done closes.
func (s *Server) silenceLoop(ctx context.Context, nodeID string, m *mux, every time.Duration, done <-chan struct{}) {
	if every <= 0 {
		every = defaultSilencePushEvery
	}
	last := "\x00"
	push := func() {
		cur := strings.TrimSpace(s.silenceURL(ctx))
		if cur == last {
			return
		}
		frame := &agentpb.ControlMessage{Payload: &agentpb.ControlMessage_SilenceConfig{SilenceConfig: &agentpb.SilenceConfig{Url: cur}}}
		if err := m.sendFrame(frame); err != nil {
			s.logger.Warn("agent: session: silence config push failed", slog.String("node_id", nodeID), slog.String("error", err.Error()))
			return
		}
		last = cur
	}
	push()
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-done:
			return
		case <-ctx.Done():
			return
		case <-t.C:
			push()
		}
	}
}
