package api

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestDoctorCheckAgentAdvertiseHost_NotConfigured(t *testing.T) {
	rt, _ := newDoctorTestRouter(t)
	c := rt.doctorCheckAgentAdvertiseHost(context.Background())
	if c.Status != doctorStatusUnknown {
		t.Errorf("status = %q, want %q", c.Status, doctorStatusUnknown)
	}
}

func TestDoctorCheckAgentAdvertiseHost_LoopbackNoNodes(t *testing.T) {
	rt, _ := newDoctorTestRouter(t)
	rt.doctorAgentAdvertiseHost = "127.0.0.1"
	rt.doctorAgentAdvertisePort = 9443

	c := rt.doctorCheckAgentAdvertiseHost(context.Background())
	if c.Status != doctorStatusOK {
		t.Errorf("status = %q, want %q (loopback default, no nodes enrolled yet)", c.Status, doctorStatusOK)
	}
}

func TestDoctorCheckAgentAdvertiseHost_LoopbackWithNodesFails(t *testing.T) {
	rt, db := newDoctorTestRouter(t)
	rt.doctorAgentAdvertiseHost = "127.0.0.1"
	rt.doctorAgentAdvertisePort = 9443

	if err := db.SaveNode(context.Background(), store.Node{ID: "node-1", Name: "worker-1", Address: "10.0.0.5", Status: store.NodeStatusPending}); err != nil {
		t.Fatalf("SaveNode() error = %v", err)
	}

	c := rt.doctorCheckAgentAdvertiseHost(context.Background())
	if c.Status != doctorStatusFail {
		t.Errorf("status = %q, want %q (loopback default with a real node enrolled)", c.Status, doctorStatusFail)
	}
	if c.Fix == "" {
		t.Error("Fix = \"\", want a concrete next step")
	}
}

func TestDoctorCheckAgentAdvertiseHost_RealHostDialed(t *testing.T) {
	rt, _ := newDoctorTestRouter(t)
	rt.doctorAgentAdvertiseHost = "agent.example.com"
	rt.doctorAgentAdvertisePort = 9443

	pipeClient, pipeServer := net.Pipe()
	t.Cleanup(func() { _ = pipeServer.Close() })

	tests := []struct {
		name       string
		dial       doctorDialContextFunc
		wantStatus string
	}{
		{
			name:       "reachable",
			dial:       func(context.Context, string, string) (net.Conn, error) { return pipeClient, nil },
			wantStatus: doctorStatusOK,
		},
		{
			name:       "unreachable",
			dial:       func(context.Context, string, string) (net.Conn, error) { return nil, errors.New("connection refused") },
			wantStatus: doctorStatusWarn,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rt.doctorDialContext = tt.dial
			c := rt.doctorCheckAgentAdvertiseHost(context.Background())
			if c.Status != tt.wantStatus {
				t.Errorf("status = %q, want %q (message %q)", c.Status, tt.wantStatus, c.Message)
			}
		})
	}
}

func TestDoctorIsLoopbackHost(t *testing.T) {
	tests := []struct {
		host string
		want bool
	}{
		{"127.0.0.1", true},
		{"localhost", true},
		{"LocalHost", true},
		{"::1", true},
		{"10.0.0.5", false},
		{"agent.example.com", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := doctorIsLoopbackHost(tt.host); got != tt.want {
			t.Errorf("doctorIsLoopbackHost(%q) = %v, want %v", tt.host, got, tt.want)
		}
	}
}

func TestDoctorCheckDiskIOLatency_NoDataDir(t *testing.T) {
	rt, _ := newDoctorTestRouter(t)
	c := rt.doctorCheckDiskIOLatency()
	if c.Status != doctorStatusUnknown {
		t.Errorf("status = %q, want %q", c.Status, doctorStatusUnknown)
	}
}

func TestDoctorCheckDiskIOLatency_FastWriteOK(t *testing.T) {
	rt, _ := newDoctorTestRouter(t)
	rt.dataDir = t.TempDir()

	c := rt.doctorCheckDiskIOLatency()
	if c.Status != doctorStatusOK {
		t.Errorf("status = %q, want %q (message %q)", c.Status, doctorStatusOK, c.Message)
	}
}

func TestDoctorCheckDiskIOLatency_WarnsBelowThreshold(t *testing.T) {
	rt, _ := newDoctorTestRouter(t)
	rt.dataDir = t.TempDir()
	rt.doctorDiskIOWarnLatency = time.Nanosecond // impossible to beat, forces warn

	c := rt.doctorCheckDiskIOLatency()
	if c.Status != doctorStatusWarn {
		t.Errorf("status = %q, want %q", c.Status, doctorStatusWarn)
	}
	if c.Fix == "" {
		t.Error("Fix = \"\", want a concrete next step")
	}
}

func TestDoctorRegistryHosts_FallsBackToDockerHub(t *testing.T) {
	rt, _ := newDoctorTestRouter(t)
	hosts := rt.doctorRegistryHosts(context.Background())
	if len(hosts) != 1 || hosts[0] != defaultDoctorRegistryHost {
		t.Errorf("hosts = %v, want [%q]", hosts, defaultDoctorRegistryHost)
	}
}

func TestDoctorRegistryHosts_CollectsCredentialsAndBuiltIn(t *testing.T) {
	rt, db := newDoctorTestRouter(t)

	if err := db.SaveRegistryCredential(context.Background(), store.RegistryCredential{
		ID: "cred-1", Name: "ghcr", RegistryHost: "ghcr.io", Username: "octo", CreatedAt: time.Now().UTC().Format(time.RFC3339),
	}); err != nil {
		t.Fatalf("SaveRegistryCredential() error = %v", err)
	}
	if err := db.UpdateRegistrySettings(context.Background(), store.RegistrySettings{
		Enabled: true, Host: "registry.internal.example.com", Username: "levelrail", CreatedAt: time.Now().UTC().Format(time.RFC3339),
	}); err != nil {
		t.Fatalf("UpdateRegistrySettings() error = %v", err)
	}

	hosts := rt.doctorRegistryHosts(context.Background())
	want := map[string]bool{"ghcr.io": true, "registry.internal.example.com": true}
	if len(hosts) != len(want) {
		t.Fatalf("hosts = %v, want exactly %v", hosts, want)
	}
	for _, h := range hosts {
		if !want[h] {
			t.Errorf("unexpected host %q in %v", h, hosts)
		}
	}
}

func TestDoctorCheckOneRegistry(t *testing.T) {
	rt, _ := newDoctorTestRouter(t)

	pipeClient, pipeServer := net.Pipe()
	t.Cleanup(func() { _ = pipeServer.Close() })
	rt.doctorDialContext = func(context.Context, string, string) (net.Conn, error) { return pipeClient, nil }
	if c := rt.doctorCheckOneRegistry(context.Background(), "ghcr.io"); c.Status != doctorStatusOK {
		t.Errorf("status = %q, want %q", c.Status, doctorStatusOK)
	}

	rt.doctorDialContext = fakeDoctorOfflineDialContext
	c := rt.doctorCheckOneRegistry(context.Background(), "ghcr.io")
	if c.Status != doctorStatusWarn {
		t.Errorf("status = %q, want %q", c.Status, doctorStatusWarn)
	}
	if c.Code != "registry_reachability_ghcr.io" {
		t.Errorf("code = %q, want %q", c.Code, "registry_reachability_ghcr.io")
	}
}
