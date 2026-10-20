package api

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/fixit"
	"github.com/GLINCKER/levelrail/internal/githubapp"
	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	envGitWebhookAllowPrivate = "APP_GIT_WEBHOOK_ALLOW_PRIVATE"

	webhookWarnPrivate  = "private_address"
	webhookWarnNoPublic = "no_public_url"

	webhookSourcePrimary   = "primary_domain"
	webhookSourceDashboard = "dashboard_url"
	webhookSourceOrigin    = "request_origin"
	webhookSourceRelative  = "relative"

	webhookErrorLimit = 300
)

// gitReliabilityStore persists webhook registration and poll bookkeeping; *store.DB satisfies it.
type gitReliabilityStore interface {
	GetGitSourceReliability(ctx context.Context, name string) (store.GitSourceReliability, error)
	SetGitSourceWebhookState(ctx context.Context, name, state, errMsg string, at time.Time) error
	NoteGitSourceHead(ctx context.Context, name, head string, at time.Time, webhook bool) error
	SetGitSourcePollResult(ctx context.Context, name string, at time.Time, pollErr string) error
}

func (rt *Router) attachGitReliability(s Store) {
	if gs, ok := any(s).(gitReliabilityStore); ok {
		rt.gitRel = gs
	}
}

type webhookURLStatus struct {
	URL     string
	Source  string
	Warning string
}

// isPrivateWebhookHost reports whether a git host on the internet could not
// reach host: localhost, loopback, private, link-local or CGNAT addresses and
// local-only names.
func isPrivateWebhookHost(host string) bool {
	h := strings.ToLower(strings.TrimSuffix(host, "."))
	if h == "" || h == "localhost" || strings.HasSuffix(h, ".localhost") || strings.HasSuffix(h, ".local") || strings.HasSuffix(h, ".internal") || strings.HasSuffix(h, ".lan") || !strings.Contains(h, ".") {
		if _, err := netip.ParseAddr(h); err != nil {
			return true
		}
	}
	addr, err := netip.ParseAddr(h)
	if err != nil {
		return false
	}
	cgnat := netip.MustParsePrefix("100.64.0.0/10")
	return addr.IsLoopback() || addr.IsPrivate() || addr.IsLinkLocalUnicast() || addr.IsUnspecified() || cgnat.Contains(addr)
}

func urlIsPrivate(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil {
		return true
	}
	host := u.Hostname()
	if host == "" {
		host, _, _ = net.SplitHostPort(u.Host)
	}
	return isPrivateWebhookHost(host)
}

func allowPrivateWebhookURL() bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv(envGitWebhookAllowPrivate)))
	return v == "1" || v == "true"
}

// gitWebhookBase resolves the public origin git hosts should call: the
// primary domain, else the dashboard URL, else (when allowed) the origin of
// the request being served. A localhost or private result is flagged.
func (rt *Router) gitWebhookBase(ctx context.Context, withOrigin bool) (base, source, warning string) {
	if b, err := rt.controlPlaneBaseURL(ctx); err == nil {
		return b, webhookSourcePrimary, ""
	}
	if u, err := rt.effectiveDashboardURL(ctx); err == nil && u != "" {
		return u, webhookSourceDashboard, rt.privateWarning(u)
	}
	if withOrigin {
		if o := requestOriginFrom(ctx); o != "" {
			return o, webhookSourceOrigin, rt.privateWarning(o)
		}
	}
	return "", webhookSourceRelative, webhookWarnNoPublic
}

func (rt *Router) privateWarning(base string) string {
	if allowPrivateWebhookURL() || !urlIsPrivate(base) {
		return ""
	}
	return webhookWarnPrivate
}

func (rt *Router) gitSourceWebhookStatus(ctx context.Context, name string) webhookURLStatus {
	base, source, warning := rt.gitWebhookBase(ctx, true)
	return webhookURLStatus{URL: base + gitSourceWebhookPath(name), Source: source, Warning: warning}
}

