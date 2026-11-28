package api

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/glincker/levelrail/internal/attention"
	"github.com/glincker/levelrail/internal/footprint"
	"github.com/glincker/levelrail/internal/runtimesettings"
	"github.com/glincker/levelrail/internal/store"
)

// Credential kinds of a git connection.
const (
	gitCredOAuth  = "oauth_auto_renew" //nolint:gosec // kind name, not a credential
	gitCredStatic = "static_token"     //nolint:gosec // kind name, not a credential
)

// Per-connection secrets keys for refresh bookkeeping; no migration needed.
const (
	gitConnLastRefreshAtKey    = "last_refresh_at"
	gitConnLastRefreshErrKey   = "last_refresh_error"
	gitConnRefreshFailSinceKey = "refresh_failing_since"
	gitConnCredExpiresKey      = "credential_expires_at" //nolint:gosec // secrets key name, not a credential
)

// Event types raised for git credentials, and the dedup state key prefixes
// kept next to the API token reminders.
const (
	eventGitReauthNeeded = "git_connections.reauth_needed"
	eventGitExpiring     = "git_connections.expiring"

	gitReauthStatePrefix = "gitreauth:"
	gitExpiryStatePrefix = "gitcred:"

	gitRefreshTick          = 30 * time.Second
	gitCredErrMax           = 200
	gitRefreshLeadFallback  = 10 * time.Minute
	gitRefreshGraceFallback = 30 * time.Minute
	gitRotateDaysFallback   = 90
)

func (rt *Router) gitRefreshEnabled(ctx context.Context) bool {
	if rt.runtimeSettings == nil {
		return true
	}
	return rt.runtimeSettings.Bool(ctx, runtimesettings.KeyGitRefreshEnabled)
}

func (rt *Router) gitRefreshDuration(ctx context.Context, key string, fallback time.Duration) time.Duration {
	if rt.runtimeSettings == nil {
		return fallback
	}
	return rt.runtimeSettings.Duration(ctx, key)
}

// gitSecret reads one bookkeeping value; empty means absent.
func (rt *Router) gitSecret(ctx context.Context, secretsKey, name string) string {
	if rt.gitConnSecrets == nil {
		return ""
	}
	if ok, err := rt.gitConnSecrets.Exists(ctx, secretsKey, name); err != nil || !ok {
		return ""
	}
	v, err := rt.gitConnSecrets.Resolve(ctx, secretsKey, name)
	if err != nil {
		return ""
	}
	return v
}

func (rt *Router) setGitSecret(ctx context.Context, secretsKey, name, value string) {
	if rt.gitConnSecrets == nil {
		return
	}
	if err := rt.gitConnSecrets.SetValue(ctx, secretsKey, name, value); err != nil {
		rt.logger.Warn("api: save git credential state failed", slog.String("key", name), slog.String("error", err.Error()))
	}
}

func gitUnixSecret(raw string) time.Time {
	n, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || n <= 0 {
		return time.Time{}
	}
	return time.Unix(n, 0).UTC()
}

// parseGitExpiry reads a provider expiry: a date (end of that day, UTC) or RFC3339.
func parseGitExpiry(raw string) time.Time {
	raw = strings.TrimSpace(raw)
	if t, err := time.Parse(time.RFC3339, raw); err == nil {
		return t.UTC()
	}
	if t, err := time.Parse("2006-01-02", raw); err == nil {
		return t.Add(24*time.Hour - time.Second).UTC()
	}
	return time.Time{}
}

// gitRefreshFailureText keeps what an operator may read: the classified
// reason when there is one, else a short provider error.
func gitRefreshFailureText(err error) string {
	var ce *gitConnError
	if errors.As(err, &ce) {
		return ce.Detail
	}
	s := err.Error()
	if len(s) > gitCredErrMax {
		s = s[:gitCredErrMax]
	}
	return s
}

// noteGitRefresh records the outcome of a refresh for one connection id.
func (rt *Router) noteGitRefresh(ctx context.Context, connectionID string, failure error) {
	if rt.gitConnSecrets == nil {
		return
	}
	sk := store.GitConnectionSecretsKey(connectionID)
	now := time.Now()
	if failure == nil {
		rt.setGitSecret(ctx, sk, gitConnLastRefreshAtKey, strconv.FormatInt(now.Unix(), 10))
		if rt.gitSecret(ctx, sk, gitConnRefreshFailSinceKey) != "" {
			rt.setGitSecret(ctx, sk, gitConnLastRefreshErrKey, "")
			rt.setGitSecret(ctx, sk, gitConnRefreshFailSinceKey, "")
		}
		return
	}
	rt.setGitSecret(ctx, sk, gitConnLastRefreshErrKey, gitRefreshFailureText(failure))
	if rt.gitSecret(ctx, sk, gitConnRefreshFailSinceKey) == "" {
		rt.setGitSecret(ctx, sk, gitConnRefreshFailSinceKey, strconv.FormatInt(now.Unix(), 10))
	}
}

