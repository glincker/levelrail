package agent

import (
	"context"
	"net"
	"runtime"
	"strconv"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/internal/agent/agentpb"
	"github.com/GLINCKER/levelrail/internal/cloudmeta"
	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/network"
	"github.com/GLINCKER/levelrail/internal/nodepreflight"
	"github.com/GLINCKER/levelrail/internal/telemetry"
)

// EnvOnboardingInterval is how often the agent re-sends its onboarding
// facts, so a fix applied on the node shows up without a restart.
const EnvOnboardingInterval = "APP_AGENT_ONBOARDING_INTERVAL"

const (
	defaultOnboardingInterval = time.Minute
	onboardingMaxAge          = 5 * time.Minute
	dockerProbeTimeout        = 5 * time.Second
)

// OnboardingReport is one node's detected cloud placement and the raw facts
// the control plane judges into preflight checks.
type OnboardingReport struct {
	Cloud    cloudmeta.Info
	Facts    nodepreflight.Facts
	Disabled bool
}

// OnboardingProbe gathers the node's current onboarding report.
type OnboardingProbe func(ctx context.Context) OnboardingReport

// OnboardingIntervalFromEnv reads EnvOnboardingInterval, or the default.
func OnboardingIntervalFromEnv(lookup func(string) (string, bool)) time.Duration {
	if v, ok := lookup(EnvOnboardingInterval); ok {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			return d
		}
	}
	return defaultOnboardingInterval
}

// WithOnboardingProbe makes RunSession report the node's onboarding facts
// every interval.
func WithOnboardingProbe(p OnboardingProbe, interval time.Duration) SessionOption {
	return func(c *sessionConfig) { c.onboardingProbe, c.onboardingEvery = p, interval }
}

// NewOnboardingProbe measures this host. Cloud detection is retried on every
// report until it finds something, then cached for the process lifetime.
func NewOnboardingProbe(rt docker.Runtime, diskPath string, meshEnabled bool, cloud cloudmeta.Config) OnboardingProbe {
	var (
		mu     sync.Mutex
		cached cloudmeta.Info
	)
	return func(ctx context.Context) OnboardingReport {
		mu.Lock()
		info := cached
		mu.Unlock()
		if !info.Found() {
			info = cloudmeta.Detect(ctx, cloud)
			mu.Lock()
			cached = info
			mu.Unlock()
		}
		f := nodepreflight.Facts{
			OS: runtime.GOOS, Arch: runtime.GOARCH, AgentTime: time.Now(),
			Provider: info.Provider, MeshEnabled: meshEnabled, MeshPort: network.DefaultListenPort,
			WireGuardKernel: network.SystemProbe{}.KernelModuleLoaded(),
		}
		probeDocker(ctx, rt, &f)
		if !meshEnabled {
			f.MeshPortBindable = udpPortFree(network.DefaultListenPort)
		}
		if total, used, err := telemetry.HostDiskBytes(diskPath); err == nil && total > 0 {
			f.DiskTotalBytes = uint64(total)
			f.DiskFreeBytes = uint64(max(total-used, 0))
		}
		return OnboardingReport{Cloud: info, Facts: f, Disabled: cloud.Disabled}
	}
}

func probeDocker(ctx context.Context, rt docker.Runtime, f *nodepreflight.Facts) {
	ctx, cancel := context.WithTimeout(ctx, dockerProbeTimeout)
	defer cancel()
	pinger, ok := rt.(interface{ Ping(context.Context) error })
	if !ok {
		f.DockerOK = true
		return
	}
	if err := pinger.Ping(ctx); err != nil {
		f.DockerError = err.Error()
		return
	}
	f.DockerOK = true
	if v, ok := rt.(interface {
		ServerVersion(context.Context) (string, error)
	}); ok {
		f.DockerVersion, _ = v.ServerVersion(ctx)
	}
}

func udpPortFree(port int) bool {
	var lc net.ListenConfig
	pc, err := lc.ListenPacket(context.Background(), "udp", ":"+strconv.Itoa(port))
	if err != nil {
		return false
	}
	_ = pc.Close()
	return true
}

func onboardingLoop(ctx context.Context, probe OnboardingProbe, interval time.Duration, send func(*agentpb.AgentMessage) error) {
	if interval <= 0 {
		interval = defaultOnboardingInterval
	}
	report := func() bool {
		return send(&agentpb.AgentMessage{Payload: &agentpb.AgentMessage_Onboarding{Onboarding: onboardingToPB(probe(ctx))}}) == nil
	}
	if !report() {
		return
	}
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if !report() {
				return
			}
		}
	}
}

