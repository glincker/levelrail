package provision

import (
	"fmt"
	"strings"
)

// defaultAgentImageRepo is the published, cosign-signed image
// (.github/workflows/release.yml's merge-image-levelrail-agent job) the
// agent ships as. There is no raw agent binary release asset to check a
// sha256sum against the way install.sh verifies the control plane binary,
// so cloud-init pulls this image instead. Matches internal/api/updates.go's
// own githubRepo constant: a concrete upstream location, not user-facing
// branding, so the project's brand-indirection rule does not apply here.
const defaultAgentImageRepo = "ghcr.io/glincker/levelrail-agent"

// CloudInitParams is everything RenderCloudInit needs to produce a
// first-boot script for a freshly created server.
type CloudInitParams struct {
	ControlPlaneAddr string
	JoinToken        string
	CAFingerprint    string
	NodeName         string
	// AgentImage overrides defaultAgentImageRepo:tag, for an operator who
	// mirrors the image elsewhere. Empty uses the default repo at
	// AgentVersion.
	AgentImage   string
	AgentVersion string
}

func (p CloudInitParams) agentImage() string {
	if p.AgentImage != "" {
		return p.AgentImage
	}
	tag := p.AgentVersion
	if tag == "" {
		tag = "latest"
	}
	return defaultAgentImageRepo + ":" + tag
}

// RenderCloudInit renders the user-data cloud-init runs on first boot: it
// installs Docker if missing (install.sh's own get.docker.com convention),
// then runs the agent as a systemd-managed container that joins the
// control plane with a one-time token. The join token and CA fingerprint
// go in a root-only env file, not container run args, so they don't show
// up in `ps`; they remain visible via `docker inspect`, a known limitation
// documented in docs/node-provisioning.md.
func RenderCloudInit(p CloudInitParams) (string, error) {
	if p.ControlPlaneAddr == "" || p.JoinToken == "" || p.NodeName == "" {
		return "", fmt.Errorf("provision: render cloud-init: control plane address, join token and node name are required")
	}
	for _, v := range []string{p.ControlPlaneAddr, p.JoinToken, p.CAFingerprint, p.NodeName} {
		if strings.ContainsAny(v, "\n\r") {
			return "", fmt.Errorf("provision: render cloud-init: value contains a newline")
		}
	}

	var envLines strings.Builder
	fmt.Fprintf(&envLines, "APP_CONTROL_PLANE_ADDR=%s\n", p.ControlPlaneAddr)
	fmt.Fprintf(&envLines, "APP_JOIN_TOKEN=%s\n", p.JoinToken)
	fmt.Fprintf(&envLines, "APP_NODE_NAME=%s\n", p.NodeName)
	if p.CAFingerprint != "" {
		fmt.Fprintf(&envLines, "APP_CA_FINGERPRINT=%s\n", p.CAFingerprint)
	}
	fmt.Fprintf(&envLines, "APP_AGENT_IDENTITY_FILE=/var/lib/levelrail-agent-data/identity.json\n")

	unit := fmt.Sprintf(agentUnit, p.agentImage())
	return fmt.Sprintf(cloudInitTemplate, indentBlock(envLines.String()), indentBlock(unit)), nil
}

const agentUnit = `[Unit]
Description=Levelrail node agent
After=network-online.target docker.service
Requires=docker.service
Wants=network-online.target

[Service]
EnvironmentFile=/etc/levelrail-agent.env
ExecStartPre=-/usr/bin/docker rm -f levelrail-agent
ExecStartPre=/usr/bin/docker pull %[1]s
ExecStart=/usr/bin/docker run --rm --name levelrail-agent --network host -v /var/run/docker.sock:/var/run/docker.sock -v /var/lib/levelrail-agent-data:/var/lib/levelrail-agent-data -e APP_CONTROL_PLANE_ADDR -e APP_JOIN_TOKEN -e APP_CA_FINGERPRINT -e APP_NODE_NAME -e APP_AGENT_IDENTITY_FILE %[1]s
Restart=on-failure
RestartSec=5

[Install]
WantedBy=multi-user.target
`

const cloudInitTemplate = `#cloud-init
write_files:
  - path: /etc/levelrail-agent.env
    permissions: '0600'
    content: |
%s
  - path: /etc/systemd/system/levelrail-agent.service
    permissions: '0644'
    content: |
%s
runcmd:
  - command -v docker >/dev/null 2>&1 || (curl -fsSL https://get.docker.com | sh)
  - systemctl enable --now docker
  - mkdir -p /var/lib/levelrail-agent-data
  - systemctl daemon-reload
  - systemctl enable --now levelrail-agent
`

// indentBlock indents every line of s by 6 spaces, matching
// cloudInitTemplate's own "content: |" block indentation.
func indentBlock(s string) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	for i, l := range lines {
		lines[i] = "      " + l
	}
	return strings.Join(lines, "\n")
}
