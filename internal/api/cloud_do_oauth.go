package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/glincker/levelrail/internal/cloudconn"
	"github.com/glincker/levelrail/internal/doauth"
	"github.com/glincker/levelrail/internal/runtimesettings"
	"github.com/glincker/levelrail/internal/secretmask"
	"github.com/glincker/levelrail/internal/store"
)

const (
	doOAuthCallbackPath = "/api/v1/cloud-accounts/oauth/digitalocean/callback"
	doOAuthRefreshKey   = "oauth_refresh_token" //nolint:gosec // legacy layout key, read only for migration
	doOAuthExpiresKey   = "oauth_expires_at"    // legacy layout key, read only for migration
	doOAuthStatePrefix  = "do-oauth"
)

// doOAuthBase is a var so tests can point the client at a fake token endpoint.
var doOAuthBase = doauth.DefaultBase

type doOAuthStatusResponse struct {
	Enabled     bool   `json:"enabled"`
	RedirectURI string `json:"redirect_uri,omitempty"`
	Hint        string `json:"hint,omitempty"`
}

type doOAuthStartResponse struct {
	URL string `json:"url"`
}

func (rt *Router) doOAuthCreds(ctx context.Context) (clientID, secret string) {
	if rt.runtimeSettings == nil {
		return "", ""
	}
	return strings.TrimSpace(rt.runtimeSettings.String(ctx, runtimesettings.KeyCloudOAuthDOClientID)),
		strings.TrimSpace(rt.runtimeSettings.String(ctx, runtimesettings.KeyCloudOAuthDOClientSecret))
}

func (rt *Router) doOAuthClient(ctx context.Context) doauth.Client {
	secs := 15
	if rt.runtimeSettings != nil {
		secs = rt.runtimeSettings.Int(ctx, runtimesettings.KeyCloudOAuthDOTimeout)
	}
	return doauth.Client{Base: doOAuthBase, Timeout: time.Duration(secs) * time.Second}
}

func (rt *Router) doOAuthLead(ctx context.Context) time.Duration {
	if rt.runtimeSettings == nil {
		return 5 * time.Minute
	}
	return time.Duration(rt.runtimeSettings.Int(ctx, runtimesettings.KeyCloudOAuthDORefreshLead)) * time.Second
}

func isDigitalOceanOAuthAccount(a store.CloudAccount) bool {
	return a.Provider == cloudconn.ProviderDigitalOcean && a.AuthMethod == cloudconn.AuthOAuth
}

func (rt *Router) handleDOOAuthStatus(w http.ResponseWriter, r *http.Request) {
	id, secret := rt.doOAuthCreds(r.Context())
	if id == "" || secret == "" {
		writeJSON(w, http.StatusOK, doOAuthStatusResponse{Hint: "Set cloud.oauth.digitalocean.client_id and cloud.oauth.digitalocean.client_secret in runtime settings to enable this."})
		return
	}
	base, err := rt.controlPlaneBaseURL(r.Context())
	if err != nil {
		writeJSON(w, http.StatusOK, doOAuthStatusResponse{Hint: "Set a primary domain in ingress settings so DigitalOcean has a redirect URL to return to."})
		return
	}
	writeJSON(w, http.StatusOK, doOAuthStatusResponse{Enabled: true, RedirectURI: base + doOAuthCallbackPath})
}

