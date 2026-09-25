package catalog

var dashboardTemplates = []Template{
	{
		ID:                     "homepage",
		Name:                   "Homepage",
		Slogan:                 "A fast, static, highly customizable start page for all your self-hosted services.",
		Category:               "Dashboard",
		DocumentationURL:       "https://gethomepage.dev/latest/",
		RecommendedMemoryBytes: 134217728, // 128Mi
		// Less certain than the other tags here that this exact patch
		// version is a real published tag for this fast-moving project;
		// the image repository and major line are correct.
		Compose: `services:
  homepage:
    image: ghcr.io/gethomepage/homepage:v0.10.4
    ports: ["3000:3000"]
    volumes:
      - homepage_config:/app/config
    healthcheck:
      test: ["CMD-SHELL", "wget -q -O- http://127.0.0.1:3000/ || exit 1"]
      interval: 30s
      timeout: 5s
      retries: 3
`,
	},
	{
		ID:                     "dashy",
		Name:                   "Dashy",
		Slogan:                 "A feature-rich, self-hosted start page with widgets, status checks, and full visual customization.",
		Category:               "Dashboard",
		DocumentationURL:       "https://dashy.to/docs",
		RecommendedMemoryBytes: 134217728, // 128Mi
		// Tag not verified against a live registry in this environment;
		// the image repository is correct.
		Compose: `services:
  dashy:
    image: lissy93/dashy:3.1.1
    ports: ["8080:8080"]
    volumes:
      - dashy_config:/app/user-data
    healthcheck:
      test: ["CMD-SHELL", "wget -q -O- http://127.0.0.1:8080/ || exit 1"]
      interval: 30s
      timeout: 5s
      retries: 3
`,
	},
	{
		ID:                     "glance",
		Name:                   "Glance",
		Slogan:                 "A fast, self-hosted dashboard that pulls RSS, weather, and other widgets onto one page.",
		Category:               "Dashboard",
		DocumentationURL:       "https://github.com/glanceapp/glance/blob/main/docs/configuration.md",
		RecommendedMemoryBytes: 134217728, // 128Mi
		// Tag not verified against a live registry in this environment;
		// the image repository is correct.
		Compose: `services:
  glance:
    image: glanceapp/glance:v0.8.6
    ports: ["8080:8080"]
    volumes:
      - glance_config:/app/config
    healthcheck:
      test: ["CMD-SHELL", "wget -q -O- http://127.0.0.1:8080/ || exit 1"]
      interval: 30s
      timeout: 5s
      retries: 3
`,
	},
	{
		ID:                     "heimdall",
		Name:                   "Heimdall",
		Slogan:                 "An application dashboard that gathers links to all your self-hosted services on one start page.",
		Category:               "Dashboard",
		DocumentationURL:       "https://github.com/linuxserver/Heimdall",
		RecommendedMemoryBytes: 134217728, // 128Mi
		Compose: `services:
  heimdall:
    image: lscr.io/linuxserver/heimdall:2.6.3
    ports: ["80:80"]
    environment:
      PUID: "1000"
      PGID: "1000"
      TZ: "Etc/UTC"
    volumes:
      - heimdall_config:/config
    healthcheck:
      test: ["CMD-SHELL", "curl -fsS -o /dev/null http://127.0.0.1:80/"]
      interval: 10s
      timeout: 5s
      retries: 3
      start_period: 30s
`,
	},
	{
		ID:                     "homarr",
		Name:                   "Homarr",
		Slogan:                 "A customizable start page dashboard for your self-hosted services with drag-and-drop widgets.",
		Category:               "Dashboard",
		DocumentationURL:       "https://homarr.dev/docs/getting-started/",
		RecommendedMemoryBytes: 268435456, // 256Mi
		Compose: `services:
  homarr:
    image: ghcr.io/ajnart/homarr:0.15.10
    ports: ["7575:7575"]
    environment:
      TZ: "Etc/UTC"
    volumes:
      - homarr_configs:/app/data/configs
      - homarr_icons:/app/public/icons
      - homarr_data:/app/data
    healthcheck:
      test: ["CMD-SHELL", "wget -q -O /dev/null http://127.0.0.1:7575/ || exit 1"]
      interval: 10s
      timeout: 5s
      retries: 3
      start_period: 30s
`,
	},
	{
		ID:                     "homer",
		Name:                   "Homer",
		Slogan:                 "A dead simple static start page for your services, configured with a single YAML file.",
		Category:               "Dashboard",
		DocumentationURL:       "https://github.com/bastienwirtz/homer/blob/main/docs/configuration.md",
		RecommendedMemoryBytes: 134217728, // 128Mi
		Compose: `services:
  homer:
    image: b4bz/homer:v24.05.1
    ports: ["8080:8080"]
    environment:
      INIT_ASSETS: "1"
    volumes:
      - homer_assets:/www/assets
    healthcheck:
      test: ["CMD-SHELL", "wget -q -O /dev/null http://127.0.0.1:8080/ || exit 1"]
      interval: 10s
      timeout: 5s
      retries: 3
      start_period: 15s
`,
	},
	{
		ID:                     "flame",
		Name:                   "Flame",
		Slogan:                 "A self-hosted start page with an app launcher, bookmarks, and Docker label integration.",
		Category:               "Dashboard",
		DocumentationURL:       "https://github.com/pawelmalak/flame#readme",
		RecommendedMemoryBytes: 134217728, // 128Mi
		Compose: `services:
  flame:
    image: pawelmalak/flame:v2.3.1
    ports: ["5005:5005"]
    environment:
      PASSWORD: $SERVICE_PASSWORD_ADMIN
    volumes:
      - flame_data:/app/data
    healthcheck:
      test: ["CMD-SHELL", "wget -q -O /dev/null http://127.0.0.1:5005/ || exit 1"]
      interval: 10s
      timeout: 5s
      retries: 3
      start_period: 15s
`,
	},
}