// gitCredentialInfo is what the connection response shows about the credential.
type gitCredentialInfo struct {
	Kind          string
	Authorized    bool
	ExpiresAt     time.Time
	LastRefreshAt time.Time
	LastError     string
	FailingSince  time.Time
	CanRotate     bool
}

func (rt *Router) gitCredentialInfo(ctx context.Context, c store.GitConnection) gitCredentialInfo {
	var info gitCredentialInfo
	if rt.gitConnSecrets == nil {
		return info
	}
	if ok, err := rt.gitConnSecrets.Exists(ctx, c.SecretsKey, gitConnAccessTokenKey); err != nil || !ok {
		return info
	}
	info.Authorized = true
	if rt.gitSecret(ctx, c.SecretsKey, gitConnRefreshTokenKey) != "" {
		info.Kind = gitCredOAuth
		info.ExpiresAt = gitUnixSecret(rt.gitSecret(ctx, c.SecretsKey, gitConnTokenExpiresKey))
	} else {
		info.Kind = gitCredStatic
		info.ExpiresAt = gitUnixSecret(rt.gitSecret(ctx, c.SecretsKey, gitConnCredExpiresKey))
		_, info.CanRotate = rt.gitlabAppClient.(gitlabTokenRotator)
		info.CanRotate = info.CanRotate && c.Provider == gitProviderGitLab
	}
	info.LastRefreshAt = gitUnixSecret(rt.gitSecret(ctx, c.SecretsKey, gitConnLastRefreshAtKey))
	info.FailingSince = gitUnixSecret(rt.gitSecret(ctx, c.SecretsKey, gitConnRefreshFailSinceKey))
	if !info.FailingSince.IsZero() {
		info.LastError = rt.gitSecret(ctx, c.SecretsKey, gitConnLastRefreshErrKey)
	}
	return info
}

// fillGitCredentialFields adds the credential health fields to a connection response.
func (rt *Router) fillGitCredentialFields(ctx context.Context, c store.GitConnection, res *gitConnectionResource) {
	info := rt.gitCredentialInfo(ctx, c)
	res.CredentialKind = info.Kind
	res.AutoRenew = info.Kind == gitCredOAuth && rt.gitRefreshEnabled(ctx)
	res.CanRotate = info.CanRotate
	res.LastRefreshError = info.LastError
	if !info.ExpiresAt.IsZero() {
		res.ExpiresAt = info.ExpiresAt.Format(time.RFC3339)
	}
	if !info.LastRefreshAt.IsZero() {
		res.LastRefreshAt = info.LastRefreshAt.Format(time.RFC3339)
	}
}

// gitReauthReason says why a connection needs the operator, or "" when it does not:
// the provider rejected the grant, or refreshes have failed for longer than the grace.
func (rt *Router) gitReauthReason(ctx context.Context, c store.GitConnection, info gitCredentialInfo, now time.Time, grace time.Duration) string {
	if info.Kind != gitCredOAuth {
		return ""
	}
	h := rt.gitConnHealth(ctx, c.ID)
	if h.Status == store.GitConnNeedsReauthorize || h.Status == store.GitConnRevoked {
		if h.Detail != "" {
			return h.Detail
		}
		return "the authorization was revoked or has expired"
	}
	if !info.FailingSince.IsZero() && now.Sub(info.FailingSince) >= grace {
		if info.LastError != "" {
			return "the access token could not be refreshed: " + info.LastError
		}
		return "the access token could not be refreshed"
	}
	return ""
}

// gitStaticExpiryLevel is the reminder level of a static token with a known expiry, -1 when none is due.
func (rt *Router) gitStaticExpiryLevel(info gitCredentialInfo, now time.Time, windows []int) int {
	if info.Kind != gitCredStatic || info.ExpiresAt.IsZero() {
		return -1
	}
	return reminderLevel(info.ExpiresAt.Sub(now), windows)
}