// handleDOOAuthStart returns the authorize URL; the state is bound to the calling principal and a PKCE verifier.
func (rt *Router) handleDOOAuthStart(w http.ResponseWriter, r *http.Request) {
	if rt.nodeProviderSecrets == nil {
		writeError(w, http.StatusNotImplemented, "cloud accounts need a master key on this control plane")
		return
	}
	ctx := r.Context()
	clientID, secret := rt.doOAuthCreds(ctx)
	if clientID == "" || secret == "" {
		writeError(w, http.StatusConflict, "DigitalOcean OAuth is not configured: set the client id and secret in runtime settings")
		return
	}
	base, err := rt.controlPlaneBaseURL(ctx)
	if err != nil {
		if errors.Is(err, errNoPrimaryDomain) {
			writeError(w, http.StatusConflict, "set a primary domain in ingress settings before connecting DigitalOcean: it needs a reachable redirect url")
			return
		}
		rt.internalError(w, "api: base url for digitalocean oauth failed", err)
		return
	}
	pid, pok := rt.doOAuthOwner(r)
	if !pok {
		writeError(w, http.StatusUnauthorized, "sign in to connect DigitalOcean")
		return
	}
	verifier, err := doauth.NewVerifier()
	if err != nil {
		rt.internalError(w, "api: digitalocean oauth verifier failed", err)
		return
	}
	state, err := rt.gitConnState.begin(strings.Join([]string{doOAuthStatePrefix, pid, verifier}, "\n"))
	if err != nil {
		rt.internalError(w, "api: digitalocean oauth state failed", err)
		return
	}
	rt.auditCloud(r, "cloud.account.oauth_started", map[string]any{"provider": cloudconn.ProviderDigitalOcean})
	writeJSON(w, http.StatusOK, doOAuthStartResponse{URL: rt.doOAuthClient(ctx).AuthorizeURL(clientID, base+doOAuthCallbackPath, state, doauth.Challenge(verifier))})
}

// handleDOOAuthCallback validates the single-use state against the signed-in principal, then stores a new account.
func (rt *Router) handleDOOAuthCallback(w http.ResponseWriter, r *http.Request) {
	st, ok := rt.cloudStoreOr501(w)
	if !ok {
		return
	}
	if rt.nodeProviderSecrets == nil {
		writeError(w, http.StatusNotImplemented, "cloud accounts need a master key on this control plane")
		return
	}
	q := r.URL.Query()
	code, state := strings.TrimSpace(q.Get("code")), strings.TrimSpace(q.Get("state"))
	if code == "" || state == "" {
		writeError(w, http.StatusBadRequest, "missing code or state parameter")
		return
	}
	val, found := rt.gitConnState.consume(state)
	parts := strings.Split(val, "\n")
	if !found || len(parts) != 3 || parts[0] != doOAuthStatePrefix || parts[2] == "" {
		writeError(w, http.StatusBadRequest, "state parameter is invalid, expired, or already used")
		return
	}
	if pid, pok := rt.doOAuthOwner(r); !pok || pid != parts[1] {
		writeError(w, http.StatusForbidden, "this authorization was started by a different session")
		return
	}
	ctx := r.Context()
	clientID, secret := rt.doOAuthCreds(ctx)
	base, err := rt.controlPlaneBaseURL(ctx)
	if clientID == "" || secret == "" || err != nil {
		writeError(w, http.StatusConflict, "DigitalOcean OAuth is not configured")
		return
	}
	tok, err := rt.doOAuthClient(ctx).Exchange(ctx, clientID, secret, base+doOAuthCallbackPath, code, parts[2])
	if err != nil {
		rt.logger.Warn("api: digitalocean oauth code exchange failed")
		writeError(w, http.StatusBadGateway, "failed to exchange the authorization code with DigitalOcean")
		return
	}
	id, err := newCloudID(cloudconn.ProviderDigitalOcean + "-")
	if err != nil {
		rt.internalError(w, "api: mint cloud account id failed", err)
		return
	}
	acct := store.CloudAccount{ID: id, Provider: cloudconn.ProviderDigitalOcean, DisplayName: "DigitalOcean (OAuth)", AuthMethod: cloudconn.AuthOAuth,
		Config: json.RawMessage("{}"), CredentialRef: cloudAccountSecretPref + id, CreatedBy: rt.nodeProviderActor(r)}
	if err := rt.saveDOOAuthTokens(ctx, acct, tok); err != nil {
		rt.internalError(w, "api: seal digitalocean oauth tokens failed", err, slog.String("account", id))
		return
	}
	if err := st.CreateCloudAccount(ctx, acct); err != nil {
		rt.internalError(w, "api: create digitalocean oauth account failed", err, slog.String("account", id))
		return
	}
	rt.nodeProviderCatalog.invalidate(acct.Provider)
	rt.auditCloud(r, "cloud.account.created", cloudAccountAuditDetail(acct))
	http.Redirect(w, r, base+"/settings/cloud-accounts?connected="+url.QueryEscape(id), http.StatusFound)
}

