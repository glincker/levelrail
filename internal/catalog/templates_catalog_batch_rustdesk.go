package catalog

// catalogRustDeskTemplates holds the self-hosted remote desktop server. It
// uses the project's s6 image so hbbs and hbbr run in one container with a shell.
var catalogRustDeskTemplates = []Template{
	{
		ID:                     "rustdesk-server",
		Name:                   "RustDesk Server",
		Slogan:                 "Self-hosted ID and relay servers for RustDesk remote desktop clients (AGPL-3.0).",
		Category:               "Infrastructure",
		DocumentationURL:       "https://rustdesk.com/docs/en/self-host/rustdesk-server-oss/docker/",
		RecommendedMemoryBytes: 134217728, // 128Mi
		Compose: `# Ports are for RustDesk clients on your network, not the HTTP proxy.
# The platform binds published ports privately by default: open the app's
# Network settings and set the public bind for each published port, and allow
# TCP 21115-21117 and UDP 21116 in the host or cloud firewall.
# Set RUSTDESK_RELAY to your public host:21117 so clients can reach the relay.
# After first start, read id_ed25519.pub in the data volume and give that key
# to every client (ID/Relay server settings, Key field). AGPL-3.0.
services:
  rustdesk:
    image: rustdesk/rustdesk-server-s6:1.1.16
    ports:
      - "21115:21115"
      - "21116:21116"
      - "21116:21116/udp"
      - "21117:21117"
    environment:
      RELAY: ${RUSTDESK_RELAY:-relay.example.com:21117}
      ENCRYPTED_ONLY: "1"
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