type gitWebhookRegistrationResource struct {
	State     string     `json:"state"`
	Error     string     `json:"error,omitempty"`
	ChangedAt *time.Time `json:"changed_at,omitempty"`
}

type gitPollStatusResource struct {
	Enabled       bool       `json:"enabled"`
	LastPolledAt  *time.Time `json:"last_polled_at,omitempty"`
	LastWebhookAt *time.Time `json:"last_webhook_at,omitempty"`
	LastSeenHead  string     `json:"last_seen_head,omitempty"`
	Error         string     `json:"error,omitempty"`
}

func timePtrUTC(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	u := t.UTC()
	return &u
}

// gitSourceResourceFor builds the wire resource with the webhook URL status,
// registration outcome and poll state filled in.
func (rt *Router) gitSourceResourceFor(ctx context.Context, g store.GitSource, hasToken bool, name string) gitSourceResource {
	st := rt.gitSourceWebhookStatus(ctx, name)
	res := toGitSourceResource(g, hasToken, st.URL)
	res.WebhookURLWarning, res.WebhookURLSource = st.Warning, st.Source
	res.Connection = rt.gitSourceConnectionState(ctx, g.ConnectionID)
	if rt.gitRel == nil {
		return res
	}
	rel, err := rt.gitRel.GetGitSourceReliability(ctx, name)
	if err != nil {
		rt.logger.Warn("api: load git source reliability failed", slog.String("name", name), slog.String("error", err.Error()))
		return res
	}
	if rel.WebhookState != "" {
		res.WebhookRegistration = &gitWebhookRegistrationResource{State: rel.WebhookState, Error: rel.WebhookError, ChangedAt: timePtrUTC(rel.WebhookChangedAt)}
	}
	res.Poll = &gitPollStatusResource{
		Enabled:      gitPollConfigFromEnv().Enabled && effectiveGitSourceTriggerMode(g.TriggerMode) != "release",
		LastPolledAt: timePtrUTC(rel.LastPolledAt), LastWebhookAt: timePtrUTC(rel.LastWebhookAt), LastSeenHead: rel.LastSeenHead, Error: rel.PollError,
	}
	return res
}

func clipError(err error) string {
	msg := err.Error()
	if len(msg) > webhookErrorLimit {
		msg = msg[:webhookErrorLimit]
	}
	return msg
}

// recordWebhookRegistration stores the outcome of registering name's webhook
// so a failure stays visible on the source instead of only in one response.
func (rt *Router) recordWebhookRegistration(ctx context.Context, name string, regErr error) {
	if rt.gitRel == nil {
		return
	}
	state, msg := store.GitWebhookRegistered, ""
	if regErr != nil {
		state, msg = store.GitWebhookFailed, clipError(regErr)
	}
	if err := rt.gitRel.SetGitSourceWebhookState(ctx, name, state, msg, time.Now()); err != nil {
		rt.logger.Warn("api: record webhook registration failed", slog.String("name", name), slog.String("error", err.Error()))
	}
}

var (
	errWebhookNoPublicURL = errors.New("set the public URL first: this control plane has no address a git host can reach")
	errWebhookNoProvider  = errors.New("no connected git provider matches this repository's host")
)