// lockedStream serializes Send for sessions without a renewer, which would
// otherwise be the only thing sharing the stream with serveSession.
type lockedStream struct {
	inner agentClientStream
	mu    sync.Mutex
}

func (s *lockedStream) Send(m *agentpb.AgentMessage) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.inner.Send(m)
}

func (s *lockedStream) Recv() (*agentpb.ControlMessage, error) { return s.inner.Recv() }

func onboardingToPB(r OnboardingReport) *agentpb.OnboardingReport {
	f := r.Facts
	return &agentpb.OnboardingReport{
		CloudProvider: r.Cloud.Provider, CloudRegion: r.Cloud.Region, CloudZone: r.Cloud.Zone,
		CloudInstanceId: r.Cloud.InstanceID, CloudSource: r.Cloud.Source,
		Os: f.OS, Arch: f.Arch,
		DockerOk: f.DockerOK, DockerError: f.DockerError, DockerVersion: f.DockerVersion,
		WireguardKernel: f.WireGuardKernel, MeshEnabled: f.MeshEnabled,
		MeshPort:         uint32(max(f.MeshPort, 0)), //nolint:gosec // port range
		MeshPortBindable: f.MeshPortBindable,
		DiskTotalBytes:   f.DiskTotalBytes, DiskFreeBytes: f.DiskFreeBytes,
		AgentUnixNano:    f.AgentTime.UnixNano(),
		MetadataDisabled: r.Disabled,
	}
}

func onboardingFromPB(p *agentpb.OnboardingReport) OnboardingReport {
	var at time.Time
	if n := p.GetAgentUnixNano(); n != 0 {
		at = time.Unix(0, n)
	}
	cloud := cloudmeta.Info{
		Provider: p.GetCloudProvider(), Region: p.GetCloudRegion(), Zone: p.GetCloudZone(),
		InstanceID: p.GetCloudInstanceId(), Source: p.GetCloudSource(),
	}
	return OnboardingReport{
		Cloud: cloud, Disabled: p.GetMetadataDisabled(),
		Facts: nodepreflight.Facts{
			OS: p.GetOs(), Arch: p.GetArch(),
			DockerOK: p.GetDockerOk(), DockerError: p.GetDockerError(), DockerVersion: p.GetDockerVersion(),
			WireGuardKernel: p.GetWireguardKernel(), MeshEnabled: p.GetMeshEnabled(),
			MeshPort: int(p.GetMeshPort()), MeshPortBindable: p.GetMeshPortBindable(),
			DiskTotalBytes: p.GetDiskTotalBytes(), DiskFreeBytes: p.GetDiskFreeBytes(),
			AgentTime: at, Provider: cloud.Provider,
		},
	}
}

// StoredOnboarding is a node's latest report with the control plane's receipt time.
type StoredOnboarding struct {
	Report     OnboardingReport
	ReceivedAt time.Time
}

// OnboardingSink receives a node's onboarding report on the control plane.
type OnboardingSink interface {
	SetNodeOnboarding(nodeID string, r OnboardingReport, receivedAt time.Time)
}

// WithOnboardingSink records the onboarding reports agents send up their
// Session stream.
func WithOnboardingSink(sink OnboardingSink) Option {
	return func(s *Server) { s.onboardingSink = sink }
}

// OnboardingStore keeps each node's latest report in memory: it is a live
// gauge and a restart simply waits for the next agent tick.
type OnboardingStore struct {
	mu      sync.Mutex
	reports map[string]StoredOnboarding
}

// NewOnboardingStore returns an empty store.
func NewOnboardingStore() *OnboardingStore {
	return &OnboardingStore{reports: make(map[string]StoredOnboarding)}
}

// SetNodeOnboarding implements OnboardingSink.
func (s *OnboardingStore) SetNodeOnboarding(nodeID string, r OnboardingReport, receivedAt time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.reports[nodeID] = StoredOnboarding{Report: r, ReceivedAt: receivedAt}
}

// Snapshot returns a copy of every node's latest report.
func (s *OnboardingStore) Snapshot() map[string]StoredOnboarding {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make(map[string]StoredOnboarding, len(s.reports))
	for k, v := range s.reports {
		out[k] = v
	}
	return out
}

// Get returns nodeID's latest report.
func (s *OnboardingStore) Get(nodeID string) (StoredOnboarding, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	st, ok := s.reports[nodeID]
	return st, ok
}

// Stale reports whether the report is old enough that the agent has probably
// stopped sending, so its facts should not be trusted as current.
func (st StoredOnboarding) Stale(now time.Time) bool {
	return now.Sub(st.ReceivedAt) > onboardingMaxAge
}
