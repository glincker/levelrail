package catalog

var infrastructureTemplates = []Template{
	{
		ID:                     "portainer",
		Name:                   "Portainer",
		Slogan:                 "A web UI for managing containers, images, volumes, and networks.",
		Category:               "Infrastructure",
		DocumentationURL:       "https://docs.portainer.io",
		RecommendedMemoryBytes: 268435456, // 256Mi
		// Portainer's usual setup bind-mounts the host Docker socket to
		// manage other containers; bind mounts of ordinary host
		// directories are now supported in general (internal/compose's
		// own doc comment on volumes:), but Docker-socket access
		// specifically stays unsupported by design, since it's a full
		// container-escape-to-host-root vector via the Docker API, a
		// categorically different and unreviewed capability that needs
		// its own explicit design decision later, not bundled into
		// general bind-mount support.
		Compose: `services:
  portainer:
    image: portainer/portainer-ce:2.21.0
    ports: ["9443:9443"]
    volumes:
      - portainer_data:/data
    healthcheck:
      test: ["CMD-SHELL", "curl -fsSk -o /dev/null https://127.0.0.1:9443/api/system/status"]
      interval: 10s
      timeout: 5s
      retries: 3
`,
	},
	{
		ID:                     "rabbitmq",
		Name:                   "RabbitMQ",
		Slogan:                 "A widely used open-source message broker supporting AMQP, MQTT, and STOMP.",
		Category:               "Infrastructure",
		DocumentationURL:       "https://www.rabbitmq.com/documentation.html",
		RecommendedMemoryBytes: 536870912, // 512Mi
		// Upstream only publishes a floating "3-management" major tag for
		// this variant; this pinned version couldn't be verified against
		// a live registry in this environment.
		Compose: `services:
  rabbitmq:
    image: rabbitmq:3.13-management-alpine
    ports: ["15672:15672"]
    environment:
      RABBITMQ_DEFAULT_USER: $SERVICE_USER_ADMIN
      RABBITMQ_DEFAULT_PASS: $SERVICE_PASSWORD_ADMIN
    volumes:
      - rabbitmq_data:/var/lib/rabbitmq
    healthcheck:
      test: ["CMD-SHELL", "curl -fsS -o /dev/null http://127.0.0.1:15672/"]
      interval: 10s
      timeout: 5s
      retries: 3
      start_period: 60s
`,
	},
	{
		ID:                     "pi-hole",
		Name:                   "Pi-hole",
		Slogan:                 "Network-wide ad blocking that works as a DNS sinkhole for your whole network.",
		Category:               "Infrastructure",
		DocumentationURL:       "https://docs.pi-hole.net",
		RecommendedMemoryBytes: 268435456, // 256Mi
		// Real Pi-hole setups also expose DNS on 53/tcp+udp; this
		// platform tracks a single container port per service, so only
		// the web admin UI is reachable here, not DNS resolution. Only
		// published under a rolling :latest tag upstream; this pinned
		// version couldn't be verified against a live registry in this
		// environment.
		Compose: `services:
  pihole:
    image: pihole/pihole:2024.07.0
    ports: ["80:80"]
    environment:
      FTLCONF_webserver_api_password: $SERVICE_PASSWORD_ADMIN
    volumes:
      - pihole_data:/etc/pihole
    healthcheck:
      test: ["CMD-SHELL", "curl -fsS -o /dev/null http://127.0.0.1:80/admin/"]
      interval: 10s
      timeout: 5s
      retries: 3
      start_period: 60s
`,
	},
	{
		ID:                     "adguard-home",
		Name:                   "AdGuard Home",
		Slogan:                 "Network-wide ad and tracker blocking with a DNS server and a friendly admin dashboard.",
		Category:               "Infrastructure",
		DocumentationURL:       "https://github.com/AdguardTeam/AdGuardHome/wiki/Docker",
		RecommendedMemoryBytes: 134217728, // 128Mi
		Compose: `services:
  adguard:
    image: adguard/adguardhome:v0.107.55
    ports: ["3000:3000"]
    volumes:
      - adguard_work:/opt/adguardhome/work
      - adguard_conf:/opt/adguardhome/conf
    healthcheck:
      test: ["CMD-SHELL", "wget -q -O- http://127.0.0.1:3000/ || exit 1"]
      interval: 10s
      timeout: 5s
      retries: 3
      start_period: 15s
`,
	},
	{
		ID:                     "nginx-proxy-manager",
		Name:                   "Nginx Proxy Manager",
		Slogan:                 "A web UI for managing Nginx reverse-proxy hosts, redirects, and Let's Encrypt certificates.",
		Category:               "Infrastructure",
		DocumentationURL:       "https://nginxproxymanager.com/guide/",
		RecommendedMemoryBytes: 268435456, // 256Mi
		Compose: `services:
  npm:
    image: jc21/nginx-proxy-manager:2.12.3
    ports: ["81:81"]
    environment:
      DB_SQLITE_FILE: /data/database.sqlite
    volumes:
      - npm_data:/data
      - npm_letsencrypt:/etc/letsencrypt
    healthcheck:
      test: ["CMD-SHELL", "curl -fsS -o /dev/null http://127.0.0.1:81/api/"]
      interval: 15s
      timeout: 5s
      retries: 3
      start_period: 60s
`,
	},
}
