package provision

import (
	"strings"
	"testing"
)

func TestRenderCloudInit(t *testing.T) {
	out, err := RenderCloudInit(CloudInitParams{
		ControlPlaneAddr: "cp.example.com:9443",
		JoinToken:        "njt-secret",
		CAFingerprint:    "abc123",
		NodeName:         "web-1",
		AgentVersion:     "v1.2.3",
	})
	if err != nil {
		t.Fatalf("RenderCloudInit: %v", err)
	}
	for _, want := range []string{
		"#cloud-init",
		"APP_CONTROL_PLANE_ADDR=cp.example.com:9443",
		"APP_JOIN_TOKEN=njt-secret",
		"APP_CA_FINGERPRINT=abc123",
		"APP_NODE_NAME=web-1",
		"ghcr.io/glincker/levelrail-agent:v1.2.3",
		"levelrail-agent.service",
		"chown 65532:65532 /var/lib/levelrail-agent-data",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("rendered cloud-init missing %q", want)
		}
	}
}

func TestRenderCloudInit_FallsBackToEdgeWithNoVersion(t *testing.T) {
	out, err := RenderCloudInit(CloudInitParams{
		ControlPlaneAddr: "cp.example.com:9443",
		JoinToken:        "tok",
		NodeName:         "web-1",
	})
	if err != nil {
		t.Fatalf("RenderCloudInit: %v", err)
	}
	if !strings.Contains(out, "ghcr.io/glincker/levelrail-agent:edge") {
		t.Errorf("expected the edge tag, got:\n%s", out)
	}
}

func TestRenderCloudInit_FallsBackToEdgeForDevBuilds(t *testing.T) {
	out, err := RenderCloudInit(CloudInitParams{
		ControlPlaneAddr: "cp.example.com:9443",
		JoinToken:        "tok",
		NodeName:         "web-1",
		AgentVersion:     "dev",
	})
	if err != nil {
		t.Fatalf("RenderCloudInit: %v", err)
	}
	if !strings.Contains(out, "ghcr.io/glincker/levelrail-agent:edge") {
		t.Errorf("expected the edge tag for a dev build, got:\n%s", out)
	}
}

func TestRenderCloudInit_CustomAgentImage(t *testing.T) {
	out, err := RenderCloudInit(CloudInitParams{
		ControlPlaneAddr: "cp.example.com:9443",
		JoinToken:        "tok",
		NodeName:         "web-1",
		AgentImage:       "registry.internal/mirror/levelrail-agent:v9",
	})
	if err != nil {
		t.Fatalf("RenderCloudInit: %v", err)
	}
	if !strings.Contains(out, "registry.internal/mirror/levelrail-agent:v9") {
		t.Errorf("expected the custom image, got:\n%s", out)
	}
}

func TestRenderCloudInit_RejectsMissingFields(t *testing.T) {
	cases := []CloudInitParams{
		{JoinToken: "tok", NodeName: "n"},
		{ControlPlaneAddr: "a", NodeName: "n"},
		{ControlPlaneAddr: "a", JoinToken: "tok"},
	}
	for i, p := range cases {
		if _, err := RenderCloudInit(p); err == nil {
			t.Errorf("case %d: expected an error for %+v", i, p)
		}
	}
}

func TestRenderCloudInit_RejectsNewlineInjection(t *testing.T) {
	_, err := RenderCloudInit(CloudInitParams{
		ControlPlaneAddr: "cp.example.com:9443",
		JoinToken:        "tok\nmalicious: true",
		NodeName:         "n",
	})
	if err == nil {
		t.Fatal("expected an error for a newline in a field")
	}
}
