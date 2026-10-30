// Thin main: every behavior (enrollment, Session loop, Docker dispatch) lives in internal/agent.
// This file only parses flags and env, persists identity, and runs the reconnect loop.
package main

import (
	"context"
	"errors"
	"fmt"
	"github.com/GLINCKER/levelrail/internal/cloudmeta"
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
	"github.com/GLINCKER/levelrail/internal/dockerguard"
	"github.com/GLINCKER/levelrail/internal/gpu"
	"github.com/GLINCKER/levelrail/internal/version"
)

const (
	// defaultIdentityFile is where an enrolled node's mTLS identity
	// persists across restarts, the local counterpart to the control
	// plane's own agent-ca.{crt,key}.pem (cmd/levelrail/main.go).
	defaultIdentityFile = "./levelrail-agent-identity.json"

	// Exponential backoff on RunSession failures, so a briefly unreachable control plane is not hammered.
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

	guard, err := bootAgentDockerGuard(ctx, logger)
	if err != nil {
		return err
	}
	clientOpts := []docker.ClientOption{docker.WithCreateDeclarer(guard.Grants)}
	if guard.Host != "" {
		clientOpts = append(clientOpts, docker.WithHost(guard.Host))
	}
	client, err := docker.NewClient(clientOpts...)
	if err != nil {
		return fmt.Errorf("connect to local docker: %w", err)
	}
	defer func() {
		if cerr := client.Close(); cerr != nil {
			logger.Error("closing docker client", slog.String("error", cerr.Error()))
		}
	}()
	dockerguard.LogPrivilege(ctx, logger, client)

	builder, closeBuilder := loadBuildRunner(ctx, logger, guard.Host)
	if closeBuilder != nil {
		defer func() {
			if cerr := closeBuilder(); cerr != nil {
				logger.Error("closing buildkit client", slog.String("error", cerr.Error()))
			}
		}()
	}

	meshCfg, err := setupAgentMesh(ctx, id.NodeID, logger)
	if err != nil {
		// Non-fatal: a misconfigured mesh must not stop this node from serving Docker operations.
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

// Non-fatal: an unreachable BuildKit still leaves container operations working.
func loadBuildRunner(ctx context.Context, logger *slog.Logger, guardHost string) (agent.BuildRunner, func() error) {
	rawOpts := []dockerclient.Opt{dockerclient.FromEnv, dockerclient.WithAPIVersionNegotiation()}
	if guardHost != "" {
		rawOpts = append(rawOpts, dockerclient.WithHost(guardHost))
	}
	rawDockerCli, err := dockerclient.NewClientWithOpts(rawOpts...)
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

// Retries until ctx is cancelled: a reverse-dialed agent that gives up is unreachable.
func runReconnectLoop(ctx context.Context, addr string, renewer *agent.Renewer, file *agent.IdentityFile, rt docker.Runtime, builder agent.BuildRunner, meshCfg *meshAgentSetup, logger *slog.Logger) {
	holder := renewer.Holder()
	opts := []agent.SessionOption{agent.WithRenewer(renewer)}
	if builder != nil {
		opts = append(opts, agent.WithBuildRunner(builder))
	}
	if meshCfg != nil {
		opts = append(opts, agent.WithMesh(holder.Current().NodeID, meshCfg.sink))
	}
	opts = append(opts, agent.WithHostProbe(agent.NewHostProbe(rt, agentDataDir())))
	opts = append(opts, agent.WithOnboardingProbe(
		agent.NewOnboardingProbe(rt, agentDataDir(), meshCfg != nil, cloudmeta.SystemConfig()),
		agent.OnboardingIntervalFromEnv(os.LookupEnv)))
	if rl, ok := rt.(gpu.RuntimeLister); ok {
		opts = append(opts, agent.WithGPUProbe(func(ctx context.Context) gpu.Info {
			return gpu.Detect(ctx, gpu.ExecRunner{}, rl)
		}))
	}
	if dir := os.Getenv("APP_AGENT_STORAGE_DIR"); dir != "" {
		opts = append(opts, agent.WithFileStorage(dir))
	}
	if scfg, serr := agent.SilenceConfigFromEnv(os.LookupEnv); serr != nil {
		logger.Error("control plane silence watcher disabled", slog.String("error", serr.Error()))
	} else {
		w := agent.NewSilenceWatcher(scfg, holder.Current().NodeID, logger, randInt63n)
		go w.Run(ctx)
		opts = append(opts, agent.WithSilenceWatcher(w))
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
		sessionStart := time.Now()
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
		}

		wait, next := nextReconnect(delay, time.Since(sessionStart), randInt63n)
		delay = next
		if !agent.IsAuthRejection(err) && !holder.Current().Expired(time.Now()) {
			logger.Warn("session ended, reconnecting",
				slog.String("error", err.Error()), slog.Duration("delay", wait))
		}

		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
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
// envAllowUnpinnedEnroll opts an agent out of the CA fingerprint requirement.
const envAllowUnpinnedEnroll = "APP_ALLOW_UNPINNED_ENROLL"

// requirePinOrOptOut refuses first enrollment without a pinned CA, because the join token would go to whoever answers.
func requirePinOrOptOut(logger *slog.Logger) error {
	if os.Getenv(envAllowUnpinnedEnroll) != "1" {
		return fmt.Errorf("APP_CA_FINGERPRINT is not set: copy it from the join command or node page, or set %s=1 to trust the control plane on first use", envAllowUnpinnedEnroll)
	}
	logger.Warn("enrolling without a pinned CA fingerprint: trusting the control plane's certificate on first use")
	return nil
}

func runReenroll(ctx context.Context, addr, path string, logger *slog.Logger) error {
	token := os.Getenv("APP_REENROLL_TOKEN")
	if token == "" {
		return fmt.Errorf("APP_REENROLL_TOKEN must be set to a re-enroll token generated for this node")
	}
	file := agent.NewIdentityFile(path)
	if err := file.CheckWritable(); err != nil {
		return err
	}
	var nodeID string
	var opts []agent.EnrollOption
	if fp := os.Getenv("APP_CA_FINGERPRINT"); fp != "" {
		opts = append(opts, agent.WithPinnedCAFingerprint(fp))
	}
	if existing, err := file.Load(); err == nil {
		nodeID = existing.NodeID
		opts = append(opts, agent.WithTrustedCA(existing.CACertPEM))
	} else if len(opts) == 0 {
		if err := requirePinOrOptOut(logger); err != nil {
			return err
		}
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

// Enrolls only when no identity file exists yet; a node enrolls once per lifetime.
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

	if err := file.CheckWritable(); err != nil {
		return nil, err
	}

	var enrollOpts []agent.EnrollOption
	if fp := os.Getenv("APP_CA_FINGERPRINT"); fp != "" {
		enrollOpts = append(enrollOpts, agent.WithPinnedCAFingerprint(fp))
	} else if err := requirePinOrOptOut(logger); err != nil {
		return nil, err
	}
	logger.Info("enrolling with control plane", slog.String("addr", addr), slog.String("node_name", nodeName))
	id, err = agent.DialEnroll(ctx, addr, token, nodeName, enrollOpts...)
	if err != nil {
		return nil, fmt.Errorf("enroll: %w", err)
	}
	if err := saveIdentityVerified(ctx, file, id); err != nil {
		return nil, fmt.Errorf("persist identity to %s (the join token is already spent: clean up this node in the dashboard and enroll again): %w", path, err)
	}
	logger.Info("enrolled", slog.String("node_id", id.NodeID))
	return id, nil
}

// The join token is spent by now, so a transient write failure is retried and the file read back before the agent trusts it.
func saveIdentityVerified(ctx context.Context, file *agent.IdentityFile, id *agent.Identity) error {
	var err error
	for attempt := 0; attempt < 4; attempt++ {
		if attempt > 0 {
			select {
			case <-time.After(time.Duration(attempt) * 500 * time.Millisecond):
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		if err = file.Save(id); err != nil {
			continue
		}
		var got *agent.Identity
		if got, err = file.Load(); err == nil && got.NodeID == id.NodeID {
			return nil
		} else if err == nil {
			err = fmt.Errorf("identity read back with node id %q, want %q", got.NodeID, id.NodeID)
		}
	}
	return err
}

func agentDataDir() string {
	if d := os.Getenv("APP_DATA_DIR"); d != "" {
		return d
	}
	return filepath.Dir(identityFilePath())
}

// bootAgentDockerGuard puts the same Docker API guard the control plane runs
// in front of this node's daemon. Denials go to a node-local audit file.
func bootAgentDockerGuard(ctx context.Context, logger *slog.Logger) (dockerguard.Booted, error) {
	dataDir := agentDataDir()
	b, err := dockerguard.Boot(ctx, dockerguard.BootConfig{
		DataDir:         dataDir,
		ResolveSymlinks: !dockerguard.RunningInContainer(),
		Sink:            dockerguard.NewFileSink(filepath.Join(dataDir, dockerguard.FileSinkName)),
		Logger:          logger,
	})
	if err != nil {
		return dockerguard.Booted{}, fmt.Errorf("docker guard: %w", err)
	}
	return b, nil
}

func identityFilePath() string {
	p := os.Getenv("APP_AGENT_IDENTITY_FILE")
	if p == "" {
		p = defaultIdentityFile
	}
	return p
}

// Zero means unset or unparseable, so RunSession's default interval applies.
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

// Zero from either variable means RunSession keeps its own keepalive defaults.
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
