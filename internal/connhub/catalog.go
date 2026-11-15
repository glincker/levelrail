package connhub

// Connect methods a catalogue entry can offer.
const (
	ConnectOneClick     = "one_click"
	ConnectGuidedOAuth  = "guided_oauth"
	ConnectToken        = "token"
	ConnectSettingsPage = "settings_page"
)

// CatalogEntry names a provider the user can connect and the existing endpoint that creates it.
// The hub never creates connections itself, so CreateEndpoint is empty when no plain create route exists.
type CatalogEntry struct {
	ID             string   `json:"id"`
	Category       string   `json:"category"`
	Name           string   `json:"name"`
	ConnectMethods []string `json:"connect_methods"`
	CreateEndpoint string   `json:"create_endpoint,omitempty"`
}

func entries(category, endpoint string, methods []string, pairs ...string) []CatalogEntry {
	out := make([]CatalogEntry, 0, len(pairs)/2)
	for i := 0; i+1 < len(pairs); i += 2 {
		out = append(out, CatalogEntry{ID: pairs[i], Category: category, Name: pairs[i+1], ConnectMethods: methods, CreateEndpoint: endpoint})
	}
	return out
}

// Catalog returns the static provider catalogue. Providers of a category share one create endpoint.
func Catalog() []CatalogEntry {
	var out []CatalogEntry
	out = append(out, entries(KindGit, "/api/v1/github-app/register/start", []string{ConnectOneClick}, "github", "GitHub")...)
	out = append(out, entries(KindGit, "/api/v1/git-connections", []string{ConnectGuidedOAuth},
		"gitlab", "GitLab", "bitbucket", "Bitbucket Cloud", "gitea", "Gitea", "forgejo", "Forgejo")...)
	out = append(out, entries(KindGit, "/api/v1/git-connections", []string{ConnectToken},
		"bitbucket_server", "Bitbucket Server", "azure_devops", "Azure DevOps")...)
	out = append(out, entries(KindRegistry, "/api/v1/registry-credentials", []string{ConnectToken},
		"dockerhub", "Docker Hub", "ghcr", "GitHub Container Registry", "gitlab", "GitLab Registry", "ecr", "Amazon ECR",
		"artifact_registry", "Google Artifact Registry", "acr", "Azure Container Registry", "harbor", "Harbor", "quay", "Quay",
		"jfrog", "JFrog", "nexus", "Nexus", "generic", "Generic v2 registry")...)
	out = append(out, entries(KindNotify, "/api/v1/notification-channels", []string{ConnectToken},
		"slack", "Slack", "discord", "Discord", "telegram", "Telegram", "email", "Email", "pushover", "Pushover", "pagerduty", "PagerDuty",
		"teams", "Microsoft Teams", "resend", "Resend", "ntfy", "ntfy", "gotify", "Gotify", "mattermost", "Mattermost", "lark", "Lark",
		"rocketchat", "Rocket.Chat", "opsgenie", "Opsgenie", "webex", "Webex", "googlechat", "Google Chat", "webpush", "Web push", "generic", "Generic webhook")...)
	out = append(out, entries(KindSecret, "", []string{ConnectSettingsPage},
		"vault", "Vault", "doppler", "Doppler", "infisical", "Infisical", "onepassword", "1Password")...)
	out = append(out, entries(KindDNS, "", []string{ConnectSettingsPage},
		"cloudflare", "Cloudflare DNS", "route53", "Route 53", "google", "Google Cloud DNS")...)
	out = append(out, entries(KindCloud, "/api/v1/settings/cloud-connections", []string{ConnectToken},
		"oracle", "Oracle Cloud", "aws", "AWS", "azure", "Azure", "gcp", "Google Cloud", "hetzner", "Hetzner", "digitalocean", "DigitalOcean")...)
	out = append(out, entries(KindWebhook, "/api/v1/event-webhooks", []string{ConnectToken}, "event", "Event webhook")...)
	out = append(out, entries(KindWebhook, "/api/v1/status-page/webhooks", []string{ConnectToken}, "status", "Status page webhook")...)
	return out
}