// registerGitSourceWebhook registers name's push webhook on the provider that
// hosts its repository, using the connected provider app.
func (rt *Router) registerGitSourceWebhook(ctx context.Context, name string, gs store.GitSource) error {
	st := rt.gitSourceWebhookStatus(ctx, name)
	if st.Warning != "" {
		return errWebhookNoPublicURL
	}
	if rt.gitSourceSecrets == nil {
		return errors.New("git sources are not configured on this control plane (no master key set)")
	}
	secret, err := rt.gitSourceSecrets.Resolve(ctx, store.GitSourceSecretsKey(name), gitSourceSecretKey)
	if err != nil {
		return fmt.Errorf("resolve webhook secret: %w", err)
	}
	host, path := repoHostAndPath(gs.RepoURL)
	if host == "" || path == "" {
		return fmt.Errorf("cannot read host and path from %q", gs.RepoURL)
	}
	if gs.ConnectionID != "" {
		return rt.registerHookOnConnection(ctx, gs, path, st.URL, secret)
	}
	if matched, err := rt.registerHookGitHub(ctx, host, path, st.URL, secret); matched {
		if err != nil {
			return fmt.Errorf("register webhook: %w", err)
		}
		return nil
	}
	for _, provider := range connectionGitProviders {
		if _, err := rt.gitConnForRepo(ctx, provider, gs.RepoURL, ""); err != nil {
			continue
		}
		return rt.registerHookOnProvider(ctx, provider, gs, path, st.URL, secret)
	}
	return errWebhookNoProvider
}

func repoHostAndPath(repoURL string) (host, path string) {
	u, err := url.Parse(repoURL)
	if err != nil || u.Host == "" {
		return "", ""
	}
	return strings.ToLower(u.Hostname()), strings.Trim(strings.TrimSuffix(strings.Trim(u.Path, "/"), ".git"), "/")
}

func repoURLHost(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	return strings.ToLower(u.Hostname())
}

func (rt *Router) registerHookGitHub(ctx context.Context, host, path, hookURL, secret string) (bool, error) {
	if rt.githubAppSecrets == nil {
		return false, nil
	}
	conn, err := rt.githubApp.GetGitHubAppConnection(ctx)
	if err != nil || conn.InstallationID == nil {
		return false, nil
	}
	instance := conn.InstanceURL
	if instance == "" {
		instance = "https://github.com"
	}
	if repoURLHost(instance) != host {
		return false, nil
	}
	owner, repo, ok := strings.Cut(path, "/")
	if !ok {
		return false, nil
	}
	instanceURL, token, err := rt.mintGitHubAppInstallationTokenForOwner(ctx, owner)
	if err != nil {
		return true, err
	}
	if err := rt.githubAppClient.CreateRepoWebhook(ctx, instanceURL, token, owner, repo, hookURL, secret); err != nil {
		if errors.Is(err, githubapp.ErrPermissionDenied) {
			return true, errors.New("this github app installation lacks the repository hooks permission; reinstall the app to grant it")
		}
		return true, err
	}
	return true, nil
}

// registerHookOnConnection registers the webhook through the connection the
// source recorded, so a repository picked from one account is never hooked
// with another account's token.
func (rt *Router) registerHookOnConnection(ctx context.Context, gs store.GitSource, path, hookURL, secret string) error {
	conn, err := rt.gitConns.GetGitConnection(ctx, gs.ConnectionID)
	if err != nil {
		return fmt.Errorf("the connection this source was picked from is gone; connect the account again: %w", err)
	}
	return rt.registerHookOnProvider(ctx, conn.Provider, gs, path, hookURL, secret)
}

func (rt *Router) registerHookOnProvider(ctx context.Context, provider string, gs store.GitSource, path, hookURL, secret string) error {
	adapter, ok := rt.gitAdapter(provider)
	if !ok {
		return errWebhookNoProvider
	}
	conn, token, err := rt.gitTokenForRepo(ctx, provider, gs.RepoURL, gs.ConnectionID)
	if err != nil {
		return fmt.Errorf("register webhook: %w", err)
	}
	repo := path
	if provider == gitProviderGitLab {
		repo, err = rt.gitlabProjectIDFor(ctx, conn, path)
		if err != nil {
			return fmt.Errorf("register webhook: %w", err)
		}
	}
	if err := adapter.webhook(ctx, conn, token, repo, hookURL, secret); err != nil {
		return fmt.Errorf("register webhook: %w", err)
	}
	return nil
}