// RunGitCredentialRefresh renews OAuth access tokens ahead of expiry and
// keeps the static token expiry list current, each on its own interval.
func (rt *Router) RunGitCredentialRefresh(ctx context.Context) error {
	var lastOAuth, lastStatic time.Time
	return footprint.Every(ctx, "git credential refresh", gitRefreshTick, rt.logger, func(ctx context.Context) error {
		now := time.Now()
		if now.Sub(lastOAuth) >= rt.gitRefreshDuration(ctx, runtimesettings.KeyGitRefreshInterval, gitRefreshTick) {
			lastOAuth = now
			rt.SweepGitOAuth(ctx, now)
		}
		if now.Sub(lastStatic) >= rt.gitRefreshDuration(ctx, runtimesettings.KeyTokensReminderInterval, gitRefreshTick) {
			lastStatic = now
			return rt.SweepGitStaticTokens(ctx, now)
		}
		return nil
	})
}

// refreshGitConn runs one refresh with the given lead and returns the error, if any.
func (rt *Router) refreshGitConn(ctx context.Context, conn store.GitConnection, lead time.Duration) error {
	adapter, ok := rt.gitAdapter(conn.Provider)
	if !ok || adapter.refresh == nil {
		return nil
	}
	_, err := rt.refreshGitOAuth(ctx, gitOAuthSpec{
		provider: conn.Provider, key: conn.ID,
		read: func(ctx context.Context) (gitOAuthState, error) {
			return rt.readGitOAuthState(ctx, rt.gitConnSecrets, conn.SecretsKey, gitConnAccessTokenKey, gitConnRefreshTokenKey, gitConnTokenExpiresKey)
		},
		exchange: func(ctx context.Context, refresh string) (gitOAuthTokens, error) {
			secret, err := rt.gitConnSecrets.Resolve(ctx, conn.SecretsKey, adapter.secretKey)
			if err != nil {
				return gitOAuthTokens{}, fmt.Errorf("resolve client secret: %w", err)
			}
			return adapter.refresh(ctx, conn, secret, refresh)
		},
		save: func(ctx context.Context, t gitOAuthTokens) error { return rt.saveGitTokens(ctx, conn, t) },
	}, lead)
	return err
}

// SweepGitOAuth refreshes the access tokens that fall inside the lead,
// skipping connections that already need a new authorization, then raises or
// clears the re-authorize alert once per connection.
func (rt *Router) SweepGitOAuth(ctx context.Context, now time.Time) {
	if rt.gitConns == nil || rt.gitConnSecrets == nil || !rt.gitRefreshEnabled(ctx) {
		return
	}
	lead := rt.gitRefreshDuration(ctx, runtimesettings.KeyGitRefreshLead, gitRefreshLeadFallback)
	grace := rt.gitRefreshDuration(ctx, runtimesettings.KeyGitRefreshFailureGrace, gitRefreshGraceFallback)
	conns, err := rt.gitConns.ListGitConnections(ctx, "")
	if err != nil {
		rt.logger.Warn("api: git credential refresh: list connections failed", slog.String("error", err.Error()))
		return
	}
	sent, err := rt.tokenStateLoad(ctx, tokenStateNotices)
	if err != nil {
		rt.logger.Warn("api: git credential refresh: load state failed", slog.String("error", err.Error()))
		return
	}
	set := map[string]int64{}
	var drop []string
	for _, c := range conns {
		info := rt.gitCredentialInfo(ctx, c)
		if info.Kind != gitCredOAuth {
			continue
		}
		h := rt.gitConnHealth(ctx, c.ID)
		skip := h.Status == store.GitConnNeedsReauthorize || h.Status == store.GitConnRevoked || h.Status == store.GitConnRateLimited
		if !skip && !now.Add(lead).Before(info.ExpiresAt) {
			if err := rt.refreshGitConn(ctx, c, lead); err != nil {
				rt.logger.Warn("api: git credential refresh failed", slog.String("connection", c.ID), slog.String("provider", c.Provider), slog.String("error", gitRefreshFailureText(err)))
			}
			info = rt.gitCredentialInfo(ctx, c)
		}
		key := gitReauthStatePrefix + c.ID
		reason := rt.gitReauthReason(ctx, c, info, now, grace)
		_, raised := sent[key]
		switch {
		case reason != "" && !raised:
			rt.EmitEvent(eventGitReauthNeeded, map[string]any{
				"connection_id": c.ID, "provider": c.Provider, "label": c.Label, "host": c.Host,
				"reason": reason, "authorize_path": gitAuthorizePath(c),
			})
			set[key] = 1
		case reason == "" && raised:
			drop = append(drop, key)
		}
	}
	drop = append(drop, rt.goneGitStateKeys(sent, conns)...)
	rt.applyGitState(ctx, set, drop)
}

