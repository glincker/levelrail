package main

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"github.com/GLINCKER/levelrail/internal/platformimport"
)

// webhookOutcome is what --create-webhooks did, or would do, for one git-sourced app.
// It never carries the webhook secret: the control plane generates and keeps it.
type webhookOutcome struct {
	State    string `json:"state"`
	Host     string `json:"host,omitempty"`
	Provider string `json:"provider,omitempty"`
	Detail   string `json:"detail"`
}

const (
	webhookWouldCreate = "would_create"
	webhookCreated     = "created"
	webhookAlready     = "already_registered"
	webhookSkipped     = "skipped"
	webhookFailed      = "failed"

	webhookRegisteredState = "registered"
	providerGitHubApp      = "github"

	envMigrateWebhookLimit     = "APP_MIGRATE_WEBHOOK_LIMIT"
	defaultMigrateWebhookLimit = 200
	envMigrateRepoURLMaxLen    = "APP_MIGRATE_REPO_URL_MAX_LEN"
	defaultMigrateRepoURLMax   = 2048
	webhookDetailMaxRunes      = 300
)

const createWebhooksHelp = "with --apply, register one push webhook per git-sourced app through the connected git provider for its host (off by default because it writes to the git host); without --apply the report lists what would be created"

var plainHostRe = regexp.MustCompile(`^[a-z0-9]([a-z0-9.-]*[a-z0-9])?$`)

// Ceilings keep a mistyped env value from turning the caps off.
const (
	ceilingMigrateWebhookLimit = 10000
	ceilingMigrateRepoURLMax   = 8192
)

func envPositiveInt(lookupEnv func(string) (string, bool), name string, def, ceiling int) int {
	if lookupEnv == nil {
		return def
	}
	if v, ok := lookupEnv(name); ok {
		if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil && n > 0 {
			return min(n, ceiling)
		}
	}
	return def
}

// webhookLimits reads the per-run hook cap and the repo URL length cap from env.
func webhookLimits(lookupEnv func(string) (string, bool)) (maxHooks, maxURL int) {
	return envPositiveInt(lookupEnv, envMigrateWebhookLimit, defaultMigrateWebhookLimit, ceilingMigrateWebhookLimit),
		envPositiveInt(lookupEnv, envMigrateRepoURLMaxLen, defaultMigrateRepoURLMax, ceilingMigrateRepoURLMax)
}

// unsafeRune covers control and format characters (bidi overrides, zero-width marks).
func unsafeRune(r rune) bool {
	return unicode.IsControl(r) || unicode.Is(unicode.Cf, r)
}

func hasControlChar(s string) bool {
	return strings.IndexFunc(s, unsafeRune) >= 0
}

// cleanDetail keeps a provider or server message printable and short.
func cleanDetail(s string) string {
	s = strings.Map(func(r rune) rune {
		if unsafeRune(r) {
			return ' '
		}
		return r
	}, s)
	if r := []rune(s); len(r) > webhookDetailMaxRunes {
		s = string(r[:webhookDetailMaxRunes]) + "..."
	}
	return strings.TrimSpace(s)
}

// httpsOrigin returns scheme, lowercase host and explicit port, so a repo and a
// connection only match when all three agree.
func httpsOrigin(raw string, maxLen int) (origin, host, reason string) {
	raw = strings.TrimSpace(raw)
	switch {
	case raw == "":
		return "", "", "no repository URL"
	case len(raw) > maxLen:
		return "", "", "the repository URL is longer than " + strconv.Itoa(maxLen) + " bytes (" + envMigrateRepoURLMaxLen + ")"
	case hasControlChar(raw):
		return "", "", "the repository URL contains control characters"
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", "", "the repository URL does not parse"
	}
	if u.Scheme != "https" {
		return "", "", "webhooks are only created for https repository URLs"
	}
	if u.User != nil {
		return "", "", "the repository URL embeds credentials"
	}
	host = strings.ToLower(u.Hostname())
	if !plainHostRe.MatchString(host) {
		return "", "", "the repository host is not a plain DNS name"
	}
	port := u.Port()
	if port == "" {
		port = "443"
	}
	return "https://" + net.JoinHostPort(host, port), host, ""
}

// repoWebhookOrigin also requires an owner/name path, which every provider hook needs.
func repoWebhookOrigin(raw string, maxLen int) (origin, host, reason string) {
	origin, host, reason = httpsOrigin(raw, maxLen)
	if reason != "" {
		return "", "", reason
	}
	u, _ := url.Parse(strings.TrimSpace(raw))
	if !strings.Contains(strings.Trim(u.Path, "/"), "/") {
		return "", "", "the repository URL has no owner and repository path"
	}
	return origin, host, ""
}

// connectedOrigins maps each https origin with an authorized git connection to its provider.
func connectedOrigins(ctx context.Context, client *Client, maxURL int) (map[string]string, error) {
	out := map[string]string{}
	gh, err := client.GetGitHubAppStatus(ctx)
	if err != nil {
		return nil, fmt.Errorf("read the github app connection: %w", err)
	}
	if gh.Connected && gh.Installed {
		instance := gh.InstanceURL
		if instance == "" {
			instance = "https://github.com"
		}
		if o, _, reason := httpsOrigin(instance, maxURL); reason == "" {
			out[o] = providerGitHubApp
		}
	}
	conns, err := client.ListGitConnections(ctx, "")
	if err != nil {
		return nil, fmt.Errorf("list git provider connections: %w", err)
	}
	for _, c := range conns {
		if !c.Authorized {
			continue
		}
		instance := c.InstanceURL
		if instance == "" && c.Host != "" {
			instance = "https://" + c.Host
		}
		if o, _, reason := httpsOrigin(instance, maxURL); reason == "" {
			if _, taken := out[o]; !taken {
				out[o] = cleanDetail(c.Provider)
			}
		}
	}
	return out, nil
}

