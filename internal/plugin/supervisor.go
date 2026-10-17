package plugin

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/plugin/pluginpb"
)

const (
	containerSocketDir = "/run/plugin"
	socketFile         = "plugin.sock"
	startPollInterval  = 200 * time.Millisecond
	maxLastErrorLen    = 500
	fallbackUID        = 65532
)

type instance struct {
	opMu sync.Mutex

	mu           sync.Mutex
	client       rpcClient
	closer       io.Closer
	failures     int
	nextAttempt  time.Time
	healthFails  int
	breakerFails int
	breakerUntil time.Time
	sem          chan struct{}
}

func (m *Manager) inst(name string) *instance {
	m.mu.Lock()
	defer m.mu.Unlock()
	in, ok := m.instances[name]
	if !ok {
		in = &instance{sem: make(chan struct{}, m.cfg.MaxConcurrent)}
		m.instances[name] = in
	}
	return in
}

func (m *Manager) dropInstance(name string) {
	m.mu.Lock()
	in := m.instances[name]
	delete(m.instances, name)
	m.mu.Unlock()
	if in != nil {
		in.closeClient()
	}
}

func (in *instance) closeClient() {
	in.mu.Lock()
	defer in.mu.Unlock()
	if in.closer != nil {
		_ = in.closer.Close()
	}
	in.client, in.closer = nil, nil
}

func (in *instance) connected() rpcClient {
	in.mu.Lock()
	defer in.mu.Unlock()
	return in.client
}

// Running reports whether the plugin has a live, handshaken connection.
func (m *Manager) Running(name string) bool {
	m.mu.Lock()
	in := m.instances[name]
	m.mu.Unlock()
	return in != nil && in.connected() != nil
}

func (m *Manager) containerName(name string) string { return m.cfg.Namespace + "-plugin-" + name }

func (m *Manager) labelKey() string { return m.cfg.Namespace + ".plugin" }

func (m *Manager) runDir(name string) string {
	return filepath.Join(m.cfg.DataDir, "plugins", name, "run")
}

// Run reconciles on an interval until ctx ends.
func (m *Manager) Run(ctx context.Context) {
	m.Reconcile(ctx)
	t := time.NewTicker(m.cfg.ReconcileInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			m.Reconcile(ctx)
		}
	}
}

// Reconcile converges running containers to the stored plugin state. It is
// level-triggered and safe to call at any time.
func (m *Manager) Reconcile(ctx context.Context) {
	recs, err := m.store.ListPlugins(ctx)
	if err != nil {
		m.log.Error("plugin reconcile: list failed", "error", err)
		return
	}
	known := map[string]bool{}
	for _, rec := range recs {
		known[rec.Name] = true
		switch rec.State {
		case StateEnabled:
			m.ensureRunning(ctx, rec)
		case StateRemoving:
			if err := m.finishRemove(ctx, rec.Name); err != nil {
				m.log.Warn("plugin removal retry failed", "plugin", rec.Name, "error", err)
			}
		default:
			m.stopPlugin(ctx, rec.Name)
		}
	}
	m.removeOrphans(ctx, known)
}

func (m *Manager) removeOrphans(ctx context.Context, known map[string]bool) {
	cs, err := m.docker.ListByPrefix(ctx, m.containerName(""))
	if err != nil {
		m.log.Warn("plugin reconcile: list containers failed", "error", err)
		return
	}
	for _, c := range cs {
		name := c.Labels[m.labelKey()]
		if name == "" || known[name] {
			continue
		}
		if err := m.docker.Remove(ctx, c.ID, true); err != nil {
			m.log.Warn("plugin orphan container removal failed", "container", c.Name, "error", err)
		}
	}
}

func (m *Manager) stopPlugin(ctx context.Context, name string) {
	in := m.inst(name)
	in.opMu.Lock()
	defer in.opMu.Unlock()
	in.closeClient()
	if err := m.removeContainer(ctx, name); err != nil {
		m.log.Warn("plugin stop: container cleanup failed", "plugin", name, "error", err)
	}
}

func (m *Manager) removeContainer(ctx context.Context, name string) error {
	st, err := m.docker.InspectByName(ctx, m.containerName(name))
	if err != nil {
		return fmt.Errorf("inspect: %w", err)
	}
	if st == nil {
		return nil
	}
	if st.Running {
		if err := m.docker.Stop(ctx, st.ID, m.cfg.StopTimeout); err != nil {
			m.log.Warn("plugin stop failed, forcing removal", "plugin", name, "error", err)
		}
	}
	if err := m.docker.Remove(ctx, st.ID, true); err != nil {
		return fmt.Errorf("remove: %w", err)
	}
	return nil
}

