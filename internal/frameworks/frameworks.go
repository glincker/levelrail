// Package frameworks is the catalog of first-class framework presets: the
// port, health path, build mode, env hints and volumes a framework needs to
// deploy with no manual build config. Data only, no I/O besides Detect.
package frameworks

import "sort"

// Output modes describe what the build produces.
const (
	OutputServer     = "server"
	OutputStandalone = "standalone"
	OutputStatic     = "static"
	OutputJar        = "jar"
	OutputBinary     = "binary"
)

// Build types mirror spec.Build* values without importing the spec package.
const (
	BuildRailpack   = "railpack"
	BuildStatic     = "static"
	BuildDockerfile = "dockerfile"
)

// EnvHint is an environment variable the framework commonly needs.
type EnvHint struct {
	Key         string `json:"key"`
	Description string `json:"description"`
	Required    bool   `json:"required"`
	Secret      bool   `json:"secret"`
	Default     string `json:"default,omitempty"`
}

// VolumeHint is a path worth persisting across deploys.
type VolumeHint struct {
	Name    string `json:"name"`
	Path    string `json:"path"`
	Purpose string `json:"purpose"`
}

// Preset is one framework's recommended deploy defaults.
type Preset struct {
	ID           string       `json:"id"`
	Name         string       `json:"name"`
	Runtime      string       `json:"runtime"`
	Icon         string       `json:"icon"`
	BuildType    string       `json:"build_type"`
	OutputMode   string       `json:"output_mode"`
	Port         int          `json:"port"`
	HealthPath   string       `json:"health_path"`
	BuildCommand string       `json:"build_command,omitempty"`
	PublishDir   string       `json:"publish_dir,omitempty"`
	StartCommand string       `json:"start_command,omitempty"`
	ReadyTimeout string       `json:"ready_timeout,omitempty"`
	Dockerfile   bool         `json:"dockerfile_template"`
	EnvHints     []EnvHint    `json:"env_hints"`
	Volumes      []VolumeHint `json:"volumes"`
	DocsAnchor   string       `json:"docs_anchor"`
}

