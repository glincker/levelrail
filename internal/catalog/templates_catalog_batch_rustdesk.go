package catalog

// catalogRustDeskTemplates holds the self-hosted remote desktop server. It
// uses the project's s6 image so hbbs and hbbr run in one container with a shell.
var catalogRustDeskTemplates = []Template{
	{
		ID:     "rustdesk-server",
		Name:   "RustDesk Server",
		Slogan: "Self-hosted RustDesk ID and relay servers (AGPL-3.0).",
		Description: "Set RUSTDESK_RELAY to your public host:21117, or the container refuses to start.\n" +
			"Published ports bind privately by default: set a public bind for each one in the app's Network settings.\n" +
			"Open TCP 21115-21117 and UDP 21116 in the host or cloud firewall.\n" +
			"After first start, read id_ed25519.pub in the data volume and give it to every client as the Key.\n" +
			"Licence: AGPL-3.0.",
		Category:               "Infrastructure",
		DocumentationURL:       "https://rustdesk.com/docs/en/self-host/rustdesk-server-oss/docker/",
		RecommendedMemoryBytes: 134217728, // 128Mi
		Compose: `# Ports serve RustDesk clients directly, not the HTTP proxy. Setup notes are in the description.
services:
  rustdesk:
    image: rustdesk/rustdesk-server-s6:1.1.16
    ports:
      - "21115:21115"
      - "21116:21116"
      - "21116:21116/udp"
      - "21117:21117"
    environment:
      RELAY: ${RUSTDESK_RELAY:-}
      ENCRYPTED_ONLY: "1"
    command: ["/command/with-contenv", "sh", "-c", "[ -n \"$RELAY\" ] || { echo 'RUSTDESK_RELAY is empty: set it to your public host:21117' >&2; exit 1; }; exec sleep infinity"]
    volumes:
      - rustdesk_data:/data
    healthcheck:
      test: ["CMD-SHELL", "nc -z 127.0.0.1 21116 && nc -z 127.0.0.1 21117 && test -s /data/id_ed25519.pub"]
      interval: 15s
      timeout: 5s
      retries: 5
      start_period: 20s
`,
	},
}