func (m *Manager) ensureRunning(ctx context.Context, rec Record) {
	in := m.inst(rec.Name)
	in.opMu.Lock()
	defer in.opMu.Unlock()

	in.mu.Lock()
	wait := m.now().Before(in.nextAttempt)
	in.mu.Unlock()
	if wait {
		return
	}
	if err := m.verifyRecord(ctx, rec); err != nil {
		m.fail(ctx, rec.Name, in, fmt.Errorf("package no longer verifies: %w", err))
		return
	}
	if client := in.connected(); client != nil {
		m.checkHealth(ctx, rec, in, client)
		return
	}
	if err := m.start(ctx, rec, in); err != nil {
		m.fail(ctx, rec.Name, in, err)
		return
	}
	in.mu.Lock()
	in.failures, in.healthFails, in.breakerFails = 0, 0, 0
	in.mu.Unlock()
	m.setLastError(ctx, rec.Name, "")
}

func (m *Manager) verifyRecord(ctx context.Context, rec Record) error {
	man, err := ParseManifest([]byte(rec.Manifest))
	if err != nil {
		return err
	}
	if man.Image != rec.Image || man.Digest() != rec.Digest || man.Name != rec.Name {
		return errors.New("stored record does not match its signed manifest")
	}
	keys, err := m.store.ListKeys(ctx)
	if err != nil {
		return err
	}
	return Verify(keys, rec.KeyID, []byte(rec.Manifest), rec.Digest, rec.Signature)
}

func (m *Manager) checkHealth(ctx context.Context, rec Record, in *instance, client rpcClient) {
	st, err := m.docker.InspectByName(ctx, m.containerName(rec.Name))
	if err != nil || st == nil || !st.Running {
		m.fail(ctx, rec.Name, in, errors.New("plugin container is not running"))
		return
	}
	hctx, cancel := context.WithTimeout(ctx, m.cfg.HealthTimeout)
	defer cancel()
	resp, err := client.Health(hctx, &pluginpb.HealthRequest{})
	if err == nil && resp.GetOk() {
		in.mu.Lock()
		in.healthFails = 0
		in.mu.Unlock()
		return
	}
	in.mu.Lock()
	in.healthFails++
	over := in.healthFails >= m.cfg.HealthFailures
	in.mu.Unlock()
	if over {
		m.fail(ctx, rec.Name, in, errors.New("health check failed repeatedly"))
	}
}

func (m *Manager) containerSpec(rec Record) docker.ContainerSpec {
	uid, gid := m.cfg.UID, m.cfg.GID
	if uid <= 0 {
		uid, gid = fallbackUID, fallbackUID
	}
	spec := docker.ContainerSpec{
		Name:  m.containerName(rec.Name),
		Image: rec.Image,
		Env:   map[string]string{"PLUGIN_SOCKET": containerSocketDir + "/" + socketFile},
		Resources: &docker.Resources{
			MemoryBytes: m.cfg.MemoryBytes, SwapMemoryBytes: m.cfg.MemoryBytes, NanoCPUs: m.cfg.NanoCPUs,
		},
		BindMounts: []docker.BindMount{{HostPath: m.runDir(rec.Name), ContainerPath: containerSocketDir}},
		Labels:     map[string]string{m.labelKey(): rec.Name},
		Hardened:   true,
		User:       strconv.Itoa(uid) + ":" + strconv.Itoa(gid),
		PidsLimit:  m.cfg.PidsLimit,
	}
	if len(rec.OutboundHosts) == 0 {
		spec.NetworkMode = "none"
	} else {
		spec.Network = &docker.NetworkAttachment{Name: m.cfg.Namespace + "-plugins", Alias: rec.Name}
	}
	return spec
}

