package plugin

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/plugin/egress"
)

// Egress modes. Open keeps the shared bridge; proxy forces every plugin
// connection through the allowlisting sidecar.
const (
	EgressOpen  = "open"
	EgressProxy = "proxy"

	// EnvEgressMode selects the mode, EnvEgressImage the image that runs the sidecar.
	EnvEgressMode  = "APP_PLUGIN_EGRESS_MODE"
	EnvEgressImage = "APP_PLUGIN_EGRESS_IMAGE"

	egressProxyPort  = "3128"
	egressRunDir     = "/run/egress"
	egressPolicyFile = "policy.sock"
	egressCacheFile  = "policy.json"
)

// ErrEgressUnavailable means proxy mode is on but the sidecar cannot be used.
// Plugins that need the network then stay down instead of falling back to open.
var ErrEgressUnavailable = errors.New("plugin egress proxy is unavailable")

type egressEntry struct {
	hash  egress.SecretHash
	hosts []string
}

// EgressMode reports the active mode, "open" unless proxy was asked for.
func (m *Manager) EgressMode() string {
	if m.cfg.EgressMode == EgressProxy {
		return EgressProxy
	}
	return EgressOpen
}

func (m *Manager) proxyMode() bool { return m.EgressMode() == EgressProxy }

func (m *Manager) pluginsNetwork() string { return m.cfg.Namespace + "-plugins" }

func (m *Manager) egressNetwork() string { return m.cfg.Namespace + "-plugin-egress" }

func (m *Manager) egressName() string { return m.cfg.Namespace + "-egress-proxy" }

func (m *Manager) egressDir() string { return filepath.Join(m.cfg.DataDir, "plugins", ".egress") }

func (m *Manager) egressSocket() string { return filepath.Join(m.egressDir(), egressPolicyFile) }

// ensurePluginNetwork creates the shared plugin network. In proxy mode it is
// Internal (no gateway) and replaced if it was created open; in open mode an
// existing network is never touched, so a default upgrade changes nothing.
func (m *Manager) ensurePluginNetwork(ctx context.Context) error {
	opts := docker.IsolatedNetworkOptions{DisableICC: true}
	if m.proxyMode() {
		// ICC stays on in proxy mode because the sidecar lives on this network;
		// per-plugin credentials, not the bridge, keep plugins apart at the proxy.
		opts = docker.IsolatedNetworkOptions{Internal: true, Recreate: true}
	}
	_, recreated, err := m.docker.EnsureIsolatedNetwork(ctx, m.pluginsNetwork(), opts)
	if err != nil {
		return err
	}
	if recreated && m.proxyMode() {
		m.resetAfterNetworkReplace(ctx)
	}
	return nil
}

// resetAfterNetworkReplace removes containers that lost their attachment so the
// next reconcile recreates them on the new network.
func (m *Manager) resetAfterNetworkReplace(ctx context.Context) {
	cs, err := m.docker.ListByPrefix(ctx, m.containerName(""))
	if err == nil {
		for _, c := range cs {
			if name := c.Labels[m.labelKey()]; name != "" {
				_ = m.docker.Remove(ctx, c.ID, true)
				m.dropInstance(name)
			}
		}
	}
	if st, _ := m.docker.InspectByName(ctx, m.egressName()); st != nil {
		_ = m.docker.Remove(ctx, st.ID, true)
	}
}

func (m *Manager) sidecarSpec() docker.ContainerSpec {
	uid, gid := m.cfg.UID, m.cfg.GID
	if uid <= 0 {
		uid, gid = fallbackUID, fallbackUID
	}
	env := map[string]string{
		egress.EnvPolicySocket: egressRunDir + "/" + egressPolicyFile,
		egress.EnvPolicyCache:  egressRunDir + "/" + egressCacheFile,
		egress.EnvListen:       "0.0.0.0:" + egressProxyPort,
	}
	for k, v := range m.cfg.EgressEnv {
		env[k] = v
	}
	return docker.ContainerSpec{
		Name:       m.egressName(),
		Image:      m.cfg.EgressImage,
		Command:    []string{"plugin-egress"},
		Env:        env,
		Resources:  &docker.Resources{MemoryBytes: m.cfg.MemoryBytes, SwapMemoryBytes: m.cfg.MemoryBytes, NanoCPUs: m.cfg.NanoCPUs},
		BindMounts: []docker.BindMount{{HostPath: m.egressDir(), ContainerPath: egressRunDir}},
		Labels:     map[string]string{m.labelKey() + ".egress": "proxy"},
		Hardened:   true,
		User:       strconv.Itoa(uid) + ":" + strconv.Itoa(gid),
		PidsLimit:  m.cfg.PidsLimit,
		// The sidecar is the only plugin-side container with a route out.
		Network:       &docker.NetworkAttachment{Name: m.egressNetwork(), Alias: m.egressName()},
		RestartPolicy: "unless-stopped",
	}
}

