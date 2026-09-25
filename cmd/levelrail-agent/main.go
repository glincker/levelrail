// Command levelrail-agent is the node agent binary (the
// repo layout names it, this is the first pass to actually build it).
// Dials out to the control plane (ADR 003, never
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
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	dockerclient "github.com/docker/docker/client"

	"github.com/GLINCKER/levelrail/internal/agent"
	"github.com/GLINCKER/levelrail/internal/build"
	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/gpu"
	"github.com/GLINCKER/levelrail/internal/version"
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

	// buildKitConnectTimeout bounds the one-time BuildKit connection at
	// startup, matching cmd/levelrail's own.
	buildKitConnectTimeout = 10 * time.Second
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

	if len(os.Args) > 1 && os.Args[1] == "reenroll" {
		return runReenroll(ctx, addr, identityFilePath(), logger)
	}

	logger.Info("starting", slog.String("version", version.Version))

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

	builder, closeBuilder := loadBuildRunner(ctx, logger)
	if closeBuilder != nil {
		defer func() {
			if cerr := closeBuilder(); cerr != nil {
				logger.Error("closing buildkit client", slog.String("error", cerr.Error()))
			}
		}()
	}

	meshCfg, err := setupAgentMesh(ctx, id.NodeID, logger)
	if err != nil {
		// Not fatal, same reasoning cmd/levelrail's own setupMesh call
		// site already applies: a misconfigured, opted-in mesh
		// (APP_MESH_ENABLED=1) should not stop this node from serving
		// Docker operations, which is this binary's actual job.
		logger.Warn("mesh not configured", slog.String("error", err.Error()))
	}
	if meshCfg != nil {
		defer meshCfg.close()
	}

	file := agent.NewIdentityFile(identityFilePath())
	renewer := agent.NewRenewer(agent.NewIdentityHolder(id), file, agent.CheckIdentityAt(addr),
		agent.RenewConfig{Fraction: floatFromEnv("APP_AGENT_CERT_RENEW_FRACTION")}, logger)
	runReconnectLoop(ctx, addr, renewer, file, client, builder, meshCfg, logger)
	return nil
}

// loadBuildRunner connects to this node's own BuildKit, so the control
// plane can dispatch builds here once an operator marks this node
// build-capable. Non-fatal: a node whose BuildKit is unreachable still
// serves every container operation, and a dispatched build fails with a
// clear reason rather than the agent refusing to start.
func loadBuildRunner(ctx context.Context, logger *slog.Logger) (agent.BuildRunner, func() error) {
	rawDockerCli, err := dockerclient.NewClientWithOpts(dockerclient.FromEnv, dockerclient.WithAPIVersionNegotiation())
	if err != nil {
		logger.Warn("no docker client for buildkit: this node cannot run dispatched builds", slog.String("error", err.Error()))
		return nil, nil
	}

	connectCtx, cancel := context.WithTimeout(ctx, buildKitConnectTimeout)
	buildClient, err := build.NewClient(connectCtx, rawDockerCli)
	cancel()
	if err != nil {
		_ = rawDockerCli.Close()
		logger.Warn("no buildkit connection: this node cannot run dispatched builds", slog.String("error", err.Error()))
		return nil, nil
	}

	return buildClient, func() error {
		buildErr := buildClient.Close()
		dockerErr := rawDockerCli.Close()
		if buildErr != nil {
			return buildErr
		}
		return dockerErr
	}
}