func gitAuthorizePath(c store.GitConnection) string {
	return "/api/v1/" + c.Provider + "-app/connect?connection_id=" + url.QueryEscape(c.ID)
}

// goneGitStateKeys lists dedup keys of connections that no longer exist.
func (rt *Router) goneGitStateKeys(sent map[string]int64, conns []store.GitConnection) []string {
	live := map[string]bool{}
	for _, c := range conns {
		live[c.ID] = true
	}
	var gone []string
	for k := range sent {
		for _, p := range []string{gitReauthStatePrefix, gitExpiryStatePrefix} {
			if id, ok := strings.CutPrefix(k, p); ok && !live[id] {
				gone = append(gone, k)
			}
		}
	}
	return gone
}

func (rt *Router) applyGitState(ctx context.Context, set map[string]int64, drop []string) {
	if len(set) == 0 && len(drop) == 0 {
		return
	}
	err := rt.tokenStateUpdate(ctx, tokenStateNotices, func(m map[string]int64) {
		for k, v := range set {
			m[k] = v
		}
		for _, k := range drop {
			delete(m, k)
		}
	})
	if err != nil {
		rt.logger.Warn("api: save git credential notices failed", slog.String("error", err.Error()))
	}
}

// SweepGitStaticTokens learns the real expiry of pasted tokens from the host
// and sends one event per reminder window, the same windows API tokens use.
func (rt *Router) SweepGitStaticTokens(ctx context.Context, now time.Time) error {
	if rt.gitConns == nil || rt.gitConnSecrets == nil {
		return nil
	}
	conns, err := rt.gitConns.ListGitConnections(ctx, "")
	if err != nil {
		return fmt.Errorf("list git connections: %w", err)
	}
	sent, err := rt.tokenStateLoad(ctx, tokenStateNotices)
	if err != nil {
		return err
	}
	windows := rt.reminderWindows(ctx)
	set := map[string]int64{}
	var drop []string
	for _, c := range conns {
		info := rt.gitCredentialInfo(ctx, c)
		if info.Kind != gitCredStatic {
			continue
		}
		if exp, ok := rt.learnGitExpiry(ctx, c); ok {
			info.ExpiresAt = exp
		}
		key := gitExpiryStatePrefix + c.ID
		level := rt.gitStaticExpiryLevel(info, now, windows)
		switch {
		case level < 0:
			if _, had := sent[key]; had {
				drop = append(drop, key)
			}
		case dueReminder(sent, key, level):
			rt.EmitEvent(eventGitExpiring, map[string]any{
				"connection_id": c.ID, "provider": c.Provider, "label": c.Label, "host": c.Host,
				"expires_at": info.ExpiresAt.Format(time.RFC3339), "window_days": level,
				"rotate_path": gitRotatePath(c, info),
			})
			set[key] = int64(level)
		}
	}
	drop = append(drop, rt.goneGitStateKeys(sent, conns)...)
	rt.applyGitState(ctx, set, drop)
	return nil
}

func gitRotatePath(c store.GitConnection, info gitCredentialInfo) string {
	if !info.CanRotate {
		return ""
	}
	return "/api/v1/git-connections/" + c.ID + "/rotate"
}

// learnGitExpiry asks the host when a pasted token expires and stores the
// answer; hosts that cannot say leave the stored value alone.
func (rt *Router) learnGitExpiry(ctx context.Context, c store.GitConnection) (time.Time, bool) {
	ti, ok := rt.gitlabAppClient.(gitlabTokenInfoer)
	if !ok || c.Provider != gitProviderGitLab {
		return time.Time{}, false
	}
	token := rt.gitSecret(ctx, c.SecretsKey, gitConnAccessTokenKey)
	if token == "" {
		return time.Time{}, false
	}
	cctx, cancel := context.WithTimeout(ctx, gitProbeTimeout)
	defer cancel()
	info, err := ti.TokenInfo(cctx, c.InstanceURL, token)
	if err != nil {
		return time.Time{}, false
	}
	exp := parseGitExpiry(info.ExpiresAt)
	if exp.IsZero() {
		rt.setGitSecret(ctx, c.SecretsKey, gitConnCredExpiresKey, "")
		return time.Time{}, true
	}
	rt.setGitSecret(ctx, c.SecretsKey, gitConnCredExpiresKey, strconv.FormatInt(exp.Unix(), 10))
	return exp, true
}

