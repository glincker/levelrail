package functions

import (
	"embed"
	"fmt"
	"io/fs"
	"path"
	"sort"
)

//go:embed all:starters
var starterFS embed.FS

// EnvVar is one environment variable a starter reads.
type EnvVar struct {
	Name        string `json:"name"`
	Required    bool   `json:"required"`
	Description string `json:"description"`
}

// Starter is a complete, tested handler for one of the idea presets.
type Starter struct {
	ID          string   `json:"id"`
	Preset      string   `json:"preset"`
	Name        string   `json:"name"`
	Runtime     string   `json:"runtime"`
	UsesKV      bool     `json:"uses_kv"`
	Description string   `json:"description"`
	Env         []EnvVar `json:"env"`
	Files       []File   `json:"files"`
}

var starterMeta = []Starter{
	{
		ID: "url-shortener", Preset: "shortener", Name: "URL shortener", Runtime: "node", UsesKV: true,
		Description: "Short links stored in the key-value store.",
		Env: []EnvVar{
			{"ADMIN_TOKEN", true, "Bearer token needed to create links."},
			{"MAX_TTL_SECONDS", false, "Cap on link lifetime."},
		},
	},
	{
		ID: "webhook-receiver", Preset: "webhook", Name: "Webhook receiver", Runtime: "node", UsesKV: true,
		Description: "Verifies an HMAC signature and appends each event to the key-value store.",
		Env: []EnvVar{
			{"WEBHOOK_SECRET", true, "Shared secret the sender signs with."},
			{"ADMIN_TOKEN", false, "Bearer token for GET /events."},
			{"EVENT_TTL_SECONDS", false, "How long events are kept, default 7 days."},
		},
	},
	{
		ID: "form-to-email", Preset: "form", Name: "Form to email", Runtime: "node", UsesKV: true,
		Description: "Stores contact form submissions and can notify a webhook. The platform has no send API, so it does not send mail itself.",
		Env: []EnvVar{
			{"ADMIN_TOKEN", false, "Bearer token for GET /submissions."},
			{"NOTIFY_WEBHOOK_URL", false, "Receives a one-line notice per submission."},
			{"ALLOWED_ORIGIN", false, "CORS origin of your site."},
		},
	},
	{
		ID: "scheduled-job-runner", Preset: "scheduled", Name: "Scheduled job runner", Runtime: "node", UsesKV: true,
		Description: "Named jobs called by an http cron job, with run history and an overlap lock.",
		Env: []EnvVar{
			{"CRON_SECRET", true, "Secret path segment: /cron/<secret>/<job>."},
			{"HEARTBEAT_URL", false, "Address the heartbeat job pings."},
		},
	},
	{
		ID: "chat-bot", Preset: "chatbot", Name: "Chat bot", Runtime: "node", UsesKV: true,
		Description: "Relays a webhook to Slack, Discord or Telegram.",
		Env: []EnvVar{
			{"WEBHOOK_TOKEN", true, "Value callers send in X-Webhook-Token."},
			{"SLACK_WEBHOOK_URL", false, "Slack incoming webhook."},
			{"DISCORD_WEBHOOK_URL", false, "Discord channel webhook."},
			{"TELEGRAM_BOT_TOKEN", false, "Telegram bot token, with TELEGRAM_CHAT_ID."},
		},
	},
}

// Starters lists the idea-preset starters with their files, in a stable order.
func Starters() []Starter {
	out := make([]Starter, 0, len(starterMeta))
	for _, m := range starterMeta {
		s := m
		files, err := readStarterFiles(m.ID)
		if err != nil {
			continue
		}
		s.Files = files
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// LookupStarter returns the starter with the given id.
func LookupStarter(id string) (Starter, bool) {
	for _, s := range Starters() {
		if s.ID == id {
			return s, true
		}
	}
	return Starter{}, false
}

func readStarterFiles(id string) ([]File, error) {
	dir := path.Join("starters", id)
	entries, err := fs.ReadDir(starterFS, dir)
	if err != nil {
		return nil, fmt.Errorf("functions: read starter %q: %w", id, err)
	}
	files := make([]File, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		b, err := fs.ReadFile(starterFS, path.Join(dir, e.Name()))
		if err != nil {
			return nil, fmt.Errorf("functions: read starter file %q/%q: %w", id, e.Name(), err)
		}
		files = append(files, File{Path: e.Name(), Content: string(b)})
	}
	return files, nil
}
