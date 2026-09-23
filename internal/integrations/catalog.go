// Package integrations holds the curated catalog of third-party tools
// an app can attach (internal/catalog's deployable-template catalog is
// unrelated: this is per-app add-ons, not services to deploy). v1 scope
// is env-var injection only, no webhook or API wiring, per the approved
// design in docs-local/specs/2026-09-23-framework-native-deploys-design.md
// item 6.
package integrations

// Field types are descriptive only, driving which input affordance the
// UI shows for a catalog entry's env var; nothing here validates a
// value against its type.
const (
	FieldTypeAPIKey    = "api_key"
	FieldTypeDSN       = "dsn"
	FieldTypeToken     = "token"
	FieldTypeProjectID = "project_id"
	FieldTypeSite      = "site"
	FieldTypeHost      = "host"
)

// EnvVar is one environment variable a catalog Integration injects into
// an attached app's container. Name is the exact variable name the
// vendor's own SDK reads, verified against that vendor's current docs
// (DocsURL on the owning Integration), never guessed.
type EnvVar struct {
	Name     string
	Type     string
	Required bool
	// Default is injected when Required is false and no value was
	// supplied at attach time, e.g. PostHog's host defaulting to its US
	// cloud region.
	Default     string
	Placeholder string
}

// Integration is one curated tool an app can attach.
type Integration struct {
	Key         string
	Name        string
	Description string
	DocsURL     string
	EnvVars     []EnvVar
	// Frameworks names the frameworks this integration is most relevant
	// for, matching internal/build's detected framework identifiers once
	// that detection work lands (docs-local spec item 2). Empty means
	// relevant regardless of framework: this catalog must degrade
	// gracefully with no framework detected at all, not depend on it.
	Frameworks []string
}

// Catalog is every integration an app can attach, source of GET
// /api/v1/integrations. Env var names below are verified against each
// vendor's own current setup docs (DocsURL), not guessed.
var Catalog = []Integration{
	{
		Key:         "sentry",
		Name:        "Sentry",
		Description: "Error tracking and performance monitoring.",
		DocsURL:     "https://docs.sentry.io/platforms/node/configuration/options/",
		EnvVars: []EnvVar{
			{Name: "SENTRY_DSN", Type: FieldTypeDSN, Required: true, Placeholder: "https://<key>@o0.ingest.sentry.io/0"},
		},
	},
	{
		Key:         "posthog",
		Name:        "PostHog",
		Description: "Product analytics, feature flags, and session replay.",
		DocsURL:     "https://posthog.com/docs/libraries/next-js",
		EnvVars: []EnvVar{
			{Name: "NEXT_PUBLIC_POSTHOG_KEY", Type: FieldTypeAPIKey, Required: true, Placeholder: "phc_..."},
			{Name: "NEXT_PUBLIC_POSTHOG_HOST", Type: FieldTypeHost, Required: false, Default: "https://us.i.posthog.com", Placeholder: "https://us.i.posthog.com"},
		},
		Frameworks: []string{"nextjs"},
	},
	{
		Key:         "datadog",
		Name:        "Datadog",
		Description: "APM tracing, infrastructure, and log monitoring.",
		DocsURL:     "https://docs.datadoghq.com/tracing/trace_collection/library_config/nodejs/",
		EnvVars: []EnvVar{
			{Name: "DD_API_KEY", Type: FieldTypeAPIKey, Required: true},
			{Name: "DD_SITE", Type: FieldTypeSite, Required: false, Default: "datadoghq.com", Placeholder: "datadoghq.com"},
		},
	},
	{
		Key:         "axiom",
		Name:        "Axiom",
		Description: "Log and event data platform.",
		DocsURL:     "https://axiom.co/docs/send-data/nextjs",
		EnvVars: []EnvVar{
			{Name: "AXIOM_TOKEN", Type: FieldTypeToken, Required: true},
			{Name: "AXIOM_DATASET", Type: FieldTypeProjectID, Required: true, Placeholder: "my-dataset"},
		},
		Frameworks: []string{"nextjs"},
	},
	{
		Key:         "betterstack",
		Name:        "Better Stack",
		Description: "Log management and uptime monitoring.",
		DocsURL:     "https://betterstack.com/docs/logs/javascript/install/",
		EnvVars: []EnvVar{
			{Name: "LOGTAIL_SOURCE_TOKEN", Type: FieldTypeToken, Required: true},
		},
	},
	{
		Key:         "logsnag",
		Name:        "LogSnag",
		Description: "Real-time event notifications and analytics.",
		DocsURL:     "https://docs.logsnag.com/quick-start",
		EnvVars: []EnvVar{
			{Name: "LOGSNAG_TOKEN", Type: FieldTypeToken, Required: true},
			{Name: "LOGSNAG_PROJECT", Type: FieldTypeProjectID, Required: true, Placeholder: "my-project"},
		},
	},
	{
		Key:         "bugsnag",
		Name:        "BugSnag",
		Description: "Error monitoring and stability management.",
		DocsURL:     "https://docs.bugsnag.com/platforms/javascript/",
		EnvVars: []EnvVar{
			{Name: "BUGSNAG_API_KEY", Type: FieldTypeAPIKey, Required: true},
		},
	},
	{
		Key:         "newrelic",
		Name:        "New Relic",
		Description: "Full-stack observability and APM.",
		DocsURL:     "https://docs.newrelic.com/docs/apm/agents/nodejs-agent/installation-configuration/install-nodejs-agent/",
		EnvVars: []EnvVar{
			{Name: "NEW_RELIC_LICENSE_KEY", Type: FieldTypeAPIKey, Required: true},
		},
	},
}

// Get returns the catalog entry for key, or ok=false if key doesn't
// match any entry (e.g. removed from the catalog after an app already
// attached it).
func Get(key string) (Integration, bool) {
	for _, i := range Catalog {
		if i.Key == key {
			return i, true
		}
	}
	return Integration{}, false
}
