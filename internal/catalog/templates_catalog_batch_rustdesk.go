package catalog

// catalogRustDeskTemplates holds the self-hosted remote desktop server. It
// uses the project's s6 image so hbbs and hbbr run in one container with a shell.
var catalogRustDeskTemplates = []Template{
	{
		ID:                     "rustdesk-server",
		Name:                   "RustDesk Server",
		Slogan:                 "Self-hosted RustDesk ID and relay servers (AGPL-3.0). Set RUSTDESK_RELAY to host:21117, set a public bind per port in Network settings, open TCP 21115-21117 and UDP 21116, read id_ed25519.pub in the data volume.",
		Category:               "Infrastructure",
		DocumentationURL:       "https://rustdesk.com/docs/en/self-host/rustdesk-server-oss/docker/",
		RecommendedMemoryBytes: 134217728, // 128Mi
		Compose: `# Ports are for RustDesk clients on your network, not the HTTP proxy.
# The platform binds published ports privately by default: open the app's
# Network settings and set the public bind for each published port, and allow
# TCP 21115-21117 and UDP 21116 in the host or cloud firewall.
# RUSTDESK_RELAY (your public host:21117) is required: the container exits with a log line if it is empty.
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
