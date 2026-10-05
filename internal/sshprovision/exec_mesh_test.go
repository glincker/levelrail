package sshprovision

import (
	"strings"
	"testing"
)

func TestRenderAgentEnvFile_MeshToggle(t *testing.T) {
	tests := []struct {
		name string
		mesh bool
		want bool
	}{{"off", false, false}, {"on", true, true}}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			env := renderAgentEnvFile(InstallParams{ControlPlaneAddr: "cp:9443", JoinToken: "t", NodeName: "n", MeshEnabled: tc.mesh})
			if got := strings.Contains(env, "APP_MESH_ENABLED=1"); got != tc.want {
				t.Errorf("env mesh line = %v, want %v", got, tc.want)
			}
			flags := meshRunFlags(tc.mesh)
			if got := strings.Contains(flags, "--cap-add NET_ADMIN") && strings.Contains(flags, "/dev/net/tun"); got != tc.want {
				t.Errorf("run flags = %q, want mesh=%v", flags, tc.want)
			}
		})
	}
}

func TestAgentUnitAndDataDir(t *testing.T) {
	tests := []struct {
		name string
		got  string
		want string
	}{
		{"runs as root", agentUnitTemplate, "--user 0:0"},
		{"identity env", agentUnitTemplate, "-e APP_AGENT_IDENTITY_FILE"},
		{"data dir exists", agentDataDirCmd, "mkdir -p /var/lib/levelrail-agent-data"},
		{"data dir root owned", agentDataDirCmd, "chown root:root /var/lib/levelrail-agent-data"},
		{"data dir private", agentDataDirCmd, "chmod 700 /var/lib/levelrail-agent-data"},
		{"mesh needs tun", meshRunFlags(true), "--device /dev/net/tun"},
		{"mesh needs net admin", meshRunFlags(true), "--cap-add NET_ADMIN"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !strings.Contains(tt.got, tt.want) {
				t.Errorf("%q missing %q", tt.got, tt.want)
			}
		})
	}
}