// gitlabProjectIDFor finds the numeric project id behind a namespaced path
// among the projects the connection can see.
func (rt *Router) gitlabProjectIDFor(ctx context.Context, conn store.GitConnection, path string) (string, error) {
	acct := rt.loadConnectionRepos(ctx, conn, false)
	if acct.Err != nil {
		return "", acct.Err
	}
	for _, r := range acct.Result.Repos {
		if strings.EqualFold(r.FullName, path) {
			return strconv.FormatInt(r.ProjectID, 10), nil
		}
	}
	return "", fmt.Errorf("project %q is not visible to the connected gitlab account", path)
}

// handleRegisterGitSourceWebhook handles POST /api/v1/apps/{name}/git-source/webhook/register:
// registers the webhook on the provider again and returns the source with the new outcome.
func (rt *Router) handleRegisterGitSourceWebhook(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	ctx := withRequestOrigin(r.Context(), r)
	gs, err := rt.gitSources.GetGitSource(ctx, name)
	if errors.Is(err, store.ErrGitSourceNotFound) {
		writeError(w, http.StatusNotFound, "no git source is connected for this app")
		return
	}
	if err != nil {
		rt.internalError(w, "api: load git source for webhook registration failed", err, slog.String("name", name))
		return
	}
	regErr := rt.registerGitSourceWebhook(ctx, name, *gs)
	rt.recordWebhookRegistration(ctx, name, regErr)
	if regErr != nil {
		rt.logger.Warn("api: re-register git source webhook failed", slog.String("name", name), slog.String("error", regErr.Error()))
	}
	hasToken, _ := rt.gitSourceSecrets.Exists(ctx, store.GitSourceSecretsKey(name), gitSourceTokenKey)
	writeJSON(w, http.StatusOK, rt.gitSourceResourceFor(ctx, *gs, hasToken, name))
}

// webhookPublicURLFix lets the operator set the dashboard URL that git
// webhook URLs are built from when no primary domain is configured.
func (rt *Router) webhookPublicURLFix() fixit.Fix {
	return fixit.Fix{
		ID: "settings.webhook_url.public", CheckID: "webhook_public_url", Subject: SubjectSettings, Kind: fixit.KindAuto,
		Title: "Set a public URL for git webhooks",
		Why:   "Git hosts call this control plane's webhook URL when you push. Without an address they can reach, pushes never arrive and nothing deploys.",
		Risk:  fixit.RiskSafe, Reversible: true, Needs: "root",
		Detect: func(ctx context.Context) (fixit.Applicability, error) {
			a := fixit.Applicability{Changes: []fixit.Change{}}
			sources, err := rt.gitSources.ListGitSources(ctx)
			if err != nil || len(sources) == 0 {
				a.Detail = "no git sources are connected"
				return a, err
			}
			base, src, warn := rt.gitWebhookBase(ctx, false)
			if warn == "" {
				a.Detail = "webhook URLs use " + base + " (" + src + ")"
				return a, nil
			}
			a.Failing = true
			a.Detail = "git webhook URLs point at an address a git host cannot reach"
			if base != "" {
				a.Detail += ": " + base
			}
			a.Param = &fixit.Param{Name: "url", Free: true, Options: []fixit.Option{}}
			a.Changes = []fixit.Change{{Label: "Save the public URL as the dashboard URL", Detail: "webhook URLs are built from it"}}
			return a, nil
		},
		Run: func(ctx context.Context, in fixit.Input) error {
			u, err := normalizeDashboardURL(in.Params["url"])
			if err != nil {
				return err
			}
			if u == "" {
				return errors.New("a public URL is required")
			}
			if urlIsPrivate(u) && !allowPrivateWebhookURL() {
				return fmt.Errorf("%s is not reachable from a git host; use a public hostname, or set %s=true to allow it", u, envGitWebhookAllowPrivate)
			}
			if err := rt.ingressSettings.SetDashboardURL(ctx, u); err != nil {
				return fmt.Errorf("save dashboard url: %w", err)
			}
			return nil
		},
	}
}
