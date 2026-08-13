// Command levelrail-agent is TASKS.md 3.2's node agent binary
// (CLAUDE.md 5's repo layout names it, this is the first pass to
// actually build it). Dials out to the control plane (ADR 003, never
// accepts an inbound connection), enrolls once using a one-time join
// token, persists the resulting identity, then holds one persistent
// Session open for as long as it runs, serving every incoming request
// against this node's own local Docker daemon (internal/docker, the
// exact "never shell the CLI" invariant the control plane's own Docker
// access already follows).
//
// This binary is a thin main(): every real behavior (enrollment, the
// Session loop, request dispatch against docker.Runtime) lives in
// internal/agent, already tested there without a real process or
// network connection. What's here is env/flag parsing, identity file
// persistence, and the reconnect loop ADR 003's Consequences section
// calls out as real, standing Phase 3 work.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/GLINCKER/levelrail/internal/agent"
	"github.com/GLINCKER/levelrail/internal/docker"
)

const (
	// defaultIdentityFile is where an enrolled node's mTLS identity
	// persists across restarts, the local counterpart to the control
	// plane's own agent-ca.{crt,key}.pem (cmd/levelrail/main.go).
	defaultIdentityFile = "./levelrail-agent-identity.json"

	// reconnectBaseDelay/reconnectMaxDelay bound RunSession's own
	// per-attempt failures with exponential backoff, so a control plane
	// that's briefly unreachable doesn't get hammered with reconnect
	// attempts, but a genuinely transient blip still recovers in
	// seconds, not minutes.
	reconnectBaseDelay = 1 * time.Second
	reconnectMaxDelay  = 30 * time.Second
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	if err := run(logger); err != nil {
		logger.Error("exited with error", slog.String("error", err.Error()))
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	addr := os.Getenv("APP_CONTROL_PLANE_ADDR")
	if addr == "" {
		return fmt.Errorf("APP_CONTROL_PLANE_ADDR must be set (host:port of the control plane's agent gRPC listener)")
	}

	id, err := loadOrEnroll(ctx, addr, identityFilePath(), logger)
	if err != nil {
		return err
	}

	client, err := docker.NewClient()
	if err != nil {
		return fmt.Errorf("connect to local docker: %w", err)
	}
	defer func() {
		if cerr := client.Close(); cerr != nil {
			logger.Error("closing docker client", slog.String("error", cerr.Error()))
		}
	}()

	runReconnectLoop(ctx, addr, id, client, logger)
	return nil
}

// runReconnectLoop calls agent.RunSession repeatedly with exponential
// backoff between attempts, until ctx is cancelled. No error return: it
// never gives up permanently on its own, by design. ADR 003's own
// Consequences section names reconnection as real, standing Phase 3
// work ("real, tested" reconnection), not a footnote: a reverse-dialed
// agent that gives up permanently on the first disconnect would be
// strictly worse than the SSH-per-command tools ADR 003 rejected, which
// at least retry by construction on the next invocation. Only ctx
// cancellation (process shutdown) ends this loop.
func runReconnectLoop(ctx context.Context, addr string, id *agent.Identity, rt docker.Runtime, logger *slog.Logger) {
	delay := reconnectBaseDelay
	for {
		if ctx.Err() != nil {
			return
		}

		err := agent.RunSession(ctx, addr, id, rt, logger)
		if ctx.Err() != nil {
			return // shutting down: the session ending is expected, not a failure
		}
		logger.Warn("session ended, reconnecting",
			slog.String("error", err.Error()), slog.Duration("delay", delay))

		select {
		case <-ctx.Done():
			return
		case <-time.After(delay):
		}

		delay *= 2
		if delay > reconnectMaxDelay {
			delay = reconnectMaxDelay
		}
	}
}

// loadOrEnroll loads a previously persisted identity from path, or, if
// none exists yet, enrolls with the control plane using APP_JOIN_TOKEN
// and persists the result. A node only ever enrolls once in its
// lifetime; every subsequent run of this binary against the same
// identity file just loads what's already there.
func loadOrEnroll(ctx context.Context, addr, path string, logger *slog.Logger) (*agent.Identity, error) {
	id, err := loadIdentity(path)
	if err == nil {
		return id, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("load identity from %s: %w", path, err)
	}

	token := os.Getenv("APP_JOIN_TOKEN")
	if token == "" {
		return nil, fmt.Errorf("no identity found at %s and APP_JOIN_TOKEN is not set: this node has never enrolled", path)
	}
	nodeName := os.Getenv("APP_NODE_NAME")
	if nodeName == "" {
		h, hostErr := os.Hostname()
		if hostErr != nil {
			return nil, fmt.Errorf("APP_NODE_NAME is not set and the local hostname is unavailable: %w", hostErr)
		}
		nodeName = h
	}

	logger.Info("enrolling with control plane", slog.String("addr", addr), slog.String("node_name", nodeName))
	id, err = agent.DialEnroll(ctx, addr, token, nodeName)
	if err != nil {
		return nil, fmt.Errorf("enroll: %w", err)
	}
	if err := saveIdentity(path, id); err != nil {
		return nil, fmt.Errorf("persist identity to %s: %w", path, err)
	}
	logger.Info("enrolled", slog.String("node_id", id.NodeID))
	return id, nil
}

// identityFileFormat is identityFilePath's on-disk JSON shape.
// []byte fields marshal as base64, so the PEM data (already ASCII) ends
// up double-encoded; a real inefficiency for a file this small and
// written once at enrollment, not worth a custom encoding to avoid.
type identityFileFormat struct {
	NodeID        string `json:"node_id"`
	ClientCertPEM []byte `json:"client_cert_pem"`
	ClientKeyPEM  []byte `json:"client_key_pem"`
	CACertPEM     []byte `json:"ca_cert_pem"`
}

func loadIdentity(path string) (*agent.Identity, error) {
	data, err := os.ReadFile(path) //nolint:gosec // operator-controlled config path, not user input
	if err != nil {
		return nil, err
	}
	var f identityFileFormat
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("parse identity file: %w", err)
	}
	return &agent.Identity{
		NodeID:        f.NodeID,
		ClientCertPEM: f.ClientCertPEM,
		ClientKeyPEM:  f.ClientKeyPEM,
		CACertPEM:     f.CACertPEM,
	}, nil
}

func saveIdentity(path string, id *agent.Identity) error {
	f := identityFileFormat{
		NodeID:        id.NodeID,
		ClientCertPEM: id.ClientCertPEM,
		ClientKeyPEM:  id.ClientKeyPEM,
		CACertPEM:     id.CACertPEM,
	}
	data, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return fmt.Errorf("encode identity: %w", err)
	}
	// 0o600: this file contains the node's private key, the same
	// "not world/group readable" requirement CA key persistence
	// already applies control-plane-side (cmd/levelrail/main.go's
	// loadOrGenerateAgentCA).
	return os.WriteFile(path, data, 0o600)
}

func identityFilePath() string {
	p := os.Getenv("APP_AGENT_IDENTITY_FILE")
	if p == "" {
		p = defaultIdentityFile
	}
	return p
}