// doOAuthOwner resolves the user a request acts for: a session user, or the owner of a bearer token,
// so a CLI start and the browser callback of the same person agree.
func (rt *Router) doOAuthOwner(r *http.Request) (string, bool) {
	if pinned, ok := rt.currentPinnedSession(r); ok && pinned.userID != "" {
		return pinned.userID, true
	}
	if uid, ok := rt.currentSessionUserID(r); ok && uid != "" {
		return uid, true
	}
	if token, ok := bearerToken(r); ok {
		if rec, err := rt.lookupBearerToken(r.Context(), token); err == nil && rec.OwnerUserID != "" {
			return rec.OwnerUserID, true
		}
	}
	return "", false
}

// doOAuthBundle is the single sealed value that holds both tokens and the expiry.
type doOAuthBundle struct {
	Access    string `json:"access"`
	Refresh   string `json:"refresh"`
	ExpiresAt int64  `json:"expires_at"`
}

func newDOOAuthBundle(t doauth.Tokens, keepRefresh string) doOAuthBundle {
	b := doOAuthBundle{Access: t.Access, Refresh: t.Refresh, ExpiresAt: time.Now().Add(t.ExpiresIn).Unix()}
	if b.Refresh == "" {
		b.Refresh = keepRefresh
	}
	secretmask.Add(b.Access)
	secretmask.Add(b.Refresh)
	return b
}

func (rt *Router) saveDOOAuthBundle(ctx context.Context, ref string, b doOAuthBundle) error {
	raw, err := json.Marshal(b) //nolint:gosec // sealed bundle, never logged
	if err != nil {
		return err
	}
	return rt.nodeProviderSecrets.SetValue(ctx, ref, store.NodeProviderTokenEnvKey, string(raw))
}

func (rt *Router) saveDOOAuthTokens(ctx context.Context, a store.CloudAccount, t doauth.Tokens) error {
	return rt.saveDOOAuthBundle(ctx, cloudAccountSecretRef(a), newDOOAuthBundle(t, ""))
}

// loadDOOAuthBundle reads the sealed bundle and still understands the older three value layout.
func (rt *Router) loadDOOAuthBundle(ctx context.Context, ref string) (b doOAuthBundle, legacy bool, err error) {
	v, err := rt.nodeProviderSecrets.Resolve(ctx, ref, store.NodeProviderTokenEnvKey)
	if err != nil {
		return b, false, err
	}
	if strings.HasPrefix(strings.TrimSpace(v), "{") {
		if err := json.Unmarshal([]byte(v), &b); err != nil {
			return b, false, errors.New("stored token bundle is not readable")
		}
		return b, false, nil
	}
	b.Access = v
	b.Refresh, _ = rt.nodeProviderSecrets.Resolve(ctx, ref, doOAuthRefreshKey)
	rawExp, err := rt.nodeProviderSecrets.Resolve(ctx, ref, doOAuthExpiresKey)
	if err != nil {
		return b, true, err
	}
	if b.ExpiresAt, err = strconv.ParseInt(rawExp, 10, 64); err != nil {
		return b, true, errors.New("stored token expiry is not readable")
	}
	return b, true, nil
}

// doOAuthAccountLive re-reads the account so a disabled or deleted one never gets tokens written back.
func (rt *Router) doOAuthAccountLive(ctx context.Context, id string) bool {
	st, ok := rt.cloudAcctStore()
	if !ok {
		return true
	}
	a, err := st.GetCloudAccount(ctx, id)
	return err == nil && a.Status != store.CloudAccountStatusDisabled
}

