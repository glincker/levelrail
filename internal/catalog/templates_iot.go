package catalog

var iotTemplates = []Template{
	{
		ID:                     "home-assistant",
		Name:                   "Home Assistant",
		Slogan:                 "Open-source home automation that puts local control and privacy first.",
		Category:               "IoT",
		DocumentationURL:       "https://www.home-assistant.io/docs/",
		RecommendedMemoryBytes: 536870912, // 512Mi
		// Home Assistant usually runs on host networking to discover
		// local devices; this template runs it on the platform's normal
		// bridge networking instead, so device auto-discovery won't
		// work out of the box, only the web UI and manually configured
		// integrations.
		Compose: `services:
  home-assistant:
    image: ghcr.io/home-assistant/home-assistant:2025.10.2
    ports: ["8123:8123"]
    volumes:
      - home_assistant_config:/config
    healthcheck:
      test: ["CMD-SHELL", "wget -q -O- http://127.0.0.1:8123/ || exit 1"]
      interval: 30s
      timeout: 5s
      retries: 3
`,
	},
}