func (m *Manager) start(ctx context.Context, rec Record, in *instance) error {
	dir := m.runDir(rec.Name)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("prepare socket dir: %w", err)
	}
	if err := os.Chmod(dir, 0o700); err != nil { //nolint:gosec // the socket dir must be owner-only, 0700 is the point
		return fmt.Errorf("secure socket dir: %w", err)
	}
	sock := filepath.Join(dir, socketFile)
	_ = os.Remove(sock)

	if st, err := m.docker.InspectByName(ctx, m.containerName(rec.Name)); err != nil {
		return fmt.Errorf("inspect before start: %w", err)
	} else if st != nil {
		if err := m.docker.Remove(ctx, st.ID, true); err != nil {
			return fmt.Errorf("remove stale container: %w", err)
		}
	}
	spec := m.containerSpec(rec)
	if spec.Network != nil {
		if _, err := m.docker.EnsureNetwork(ctx, spec.Network.Name); err != nil {
			return fmt.Errorf("ensure plugin network: %w", err)
		}
	}
	id, err := m.docker.Create(ctx, spec)
	if err != nil {
		return fmt.Errorf("create container: %w", err)
	}
	if err := m.docker.Start(ctx, id); err != nil {
		if rerr := m.docker.Remove(ctx, id, true); rerr != nil {
			m.log.Warn("plugin start failed and cleanup failed, will retry", "plugin", rec.Name, "error", rerr)
		}
		return fmt.Errorf("start container: %w", err)
	}
	client, closer, err := m.connect(ctx, rec, sock)
	if err != nil {
		if terr := m.removeContainer(ctx, rec.Name); terr != nil {
			m.log.Warn("plugin handshake failed and cleanup failed, will retry", "plugin", rec.Name, "error", terr)
		}
		return err
	}
	in.mu.Lock()
	in.client, in.closer = client, closer
	in.mu.Unlock()
	return nil
}

func (m *Manager) connect(ctx context.Context, rec Record, sock string) (rpcClient, io.Closer, error) {
	maxSend := m.cfg.MaxEventBytes + m.cfg.MaxConfigBytes + m.cfg.MaxSecretBytes + 4<<10
	client, closer, err := m.dial(ctx, sock, maxSend, maxSend)
	if err != nil {
		return nil, nil, err
	}
	deadline := m.now().Add(m.cfg.StartTimeout)
	var lastErr error
	for {
		dctx, cancel := context.WithTimeout(ctx, m.cfg.HealthTimeout)
		desc, err := client.Describe(dctx, &pluginpb.DescribeRequest{HostContractVersions: ContractVersions})
		cancel()
		if err == nil {
			if verr := checkDescribe(rec, desc); verr != nil {
				_ = closer.Close()
				return nil, nil, verr
			}
			return client, closer, nil
		}
		lastErr = err
		if !m.now().Before(deadline) {
			_ = closer.Close()
			return nil, nil, fmt.Errorf("plugin did not answer Describe in time: %w", lastErr)
		}
		select {
		case <-ctx.Done():
			_ = closer.Close()
			return nil, nil, ctx.Err()
		case <-time.After(startPollInterval):
		}
	}
}

func checkDescribe(rec Record, d *pluginpb.DescribeResponse) error {
	if !slices.Contains(ContractVersions, d.GetContractVersion()) {
		return fmt.Errorf("plugin speaks contract version %d, host supports %v", d.GetContractVersion(), ContractVersions)
	}
	if d.GetName() != rec.Name || d.GetKind() != rec.Kind {
		return fmt.Errorf("plugin identifies as %q (%s), expected %q (%s)", d.GetName(), d.GetKind(), rec.Name, rec.Kind)
	}
	for _, a := range d.GetRequestedAbilities() {
		if !slices.Contains(rec.RequestedAbilities, a) {
			return fmt.Errorf("plugin asks for ability %q that its signed manifest does not declare", a)
		}
	}
	return nil
}

func (m *Manager) fail(ctx context.Context, name string, in *instance, cause error) {
	in.closeClient()
	if err := m.removeContainer(ctx, name); err != nil {
		m.log.Warn("plugin cleanup after failure failed, will retry", "plugin", name, "error", err)
	}
	in.mu.Lock()
	in.failures++
	delay := m.cfg.BackoffBase
	for i := 1; i < in.failures && delay < m.cfg.BackoffMax; i++ {
		delay *= 2
	}
	delay = min(delay, m.cfg.BackoffMax)
	in.nextAttempt = m.now().Add(delay)
	in.healthFails = 0
	in.mu.Unlock()
	m.setLastError(ctx, name, cause.Error())
	m.log.Warn("plugin unavailable", "plugin", name, "error", cause, "retry_in", delay.String())
}

func (m *Manager) setLastError(ctx context.Context, name, msg string) {
	rec, err := m.store.GetPlugin(ctx, name)
	if err != nil || rec.LastError == msg {
		return
	}
	msg = strings.ToValidUTF8(msg, "")
	if len(msg) > maxLastErrorLen {
		msg = msg[:maxLastErrorLen]
	}
	if err := m.store.SetLastError(ctx, name, msg, stamp(m.now())); err != nil {
		m.log.Warn("plugin status update failed", "plugin", name, "error", err)
	}
}
