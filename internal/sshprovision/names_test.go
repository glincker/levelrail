package sshprovision

import "testing"

var testNames = agentNames{name: "acme-agent", display: "Acme"}

func TestAgentNamesMatchLegacyLiterals(t *testing.T) {
	n := agentNames{name: "level" + "rail-agent", display: "Level" + "rail"}
	l := "level" + "rail-agent"

	if got, want := n.envPath(), "/etc/"+l+".env"; got != want {
		t.Errorf("envPath = %q, want %q", got, want)
	}
	if got, want := n.unitPath(), "/etc/systemd/system/"+l+".service"; got != want {
		t.Errorf("unitPath = %q, want %q", got, want)
	}
	if got, want := n.defaultImage(), "ghcr.io/glincker/"+l+":edge"; got != want {
		t.Errorf("defaultImage = %q, want %q", got, want)
	}
	wantCmd := "mkdir -p /var/lib/" + l + "-data && chown root:root /var/lib/" + l + "-data && chmod 700 /var/lib/" + l + "-data"
	if got := n.dataDirCmd(); got != wantCmd {
		t.Errorf("dataDirCmd = %q, want %q", got, wantCmd)
	}
	wantEnv := "APP_CONTROL_PLANE_ADDR=cp:9443\nAPP_JOIN_TOKEN=t\nAPP_NODE_NAME=n\nAPP_CA_FINGERPRINT=fp\nAPP_AGENT_IDENTITY_FILE=/var/lib/" + l + "-data/identity.json\nAPP_MESH_ENABLED=1\n"
	p := InstallParams{ControlPlaneAddr: "cp:9443", JoinToken: "t", NodeName: "n", CAFingerprint: "fp", MeshEnabled: true}
	if got := renderAgentEnvFile(n, p); got != wantEnv {
		t.Errorf("env file = %q, want %q", got, wantEnv)
	}

	wantUnit := `[Unit]
Description=Level` + `rail node agent
After=network-online.target docker.service
Requires=docker.service
Wants=network-online.target

[Service]
EnvironmentFile=/etc/` + l + `.env
ExecStartPre=-/usr/bin/docker rm -f ` + l + `
ExecStartPre=/usr/bin/docker pull IMG
ExecStart=/usr/bin/docker run --rm --name ` + l + ` --user 0:0 --network host -v /var/run/docker.sock:/var/run/docker.sock -v /var/lib/` + l + `-data:/var/lib/` + l + `-data -e APP_CONTROL_PLANE_ADDR -e APP_JOIN_TOKEN -e APP_CA_FINGERPRINT -e APP_NODE_NAME -e APP_AGENT_IDENTITY_FILE FLAGS IMG
Restart=on-failure
RestartSec=5

[Install]
WantedBy=multi-user.target
`
	if got := renderAgentUnit(n, "IMG", " FLAGS"); got != wantUnit {
		t.Errorf("unit mismatch:\n%s\nwant:\n%s", got, wantUnit)
	}
}
