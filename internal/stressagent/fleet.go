// Package stressagent runs a fleet of fake nodes against a real control plane.
// Each node enrolls and holds a session through the production agent client, backed by an in-memory container runtime.
package stressagent

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/GLINCKER/levelrail/internal/agent"
	"github.com/GLINCKER/levelrail/internal/docker"
)

// TokenSource returns a fresh join token for the named node.
type TokenSource func(ctx context.Context, nodeName string) (string, error)

// Config describes the fleet.
type Config struct {
	// Addr is the control plane agent listener (host:port).
	Addr       string
	Count      int
	NamePrefix string
	Token      TokenSource
	// HeartbeatInterval of zero keeps the agent default.
	HeartbeatInterval time.Duration
	Logger            *slog.Logger
}

// Fleet is a set of enrolled, connected fake nodes.
type Fleet struct {
	nodes  []*node
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

type node struct {
	id       string
	rt       *Runtime
	sessions atomic.Int64
}

// Start enrolls Count nodes and keeps a session open for each until Stop.
func Start(ctx context.Context, cfg Config) (*Fleet, error) {
	if cfg.Count < 1 || cfg.Addr == "" || cfg.Token == nil {
		return nil, errors.New("stressagent: Addr, Token and a positive Count are required")
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	runCtx, cancel := context.WithCancel(context.Background())
	f := &Fleet{cancel: cancel}
	for i := 0; i < cfg.Count; i++ {
		name := fmt.Sprintf("%s-%03d", cfg.NamePrefix, i)
		tok, err := cfg.Token(ctx, name)
		if err != nil {
			f.Stop()
			return nil, fmt.Errorf("stressagent: join token for %s: %w", name, err)
		}
		id, err := agent.DialEnroll(ctx, cfg.Addr, tok, name)
		if err != nil {
			f.Stop()
			return nil, fmt.Errorf("stressagent: enroll %s: %w", name, err)
		}
		n := &node{id: id.NodeID, rt: NewRuntime()}
		f.nodes = append(f.nodes, n)
		f.wg.Add(1)
		go func() {
			defer f.wg.Done()
			n.run(runCtx, cfg, id)
		}()
	}
	return f, nil
}

func (n *node) run(ctx context.Context, cfg Config, id *agent.Identity) {
	var opts []agent.SessionOption
	if cfg.HeartbeatInterval > 0 {
		opts = append(opts, agent.WithHeartbeatInterval(cfg.HeartbeatInterval))
	}
	for ctx.Err() == nil {
		n.sessions.Add(1)
		err := agent.RunSession(ctx, cfg.Addr, id, n.rt, cfg.Logger, opts...)
		if err != nil && ctx.Err() == nil {
			cfg.Logger.Warn("stressagent: session ended", slog.String("node_id", id.NodeID), slog.String("error", err.Error()))
		}
		select {
		case <-ctx.Done():
		case <-time.After(time.Second):
		}
	}
}

// NodeIDs returns the control plane's ids for the enrolled nodes, in enrollment order.
func (f *Fleet) NodeIDs() []string {
	ids := make([]string, 0, len(f.nodes))
	for _, n := range f.nodes {
		ids = append(ids, n.id)
	}
	return ids
}

// Containers counts containers across the fleet.
func (f *Fleet) Containers() int {
	total := 0
	for _, n := range f.nodes {
		total += n.rt.Count()
	}
	return total
}

// Sessions counts session attempts across the fleet, so reconnect churn is visible.
func (f *Fleet) Sessions() int64 {
	var total int64
	for _, n := range f.nodes {
		total += n.sessions.Load()
	}
	return total
}

// Stop ends every session and waits for the nodes to return.
func (f *Fleet) Stop() {
	f.cancel()
	f.wg.Wait()
}

var _ docker.Runtime = (*Runtime)(nil)