// ensureSidecar makes the proxy container exist and run on both networks.
func (m *Manager) ensureSidecar(ctx context.Context) error {
	if strings.TrimSpace(m.cfg.EgressImage) == "" {
		return fmt.Errorf("%w: set %s", ErrEgressUnavailable, EnvEgressImage)
	}
	if err := m.ensurePluginNetwork(ctx); err != nil {
		return fmt.Errorf("ensure plugin network: %w", err)
	}
	if st, err := m.docker.InspectByName(ctx, m.egressName()); err != nil {
		return fmt.Errorf("inspect egress proxy: %w", err)
	} else if st != nil && st.Running {
		return nil
	} else if st != nil {
		if err := m.docker.Remove(ctx, st.ID, true); err != nil {
			return fmt.Errorf("remove stale egress proxy: %w", err)
		}
	}
	if _, err := m.docker.EnsureNetwork(ctx, m.egressNetwork()); err != nil {
		return fmt.Errorf("ensure egress network: %w", err)
	}
	if err := mkdirPrivate(m.egressDir()); err != nil {
		return err
	}
	id, err := m.docker.Create(ctx, m.sidecarSpec())
	if err != nil {
		return fmt.Errorf("create egress proxy: %w", err)
	}
	if err := m.docker.NetworkConnect(ctx, m.pluginsNetwork(), id); err != nil {
		_ = m.docker.Remove(ctx, id, true)
		return fmt.Errorf("attach egress proxy to plugin network: %w", err)
	}
	if err := m.docker.Start(ctx, id); err != nil {
		_ = m.docker.Remove(ctx, id, true)
		return fmt.Errorf("start egress proxy: %w", err)
	}
	m.egMu.Lock()
	m.egDirty = true
	m.egMu.Unlock()
	return nil
}

// egressEnvFor registers a fresh credential for the plugin, pushes the policy
// and returns the proxy environment for its container. Any failure is an error
// so the plugin does not start with an open route.
func (m *Manager) egressEnvFor(ctx context.Context, rec Record) (map[string]string, error) {
	if err := m.ensureSidecar(ctx); err != nil {
		return nil, err
	}
	secret, hash, err := egress.NewCredential()
	if err != nil {
		return nil, err
	}
	m.egMu.Lock()
	if m.egPolicy == nil {
		m.egPolicy = map[string]egressEntry{}
	}
	m.egPolicy[rec.Name] = egressEntry{hash: hash, hosts: slices.Clone(rec.OutboundHosts)}
	m.egMu.Unlock()
	if err := m.pushPolicy(ctx); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrEgressUnavailable, err)
	}
	u := egress.ProxyURL(rec.Name, secret, m.egressName()+":"+egressProxyPort)
	return map[string]string{"HTTPS_PROXY": u, "HTTP_PROXY": u, "https_proxy": u, "http_proxy": u}, nil
}

// pushPolicy sends the current allowlists, retrying while the sidecar boots.
func (m *Manager) pushPolicy(ctx context.Context) error {
	m.egMu.Lock()
	m.egVersion++
	pol := egress.Policy{Version: m.egVersion}
	names := make([]string, 0, len(m.egPolicy))
	for n := range m.egPolicy {
		names = append(names, n)
	}
	slices.Sort(names)
	for _, n := range names {
		e := m.egPolicy[n]
		pol.Plugins = append(pol.Plugins, egress.PluginPolicy{Name: n, SecretHash: egress.HexHash(e.hash), Hosts: e.hosts})
	}
	m.egMu.Unlock()

	deadline := m.now().Add(m.cfg.StartTimeout)
	for {
		err := m.pushFn(ctx, m.egressSocket(), pol)
		if err == nil {
			m.egMu.Lock()
			m.egDirty = false
			m.egMu.Unlock()
			return nil
		}
		if !m.now().Before(deadline) {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(startPollInterval):
		}
	}
}

// forgetEgress drops a plugin's credential; the sidecar learns on the next push.
func (m *Manager) forgetEgress(name string) {
	m.egMu.Lock()
	defer m.egMu.Unlock()
	if _, ok := m.egPolicy[name]; ok {
		delete(m.egPolicy, name)
		m.egDirty = true
	}
}

// syncEgress runs each reconcile in proxy mode: it pushes a pending change.
func (m *Manager) syncEgress(ctx context.Context) {
	if !m.proxyMode() {
		return
	}
	m.egMu.Lock()
	dirty := m.egDirty
	m.egMu.Unlock()
	if !dirty {
		return
	}
	if st, _ := m.docker.InspectByName(ctx, m.egressName()); st == nil || !st.Running {
		return
	}
	if err := m.pushPolicy(ctx); err != nil {
		m.log.Warn("plugin egress policy push failed, plugins stay denied", "error", err)
	}
}

func mkdirPrivate(dir string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("prepare egress dir: %w", err)
	}
	if err := os.Chmod(dir, 0o700); err != nil { //nolint:gosec // owner-only dir holds the policy socket
		return fmt.Errorf("secure egress dir: %w", err)
	}
	return nil
}