// runReconnectLoop calls agent.RunSession repeatedly with exponential
// backoff between attempts, until ctx is cancelled. It never gives up on
// its own: a reverse-dialed agent that stops retrying would be worse than
// the SSH-per-command tools ADR 003 rejected.
func runReconnectLoop(ctx context.Context, addr string, renewer *agent.Renewer, file *agent.IdentityFile, rt docker.Runtime, builder agent.BuildRunner, meshCfg *meshAgentSetup, logger *slog.Logger) {
	holder := renewer.Holder()
	opts := []agent.SessionOption{agent.WithRenewer(renewer)}
	if builder != nil {
		opts = append(opts, agent.WithBuildRunner(builder))
	}
	if meshCfg != nil {
		opts = append(opts, agent.WithMesh(holder.Current().NodeID, meshCfg.sink))
	}
	if rl, ok := rt.(gpu.RuntimeLister); ok {
		opts = append(opts, agent.WithGPUProbe(func(ctx context.Context) gpu.Info {
			return gpu.Detect(ctx, gpu.ExecRunner{}, rl)
		}))
	}
	if d := heartbeatIntervalFromEnv(); d > 0 {
		opts = append(opts, agent.WithHeartbeatInterval(d))
	}
	if t, timeout := keepaliveFromEnv(); t > 0 || timeout > 0 {
		opts = append(opts, agent.WithKeepalive(t, timeout))
	}

	delay := reconnectBaseDelay
	for {
		if ctx.Err() != nil {
			return
		}

		var err error
		if holder.Current().Expired(time.Now()) && !adoptIdentityFromDisk(holder, file, logger) {
			err = errors.New("certificate expired")
			delay = reconnectMaxDelay
		} else {
			err = agent.RunSession(ctx, addr, holder.Current(), rt, logger, opts...)
		}
		if ctx.Err() != nil {
			return // shutting down: the session ending is expected, not a failure
		}
		if agent.IsAuthRejection(err) || holder.Current().Expired(time.Now()) {
			if adoptIdentityFromDisk(holder, file, logger) {
				delay = reconnectBaseDelay
				continue
			}
			logger.Error("control plane no longer accepts this node's certificate: generate a re-enroll token for this node in the dashboard or CLI, then run: "+reenrollCommandHint(),
				slog.String("node_id", holder.Current().NodeID), slog.String("error", err.Error()))
		} else {
			logger.Warn("session ended, reconnecting",
				slog.String("error", err.Error()), slog.Duration("delay", delay))
		}

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

// adoptIdentityFromDisk switches to the identity file's contents when it
// holds a different, unexpired certificate, which is how a re-enrollment
// run as a separate command reaches the running agent without a restart.
func adoptIdentityFromDisk(holder *agent.IdentityHolder, file *agent.IdentityFile, logger *slog.Logger) bool {
	onDisk, err := file.Load()
	if err != nil || onDisk.Expired(time.Now()) || string(onDisk.ClientCertPEM) == string(holder.Current().ClientCertPEM) {
		return false
	}
	holder.Set(onDisk)
	logger.Info("picked up a new identity from disk", slog.String("node_id", onDisk.NodeID))
	return true
}

func reenrollCommandHint() string {
	return "APP_REENROLL_TOKEN=<token> " + filepath.Base(os.Args[0]) + " reenroll"
}

// runReenroll implements "<agent> reenroll": exchanges APP_REENROLL_TOKEN
// for a new certificate for this node and saves it. A running agent picks
// it up at its next reconnect.
func runReenroll(ctx context.Context, addr, path string, logger *slog.Logger) error {
	token := os.Getenv("APP_REENROLL_TOKEN")
	if token == "" {
		return fmt.Errorf("APP_REENROLL_TOKEN must be set to a re-enroll token generated for this node")
	}
	file := agent.NewIdentityFile(path)
	var nodeID string
	var opts []agent.EnrollOption
	if fp := os.Getenv("APP_CA_FINGERPRINT"); fp != "" {
		opts = append(opts, agent.WithPinnedCAFingerprint(fp))
	}
	if existing, err := file.Load(); err == nil {
		nodeID = existing.NodeID
		opts = append(opts, agent.WithTrustedCA(existing.CACertPEM))
	} else if len(opts) == 0 {
		logger.Warn("no identity file and APP_CA_FINGERPRINT is not set: trusting the control plane's certificate on first use")
	}
	id, err := agent.DialReenroll(ctx, addr, token, nodeID, opts...)
	if err != nil {
		return fmt.Errorf("re-enroll: %w", err)
	}
	if err := file.Save(id); err != nil {
		return fmt.Errorf("persist identity to %s: %w", path, err)
	}
	if err := file.Confirm(); err != nil {
		return err
	}
	logger.Info("re-enrolled: a running agent picks up the new certificate at its next reconnect, or start the agent now", slog.String("node_id", id.NodeID))
	return nil
}

// loadOrEnroll loads a previously persisted identity from path, or, if
// none exists yet, enrolls with the control plane using APP_JOIN_TOKEN
// and persists the result. A node only ever enrolls once in its
// lifetime; every subsequent run of this binary against the same
// identity file just loads what's already there.
func loadOrEnroll(ctx context.Context, addr, path string, logger *slog.Logger) (*agent.Identity, error) {
	file := agent.NewIdentityFile(path)
	id, err := file.ResolveStaged(ctx, agent.CheckIdentityAt(addr))
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

	var enrollOpts []agent.EnrollOption
	if fp := os.Getenv("APP_CA_FINGERPRINT"); fp != "" {
		enrollOpts = append(enrollOpts, agent.WithPinnedCAFingerprint(fp))
	} else {
		logger.Warn("APP_CA_FINGERPRINT is not set: trusting the control plane's certificate on first use for enrollment")
	}
	logger.Info("enrolling with control plane", slog.String("addr", addr), slog.String("node_name", nodeName))
	id, err = agent.DialEnroll(ctx, addr, token, nodeName, enrollOpts...)
	if err != nil {
		return nil, fmt.Errorf("enroll: %w", err)
	}
	if err := file.Save(id); err != nil {
		return nil, fmt.Errorf("persist identity to %s: %w", path, err)
	}
	logger.Info("enrolled", slog.String("node_id", id.NodeID))
	return id, nil
}

func identityFilePath() string {
	p := os.Getenv("APP_AGENT_IDENTITY_FILE")
	if p == "" {
		p = defaultIdentityFile
	}
	return p
}

// heartbeatIntervalFromEnv reads APP_NODE_HEARTBEAT_INTERVAL, the same
// env var name cmd/levelrail's own main.go reads for the control
// plane's local-node self-heartbeat, so one setting governs both
// cadences by default: how often this agent sends an unprompted
// Heartbeat frame up its Session stream (internal/agent.WithHeartbeatInterval).
// A zero return means unset or unparseable: runReconnectLoop leaves
// agent.RunSession's own default in place rather than passing a zero
// duration through.
func heartbeatIntervalFromEnv() time.Duration {
	raw := os.Getenv("APP_NODE_HEARTBEAT_INTERVAL")
	if raw == "" {
		return 0
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		return 0
	}
	return d
}

// keepaliveFromEnv reads APP_NODE_KEEPALIVE_TIME/APP_NODE_KEEPALIVE_TIMEOUT,
// the same env var names cmd/levelrail's own main.go reads for the agent
// gRPC server's side of this connection's HTTP/2 PING keepalive
// (internal/agent.WithKeepalive). Either returning zero means
// runReconnectLoop leaves agent.RunSession's own defaults in place.
func keepaliveFromEnv() (t, timeout time.Duration) {
	if raw := os.Getenv("APP_NODE_KEEPALIVE_TIME"); raw != "" {
		if d, err := time.ParseDuration(raw); err == nil {
			t = d
		}
	}
	if raw := os.Getenv("APP_NODE_KEEPALIVE_TIMEOUT"); raw != "" {
		if d, err := time.ParseDuration(raw); err == nil {
			timeout = d
		}
	}
	return t, timeout
}

// floatFromEnv returns name parsed as a float, or 0 when unset or invalid
// so the caller's default applies.
func floatFromEnv(name string) float64 {
	v, err := strconv.ParseFloat(os.Getenv(name), 64)
	if err != nil {
		return 0
	}
	return v
}