func gitSourced(a *mappedApp) bool {
	return !a.Blocking && a.Compose == nil && a.RepoURL != ""
}

// planMigrationWebhooks fills the dry-run outcome: no request is made and nothing is written.
func planMigrationWebhooks(apps []mappedApp, lookupEnv func(string) (string, bool)) {
	maxHooks, maxURL := webhookLimits(lookupEnv)
	n := 0
	for i := range apps {
		a := &apps[i]
		if !gitSourced(a) {
			continue
		}
		_, host, reason := repoWebhookOrigin(a.RepoURL, maxURL)
		switch {
		case reason != "":
			a.Webhook = &webhookOutcome{State: webhookSkipped, Detail: reason}
		case n >= maxHooks:
			a.Webhook = &webhookOutcome{State: webhookSkipped, Host: host, Detail: overLimitDetail(maxHooks)}
		default:
			n++
			a.Webhook = &webhookOutcome{State: webhookWouldCreate, Host: host,
				Detail: "with --apply, one push webhook is registered on " + host + " through the git connection for that host, if one is connected; nothing was written"}
		}
	}
}

func overLimitDetail(limit int) string {
	return "over the per-run limit of " + strconv.Itoa(limit) + " webhooks (" + envMigrateWebhookLimit + "); register it with apps git-source register-webhook"
}

// applyMigrationWebhooks registers one push hook per created git-sourced app through the
// existing register route, so the caller's own token and the control plane's own
// secret are used. A failure on one app is recorded and the loop continues.
func applyMigrationWebhooks(ctx context.Context, client *Client, apps []mappedApp, lookupEnv func(string) (string, bool)) {
	maxHooks, maxURL := webhookLimits(lookupEnv)
	var origins map[string]string
	var originsErr error
	loaded := false
	n := 0
	for i := range apps {
		a := &apps[i]
		if !gitSourced(a) {
			continue
		}
		registerHint := "; register it later with apps git-source register-webhook " + a.ServiceName
		if !a.Applied || !a.gitSourceSet {
			a.Webhook = &webhookOutcome{State: webhookSkipped, Detail: "the app or its git source was not created in this run, so no webhook was registered" + registerHint}
			continue
		}
		origin, host, reason := repoWebhookOrigin(a.RepoURL, maxURL)
		if reason != "" {
			a.Webhook = &webhookOutcome{State: webhookSkipped, Detail: reason}
			continue
		}
		if !loaded {
			origins, originsErr = connectedOrigins(ctx, client, maxURL)
			loaded = true
		}
		if originsErr != nil {
			a.Webhook = &webhookOutcome{State: webhookSkipped, Host: host, Detail: cleanDetail(originsErr.Error()) + registerHint}
			continue
		}
		provider, ok := origins[origin]
		if !ok {
			a.Webhook = &webhookOutcome{State: webhookSkipped, Host: host,
				Detail: "no authorized git connection matches https://" + host + " (scheme, host and port), connect the account first" + registerHint}
			continue
		}
		if n >= maxHooks {
			a.Webhook = &webhookOutcome{State: webhookSkipped, Host: host, Provider: provider, Detail: overLimitDetail(maxHooks)}
			continue
		}
		n++
		a.Webhook = registerOneWebhook(ctx, client, a.ServiceName, host, provider)
	}
}

func registerOneWebhook(ctx context.Context, client *Client, name, host, provider string) *webhookOutcome {
	out := &webhookOutcome{Host: host, Provider: provider}
	if gs, err := client.GetGitSource(ctx, name); err == nil && gs.WebhookRegistration != nil && gs.WebhookRegistration.State == webhookRegisteredState {
		out.State, out.Detail = webhookAlready, "a webhook is already registered for this app, none was added"
		return out
	}
	gs, err := client.RegisterGitSourceWebhook(ctx, name)
	switch {
	case err != nil:
		out.State, out.Detail = webhookFailed, cleanDetail(err.Error())
	case gs.WebhookRegistration == nil || gs.WebhookRegistration.State != webhookRegisteredState:
		msg := "the provider did not confirm the webhook"
		if gs.WebhookRegistration != nil && gs.WebhookRegistration.Error != "" {
			msg = gs.WebhookRegistration.Error
		}
		out.State, out.Detail = webhookFailed, cleanDetail(msg)
	default:
		out.State, out.Detail = webhookCreated, "push webhook registered on "+host+" with a secret the control plane generated"
	}
	return out
}

// webhookDryRunNotes lists, for the exit plan, which git-sourced apps --create-webhooks would hook.
func webhookDryRunNotes(apps []platformimport.App, lookupEnv func(string) (string, bool)) []string {
	maxHooks, maxURL := webhookLimits(lookupEnv)
	var out []string
	n := 0
	for _, app := range apps {
		if app.GitURL == "" {
			continue
		}
		name := platformimport.SanitizeName(app.Name)
		clean, _ := platformimport.DisplayRepoURL(app.GitURL)
		_, host, reason := repoWebhookOrigin(clean, maxURL)
		switch {
		case reason != "":
			out = append(out, "webhook for "+name+" would be skipped: "+reason)
		case n >= maxHooks:
			out = append(out, "webhook for "+name+" would be skipped: "+overLimitDetail(maxHooks))
		default:
			n++
			out = append(out, "webhook for "+name+" would be registered on "+host+" after --apply, if a git connection for that host exists")
		}
	}
	return out
}

func printWebhookOutcome(out io.Writer, w *webhookOutcome) {
	if w == nil {
		return
	}
	_, _ = fmt.Fprintf(out, "      webhook %s: %s\n", strings.ReplaceAll(w.State, "_", " "), w.Detail)
}