// gitCredentialAttentionItems raises one row per connection that needs the
// operator, derived from stored state so it clears the moment the cause does.
func (rt *Router) gitCredentialAttentionItems(r *http.Request, abilities []string, now time.Time) []attention.Item {
	if !hasAbility(abilities, AbilityReadSensitive) || rt.gitConns == nil || rt.gitConnSecrets == nil {
		return nil
	}
	ctx := r.Context()
	conns, err := rt.gitConns.ListGitConnections(ctx, "")
	if err != nil {
		rt.logger.Warn("api: attention feed: list git connections failed", slog.String("error", err.Error()))
		return nil
	}
	grace := rt.gitRefreshDuration(ctx, runtimesettings.KeyGitRefreshFailureGrace, gitRefreshGraceFallback)
	windows := rt.reminderWindows(ctx)
	var items []attention.Item
	for _, c := range conns {
		info := rt.gitCredentialInfo(ctx, c)
		if reason := rt.gitReauthReason(ctx, c, info, now, grace); reason != "" {
			it := feedItem(attention.Warning, attention.KindGitConnectionReauth, c.ID, c.Label+" ("+c.Host+"): "+reason,
				map[string]string{"connection_id": c.ID, "provider": c.Provider})
			it.Title = "Git connection " + c.Label + " needs to be authorized again"
			it.Link = gitConnLink(c.ID, true)
			items = append(items, it)
			continue
		}
		level := rt.gitStaticExpiryLevel(info, now, windows)
		if level < 0 {
			continue
		}
		sev, verb := attention.Warning, "expires "+info.ExpiresAt.Format("2006-01-02")
		if level == 0 {
			sev, verb = attention.Critical, "expired "+info.ExpiresAt.Format("2006-01-02")
		}
		it := feedItem(sev, attention.KindGitTokenExpiring, c.ID, c.Label+" ("+c.Host+"): the access token "+verb,
			map[string]string{"connection_id": c.ID, "provider": c.Provider, "can_rotate": strconv.FormatBool(info.CanRotate)})
		it.Title = "Git access token of " + c.Label + " " + verb
		it.Link = gitConnLink(c.ID, false)
		items = append(items, it)
	}
	return items
}

func gitConnLink(id string, reauthorize bool) string {
	l := "/settings/integrations?connection=" + url.QueryEscape(id)
	if reauthorize {
		l += "&reauthorize=1"
	}
	return l
}

// doctorCheckGitOAuthRefresh reports whether authorized git connections renew
// themselves, and warns when the switch is off or a connection is failing.
func (rt *Router) doctorCheckGitOAuthRefresh(ctx context.Context) doctorCheckResource {
	const code, name = "git_oauth_refresh", "Git OAuth auto-renew"
	res := doctorCheckResource{Code: code, Name: name, Status: doctorStatusOK, DocsPath: gitConnectionsDocsPath}
	if !rt.gitRefreshEnabled(ctx) {
		res.Status = doctorStatusWarn
		res.Message = "auto-renew is off: access tokens refresh only when a request needs them, so a connection can fail after an idle stretch; turn on git.oauth_refresh_enabled"
		return res
	}
	if rt.gitConns == nil || rt.gitConnSecrets == nil {
		res.Message = "no git connections to renew"
		return res
	}
	conns, err := rt.gitConns.ListGitConnections(ctx, "")
	if err != nil {
		res.Status, res.Message = doctorStatusWarn, "could not list git connections"
		return res
	}
	grace := rt.gitRefreshDuration(ctx, runtimesettings.KeyGitRefreshFailureGrace, gitRefreshGraceFallback)
	var renewing int
	var failing []string
	for _, c := range conns {
		info := rt.gitCredentialInfo(ctx, c)
		if info.Kind != gitCredOAuth {
			continue
		}
		renewing++
		if rt.gitReauthReason(ctx, c, info, time.Now(), grace) != "" {
			failing = append(failing, c.Label)
		}
	}
	if len(failing) > 0 {
		res.Status = doctorStatusWarn
		res.Message = strings.Join(failing, ", ") + ": the authorization needs to be redone, press Re-authorize on the connection"
		res.Evidence = strings.Join(failing, ", ")
		return res
	}
	res.Message = fmt.Sprintf("auto-renew is on, %d OAuth connection(s) renew themselves", renewing)
	return res
}