var presets = []Preset{
	{
		ID: "nextjs", Name: "Next.js", Runtime: "node", Icon: "nextjs", BuildType: BuildRailpack, OutputMode: OutputStandalone,
		Port: 3000, HealthPath: "/", BuildCommand: "next build", StartCommand: "node server.js", Dockerfile: true, DocsAnchor: "nextjs",
		EnvHints: []EnvHint{
			{Key: "HOSTNAME", Description: "Bind address for the standalone server; must be 0.0.0.0 to be reachable.", Default: "0.0.0.0"},
			{Key: "NEXT_TELEMETRY_DISABLED", Description: "Turn off Next.js build telemetry.", Default: "1"},
			{Key: "NEXT_PUBLIC_SITE_URL", Description: "Public URL baked in at build time; set it as a build arg."},
		},
		Volumes: []VolumeHint{{Name: "next-cache", Path: "/app/.next/cache", Purpose: "ISR pages and the image optimization cache survive redeploys"}},
	},
	{
		ID: "nextjs-static", Name: "Next.js (static export)", Runtime: "node", Icon: "nextjs", BuildType: BuildStatic, OutputMode: OutputStatic,
		HealthPath: "/", BuildCommand: "next build", PublishDir: "out", DocsAnchor: "nextjs-static-export",
		EnvHints: []EnvHint{{Key: "NEXT_PUBLIC_API_URL", Description: "Public values are inlined at build time."}},
	},
	{
		ID: "vite", Name: "React / Vite", Runtime: "node", Icon: "vite", BuildType: BuildStatic, OutputMode: OutputStatic,
		HealthPath: "/", BuildCommand: "vite build", PublishDir: "dist", DocsAnchor: "react-vite",
		EnvHints: []EnvHint{{Key: "VITE_API_URL", Description: "Variables prefixed VITE_ are inlined at build time."}},
	},
	{
		ID: "astro", Name: "Astro", Runtime: "node", Icon: "astro", BuildType: BuildStatic, OutputMode: OutputStatic,
		HealthPath: "/", BuildCommand: "astro build", PublishDir: "dist", DocsAnchor: "astro",
		EnvHints: []EnvHint{{Key: "PUBLIC_SITE_URL", Description: "Variables prefixed PUBLIC_ are inlined at build time."}},
	},
	{
		ID: "docusaurus", Name: "Docusaurus", Runtime: "node", Icon: "docusaurus", BuildType: BuildStatic, OutputMode: OutputStatic,
		HealthPath: "/", BuildCommand: "docusaurus build", PublishDir: "build", DocsAnchor: "docusaurus",
	},
	{
		ID: "nuxt", Name: "Nuxt", Runtime: "node", Icon: "nuxt", BuildType: BuildRailpack, OutputMode: OutputServer,
		Port: 3000, HealthPath: "/", BuildCommand: "nuxt build", StartCommand: "node .output/server/index.mjs", DocsAnchor: "nuxt",
		EnvHints: []EnvHint{
			{Key: "NITRO_HOST", Description: "Bind address for the Nitro server.", Default: "0.0.0.0"},
			{Key: "NUXT_PUBLIC_API_BASE", Description: "Runtime public config override."},
		},
	},
	{
		ID: "remix", Name: "Remix", Runtime: "node", Icon: "remix", BuildType: BuildRailpack, OutputMode: OutputServer,
		Port: 3000, HealthPath: "/", BuildCommand: "remix vite:build", StartCommand: "remix-serve ./build/server/index.js", DocsAnchor: "remix",
		EnvHints: []EnvHint{
			{Key: "PORT", Description: "remix-serve listens on this port; keep it equal to the app port.", Default: "3000"},
			{Key: "SESSION_SECRET", Description: "Signs session cookies; generate a long random value.", Required: true},
		},
	},
	{
		ID: "sveltekit", Name: "SvelteKit", Runtime: "node", Icon: "svelte", BuildType: BuildRailpack, OutputMode: OutputServer,
		Port: 3000, HealthPath: "/", BuildCommand: "vite build", StartCommand: "node build", DocsAnchor: "sveltekit",
		EnvHints: []EnvHint{
			{Key: "HOST", Description: "Bind address for adapter-node.", Default: "0.0.0.0"},
			{Key: "ORIGIN", Description: "Public origin; form actions fail CSRF checks without it.", Required: true},
		},
	},
	{
		ID: "node", Name: "Node.js (Express)", Runtime: "node", Icon: "node", BuildType: BuildRailpack, OutputMode: OutputServer,
		Port: 3000, HealthPath: "/", StartCommand: "npm start", DocsAnchor: "nodejs-express",
		EnvHints: []EnvHint{
			{Key: "NODE_ENV", Description: "Enables production code paths in Express and most libraries.", Default: "production"},
			{Key: "PORT", Description: "Listen on this port; keep it equal to the app port.", Default: "3000"},
		},
	},
	{
		ID: "go", Name: "Go", Runtime: "go", Icon: "go", BuildType: BuildRailpack, OutputMode: OutputBinary,
		Port: 8080, HealthPath: "/healthz", BuildCommand: "go build -o app .", StartCommand: "./app", Dockerfile: true, DocsAnchor: "go",
		EnvHints: []EnvHint{{Key: "PORT", Description: "Read the port from the environment.", Default: "8080"}},
	},
	{
		ID: "spring-boot", Name: "Spring Boot", Runtime: "java", Icon: "spring", BuildType: BuildRailpack, OutputMode: OutputJar,
		Port: 8080, HealthPath: "/actuator/health", ReadyTimeout: "120s", Dockerfile: true, DocsAnchor: "spring-boot",
		StartCommand: "java -jar app.jar",
		EnvHints: []EnvHint{
			{Key: "SPRING_PROFILES_ACTIVE", Description: "Active profile, for example prod.", Default: "prod"},
			{Key: "JAVA_TOOL_OPTIONS", Description: "Size the heap from the container limit.", Default: "-XX:MaxRAMPercentage=75"},
			{Key: "MANAGEMENT_ENDPOINT_HEALTH_PROBES_ENABLED", Description: "Expose liveness and readiness groups on the actuator.", Default: "true"},
		},
	},
	{
		ID: "django", Name: "Django", Runtime: "python", Icon: "django", BuildType: BuildRailpack, OutputMode: OutputServer,
		Port: 8000, HealthPath: "/", StartCommand: "gunicorn config.wsgi", DocsAnchor: "django",
		EnvHints: []EnvHint{
			{Key: "DJANGO_SECRET_KEY", Description: "Signing key; read it in settings.py.", Required: true, Secret: true},
			{Key: "DJANGO_ALLOWED_HOSTS", Description: "Comma separated host names; a 400 means this is missing the app domain."},
			{Key: "DJANGO_SETTINGS_MODULE", Description: "Settings module for production."},
		},
		Volumes: []VolumeHint{{Name: "media", Path: "/app/media", Purpose: "User uploaded files (MEDIA_ROOT)"}},
	},
	{
		ID: "fastapi", Name: "FastAPI", Runtime: "python", Icon: "fastapi", BuildType: BuildRailpack, OutputMode: OutputServer,
		Port: 8000, HealthPath: "/docs", StartCommand: "uvicorn main:app --host 0.0.0.0 --port 8000", DocsAnchor: "fastapi",
		EnvHints: []EnvHint{{Key: "WEB_CONCURRENCY", Description: "Uvicorn worker count.", Default: "2"}},
	},
	{
		ID: "rails", Name: "Ruby on Rails", Runtime: "ruby", Icon: "ruby", BuildType: BuildRailpack, OutputMode: OutputServer,
		Port: 3000, HealthPath: "/up", StartCommand: "bin/rails server -b 0.0.0.0", DocsAnchor: "rails",
		EnvHints: []EnvHint{
			{Key: "SECRET_KEY_BASE", Description: "Rails refuses to boot in production without it.", Required: true, Secret: true},
			{Key: "RAILS_ENV", Description: "Rails environment.", Default: "production"},
			{Key: "RAILS_SERVE_STATIC_FILES", Description: "Serve compiled assets from the app.", Default: "true"},
			{Key: "RAILS_LOG_TO_STDOUT", Description: "Send logs to the log viewer.", Default: "true"},
		},
		Volumes: []VolumeHint{{Name: "storage", Path: "/app/storage", Purpose: "Active Storage disk service files"}},
	},
	{
		ID: "laravel", Name: "Laravel", Runtime: "php", Icon: "laravel", BuildType: BuildRailpack, OutputMode: OutputServer,
		Port: 80, HealthPath: "/up", DocsAnchor: "laravel",
		EnvHints: []EnvHint{
			{Key: "APP_KEY", Description: "Generate with php artisan key:generate --show.", Required: true, Secret: true},
			{Key: "APP_ENV", Description: "Application environment.", Default: "production"},
			{Key: "APP_DEBUG", Description: "Never true in production.", Default: "false"},
			{Key: "APP_URL", Description: "Public URL used when generating links."},
		},
		Volumes: []VolumeHint{{Name: "storage", Path: "/app/storage", Purpose: "Uploads, sessions and logs written by the app"}},
	},
}

// List returns every preset sorted by name. The slice is a copy.
func List() []Preset {
	out := make([]Preset, len(presets))
	copy(out, presets)
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// Get returns the preset with the given id.
func Get(id string) (Preset, bool) {
	for _, p := range presets {
		if p.ID == id {
			return p, true
		}
	}
	return Preset{}, false
}

// DefaultStaticBuildCommand runs the project's own build script, which every
// static preset ships, so the package manager and any wrapper stay the author's.
const DefaultStaticBuildCommand = "npm run build"

// StaticBuild returns the build command and publish directory that prefill a
// static build for a preset, or false when the preset is not a static site
// with a known output directory.
func (p Preset) StaticBuild() (command, publish string, ok bool) {
	if p.BuildType != BuildStatic || p.PublishDir == "" {
		return "", "", false
	}
	return DefaultStaticBuildCommand, p.PublishDir, true
}
