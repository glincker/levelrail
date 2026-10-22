package agent

import (
	"context"
	"errors"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/agent/agentpb"
	"github.com/GLINCKER/levelrail/internal/cloudmeta"
	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/nodepreflight"
)

type pingRuntime struct {
	docker.Runtime
	err error
	ver string
}

func (p pingRuntime) Ping(context.Context) error                    { return p.err }
func (p pingRuntime) ServerVersion(context.Context) (string, error) { return p.ver, nil }

func offlineCloud() cloudmeta.Config {
	d := "http://127.0.0.1:1"
	return cloudmeta.Config{Bases: cloudmeta.Bases{AWS: d, GCP: d, Azure: d, Hetzner: d, DigitalOcean: d}, Timeout: 200 * time.Millisecond}
}

func TestOnboardingPBRoundTrip(t *testing.T) {
	at := time.Unix(1700000000, 123)
	in := OnboardingReport{
		Cloud:    cloudmeta.Info{Provider: "aws", Region: "eu-west-1", Zone: "eu-west-1a", InstanceID: "i-1", Source: cloudmeta.SourceMetadata},
		Disabled: true,
		Facts: nodepreflight.Facts{
			OS: "linux", Arch: "arm64", DockerOK: true, DockerVersion: "27", WireGuardKernel: true,
			MeshEnabled: true, MeshPort: 51820, MeshPortBindable: true, DiskTotalBytes: 10, DiskFreeBytes: 4,
			AgentTime: at, Provider: "aws",
		},
	}
	out := onboardingFromPB(onboardingToPB(in))
	if out.Cloud != in.Cloud || out.Facts != in.Facts || out.Disabled != in.Disabled {
		t.Fatalf("round trip lost data:\n got %+v\nwant %+v", out, in)
	}
}

func TestOnboardingProbeReportsDockerDown(t *testing.T) {
	probe := NewOnboardingProbe(pingRuntime{err: errors.New("daemon not running")}, t.TempDir(), true, offlineCloud())
	r := probe(context.Background())
	if r.Facts.DockerOK || r.Facts.DockerError != "daemon not running" {
		t.Errorf("docker facts = %+v", r.Facts)
	}
	if r.Cloud.Found() {
		t.Errorf("an offline host must report no cloud, got %+v", r.Cloud)
	}
	if r.Facts.DiskTotalBytes == 0 || r.Facts.OS == "" || r.Facts.Arch == "" || r.Facts.AgentTime.IsZero() {
		t.Errorf("host facts missing: %+v", r.Facts)
	}
}

func TestOnboardingProbeVersionAndOverride(t *testing.T) {
	cfg := offlineCloud()
	cfg.Override = cloudmeta.Info{Provider: "home", Zone: "rack-1"}
	probe := NewOnboardingProbe(pingRuntime{ver: "27.3.1"}, t.TempDir(), false, cfg)
	r := probe(context.Background())
	if !r.Facts.DockerOK || r.Facts.DockerVersion != "27.3.1" {
		t.Errorf("docker facts = %+v", r.Facts)
	}
	if r.Cloud.Provider != "home" || r.Cloud.Zone != "rack-1" || r.Cloud.Source != cloudmeta.SourceEnv {
		t.Errorf("cloud = %+v", r.Cloud)
	}
}

func TestUDPPortFreeSeesAHeldPort(t *testing.T) {
	pc, err := net.ListenPacket("udp", ":0")
	if err != nil {
		t.Skip("cannot bind udp here")
	}
	defer func() { _ = pc.Close() }()
	port := pc.LocalAddr().(*net.UDPAddr).Port
	if udpPortFree(port) {
		t.Error("udpPortFree reported a held port as free")
	}
}

func TestOnboardingLoopSendsImmediatelyThenOnTick(t *testing.T) {
	var mu sync.Mutex
	var got []*agentpb.OnboardingReport
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	probe := func(context.Context) OnboardingReport {
		return OnboardingReport{Facts: nodepreflight.Facts{OS: "linux"}}
	}
	go func() {
		defer close(done)
		onboardingLoop(ctx, probe, 20*time.Millisecond, func(m *agentpb.AgentMessage) error {
			mu.Lock()
			defer mu.Unlock()
			got = append(got, m.GetOnboarding())
			return nil
		})
	}()
	deadline := time.After(2 * time.Second)
	for {
		mu.Lock()
		n := len(got)
		mu.Unlock()
		if n >= 3 {
			break
		}
		select {
		case <-deadline:
			t.Fatalf("only %d reports", n)
		case <-time.After(5 * time.Millisecond):
		}
	}
	cancel()
	<-done
	if got[0].GetOs() != "linux" {
		t.Errorf("first report = %+v", got[0])
	}
}

func TestOnboardingLoopStopsWhenSendFails(t *testing.T) {
	done := make(chan struct{})
	go func() {
		defer close(done)
		onboardingLoop(context.Background(), func(context.Context) OnboardingReport { return OnboardingReport{} }, time.Hour,
			func(*agentpb.AgentMessage) error { return errors.New("stream closed") })
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("loop kept running after the stream closed")
	}
}

func TestOnboardingStoreKeepsLatest(t *testing.T) {
	st := NewOnboardingStore()
	now := time.Now()
	st.SetNodeOnboarding("n1", OnboardingReport{Facts: nodepreflight.Facts{Arch: "amd64"}}, now)
	st.SetNodeOnboarding("n1", OnboardingReport{Facts: nodepreflight.Facts{Arch: "arm64"}}, now.Add(time.Second))
	got, ok := st.Get("n1")
	if !ok || got.Report.Facts.Arch != "arm64" || len(st.Snapshot()) != 1 {
		t.Fatalf("got %+v ok=%v", got, ok)
	}
	if _, ok := st.Get("missing"); ok {
		t.Error("unknown node returned a report")
	}
	if got.Stale(now.Add(time.Minute)) || !got.Stale(now.Add(time.Hour)) {
		t.Error("staleness window is wrong")
	}
}

func TestOnboardingIntervalFromEnv(t *testing.T) {
	look := func(v string) func(string) (string, bool) {
		return func(string) (string, bool) { return v, v != "" }
	}
	if OnboardingIntervalFromEnv(look("")) != defaultOnboardingInterval ||
		OnboardingIntervalFromEnv(look("bogus")) != defaultOnboardingInterval ||
		OnboardingIntervalFromEnv(look("15s")) != 15*time.Second {
		t.Error("interval parsing is wrong")
	}
}
