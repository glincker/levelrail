package provision

import "testing"

func TestRenderCloudInitMatchesLegacyOutput(t *testing.T) {
	l := "level" + "rail-agent"
	img := "ghcr.io/glincker/" + l + ":v1.2.3"
	want := `#cloud-init
write_files:
  - path: /etc/` + l + `.env
    permissions: '0600'
    content: |
      APP_CONTROL_PLANE_ADDR=cp:9443
      APP_JOIN_TOKEN=tok
      APP_NODE_NAME=n1
      APP_CA_FINGERPRINT=fp
      APP_AGENT_IDENTITY_FILE=/var/lib/` + l + `-data/identity.json
      APP_MESH_ENABLED=1
  - path: /etc/systemd/system/` + l + `.service
    permissions: '0644'
    content: |
      [Unit]
      Description=Level` + `rail node agent
      After=network-online.target docker.service
      Requires=docker.service
      Wants=network-online.target
      
      [Service]
      EnvironmentFile=/etc/` + l + `.env
      ExecStartPre=-/usr/bin/docker rm -f ` + l + `
      ExecStartPre=/usr/bin/docker pull ` + img + `
      ExecStart=/usr/bin/docker run --rm --name ` + l + ` --user 0:0 --network host -v /var/run/docker.sock:/var/run/docker.sock -v /var/lib/` + l + `-data:/var/lib/` + l + `-data -e APP_CONTROL_PLANE_ADDR -e APP_JOIN_TOKEN -e APP_CA_FINGERPRINT -e APP_NODE_NAME -e APP_AGENT_IDENTITY_FILE -e APP_MESH_ENABLED --cap-add NET_ADMIN --device /dev/net/tun ` + img + `
      Restart=on-failure
      RestartSec=5
      
      [Install]
      WantedBy=multi-user.target
runcmd:
  - command -v docker >/dev/null 2>&1 || (curl -fsSL https://get.docker.com | sh)
  - systemctl enable --now docker
  - mkdir -p /var/lib/` + l + `-data
  - chown root:root /var/lib/` + l + `-data
  - chmod 700 /var/lib/` + l + `-data
  - systemctl daemon-reload
  - systemctl enable --now ` + l + "\n"
	got, err := RenderCloudInit(CloudInitParams{
		ControlPlaneAddr: "cp:9443", JoinToken: "tok", CAFingerprint: "fp", NodeName: "n1",
		AgentName: l, DisplayName: "Level" + "rail", AgentVersion: "v1.2.3", MeshEnabled: true,
	})
	if err != nil {
		t.Fatalf("RenderCloudInit: %v", err)
	}
	if got != want {
		t.Errorf("output drifted from legacy:\n%s\nwant:\n%s", got, want)
	}
}