// doOAuthDetached keeps a refresh and its save from being cut short by a client that went away,
// because the provider rotates the refresh token on use and a lost write strands the account.
func (rt *Router) doOAuthDetached(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(ctx), 2*rt.doOAuthClient(ctx).Timeout)
}

// doOAuthAccessToken returns a usable access token, refreshing it first when it is near expiry.
// The lock serializes refreshes per account because DigitalOcean rotates the refresh token on every use.
func (rt *Router) doOAuthAccessToken(ctx context.Context, a store.CloudAccount) (string, error) {
	unlock := rt.gitOAuthLocks.lock("do-oauth:" + a.ID)
	defer unlock()
	ref := cloudAccountSecretRef(a)
	cur, legacy, err := rt.loadDOOAuthBundle(ctx, ref)
	if err != nil {
		return "", err
	}
	if time.Now().Add(rt.doOAuthLead(ctx)).Before(time.Unix(cur.ExpiresAt, 0)) && cur.Access != "" {
		return cur.Access, nil
	}
	if cur.Refresh == "" {
		return "", errors.New("the DigitalOcean authorization expired, connect the account again")
	}
	clientID, secret := rt.doOAuthCreds(ctx)
	if clientID == "" || secret == "" {
		return "", errors.New("DigitalOcean OAuth is not configured")
	}
	dctx, cancel := rt.doOAuthDetached(ctx)
	defer cancel()
	if !rt.doOAuthAccountLive(dctx, a.ID) {
		return "", errors.New("the cloud account is disabled")
	}
	tok, err := rt.doOAuthClient(dctx).Refresh(dctx, clientID, secret, cur.Refresh)
	if err != nil {
		return "", errors.New("the DigitalOcean authorization could not be refreshed, connect the account again")
	}
	next := newDOOAuthBundle(tok, cur.Refresh)
	if !rt.doOAuthAccountLive(dctx, a.ID) {
		return "", errors.New("the cloud account is disabled")
	}
	if err := rt.saveDOOAuthBundle(dctx, ref, next); err != nil {
		return "", err
	}
	if legacy {
		_ = rt.nodeProviderSecrets.SetValue(dctx, ref, doOAuthRefreshKey, "")
		_ = rt.nodeProviderSecrets.SetValue(dctx, ref, doOAuthExpiresKey, "0")
	}
	return next.Access, nil
}

// revokeDOOAuthAccount revokes both tokens at DigitalOcean on delete or disable, then blanks them locally.
// It takes the refresh lock so it cannot interleave with a rotation in flight.
func (rt *Router) revokeDOOAuthAccount(ctx context.Context, a store.CloudAccount) {
	if !isDigitalOceanOAuthAccount(a) || rt.nodeProviderSecrets == nil {
		return
	}
	unlock := rt.gitOAuthLocks.lock("do-oauth:" + a.ID)
	defer unlock()
	ref := cloudAccountSecretRef(a)
	cur, _, err := rt.loadDOOAuthBundle(ctx, ref)
	if err != nil {
		return
	}
	dctx, cancel := rt.doOAuthDetached(ctx)
	defer cancel()
	cl := rt.doOAuthClient(dctx)
	for _, tok := range []string{cur.Access, cur.Refresh} {
		if tok == "" {
			continue
		}
		if err := cl.Revoke(dctx, tok); err != nil {
			rt.logger.Warn("api: revoke digitalocean oauth token failed", slog.String("account", a.ID))
		}
	}
	if err := rt.saveDOOAuthBundle(dctx, ref, doOAuthBundle{}); err != nil {
		rt.logger.Warn("api: blank digitalocean oauth tokens failed", slog.String("account", a.ID))
	}
	_ = rt.nodeProviderSecrets.SetValue(dctx, ref, doOAuthRefreshKey, "")
	_ = rt.nodeProviderSecrets.SetValue(dctx, ref, doOAuthExpiresKey, "0")
}
