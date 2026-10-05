package provision

import (
	"fmt"
	"strings"
)

// agentImageOwner is the registry namespace the agent image is published under, a concrete upstream location rather than branding.
const agentImageOwner = "ghcr.io/glincker/"

// CloudInitParams is everything RenderCloudInit needs to produce a
// first-boot script for a freshly created server.
type CloudInitParams struct {
	ControlPlaneAddr string
	JoinToken        string
	CAFingerprint    string
	NodeName         string
	// AgentName is the brand-derived agent identifier (Brand.AgentName()), used for the unit, env file, data dir and default image.
	AgentName string
	// DisplayName is the product name shown in the unit description (Brand.Name).
	DisplayName string
	// AgentImage overrides the default repo:tag, for an operator who
	// mirrors the image elsewhere. Empty uses the default repo at
	// AgentVersion.
	AgentImage   string
	AgentVersion string
	// MeshEnabled gives the agent container the TUN device and NET_ADMIN
	// its WireGuard device needs.
	MeshEnabled bool
}

func (p CloudInitParams) agentImage() string {
	if p.AgentImage != "" {
		return p.AgentImage
	}
	tag := p.AgentVersion
	// "" (never set) and "dev" (internal/version.Version's own default for
	// an unreleased build) both mean "not a tagged release": the release
	// pipeline never publishes an image tagged "dev", only "edge" for a
	// main-branch build and a version tag for a real release, so either
	// case must fall back to "edge" or the agent's image pull 404s and
	// the node never enrolls.
	if tag == "" || tag == "dev" {
		tag = "edge"
	}
	return agentImageOwner + p.AgentName + ":" + tag
}

// RenderCloudInit renders the user-data cloud-init runs on first boot: it
// installs Docker if missing (install.sh's own get.docker.com convention),
// then runs the agent as a systemd-managed container that joins the
// control plane with a one-time token. The join token and CA fingerprint
// go in a root-only env file, not container run args, so they don't show
// up in `ps`; they remain visible via `docker inspect`, a known limitation
// documented in docs/node-provisioning.md. The agent runs as root
// (--user 0:0, it needs docker.sock and NET_ADMIN), so the bind-mounted
// identity directory is created root-owned with mode 0700.
func RenderCloudInit(p CloudInitParams) (string, error) {
	if p.ControlPlaneAddr == "" || p.JoinToken == "" || p.NodeName == "" || p.AgentName == "" || p.DisplayName == "" {
		return "", fmt.Errorf("provision: render cloud-init: control plane address, join token, node name, agent name and display name are required")
	}
	for _, v := range []string{p.ControlPlaneAddr, p.JoinToken, p.CAFingerprint, p.NodeName, p.AgentName, p.DisplayName} {
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
	fmt.Fprintf(&envLines, "APP_AGENT_IDENTITY_FILE=/var/lib/%s-data/identity.json\n", p.AgentName)
	meshFlags := ""
	if p.MeshEnabled {
		envLines.WriteString("APP_MESH_ENABLED=1\n")
		meshFlags = " -e APP_MESH_ENABLED --cap-add NET_ADMIN --device /dev/net/tun"
	}

	unit := fmt.Sprintf(agentUnit, p.agentImage(), meshFlags, p.AgentName, p.DisplayName)
	return fmt.Sprintf(cloudInitTemplate, indentBlock(envLines.String()), indentBlock(unit), p.AgentName), nil
}

const agentUnit = `[Unit]
Description=%[4]s node agent
After=network-online.target docker.service
Requires=docker.service
Wants=network-online.target

[Service]
EnvironmentFile=/etc/%[3]s.env
ExecStartPre=-/usr/bin/docker rm -f %[3]s
ExecStartPre=/usr/bin/docker pull %[1]s
ExecStart=/usr/bin/docker run --rm --name %[3]s --user 0:0 --network host -v /var/run/docker.sock:/var/run/docker.sock -v /var/lib/%[3]s-data:/var/lib/%[3]s-data -e APP_CONTROL_PLANE_ADDR -e APP_JOIN_TOKEN -e APP_CA_FINGERPRINT -e APP_NODE_NAME -e APP_AGENT_IDENTITY_FILE%[2]s %[1]s
Restart=on-failure
RestartSec=5

[Install]
WantedBy=multi-user.target
`

const cloudInitTemplate = `#cloud-init
write_files:
  - path: /etc/%[3]s.env
    permissions: '0600'
    content: |
%[1]s
  - path: /etc/systemd/system/%[3]s.service
    permissions: '0644'
    content: |
%[2]s
runcmd:
  - command -v docker >/dev/null 2>&1 || (curl -fsSL https://get.docker.com | sh)
  - systemctl enable --now docker
  - mkdir -p /var/lib/%[3]s-data
  - chown root:root /var/lib/%[3]s-data
  - chmod 700 /var/lib/%[3]s-data
  - systemctl daemon-reload
  - systemctl enable --now %[3]s
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
