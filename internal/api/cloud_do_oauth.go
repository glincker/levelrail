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

	"github.com/GLINCKER/levelrail/internal/cloudconn"
	"github.com/GLINCKER/levelrail/internal/doauth"
	"github.com/GLINCKER/levelrail/internal/runtimesettings"
	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	doOAuthCallbackPath = "/api/v1/cloud-accounts/oauth/digitalocean/callback"
	doOAuthRefreshKey   = "oauth_refresh_token" //nolint:gosec // secret key name, not a credential
	doOAuthExpiresKey   = "oauth_expires_at"
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
	_, pid, _, perr := rt.callerPrincipal(r)
	if perr != nil || pid == "" {
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
	if _, pid, _, perr := rt.callerPrincipal(r); perr != nil || pid != parts[1] {
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

func (rt *Router) saveDOOAuthTokens(ctx context.Context, a store.CloudAccount, t doauth.Tokens) error {
	ref := cloudAccountSecretRef(a)
	if err := rt.nodeProviderSecrets.SetValue(ctx, ref, store.NodeProviderTokenEnvKey, t.Access); err != nil {
		return err
	}
	if err := rt.nodeProviderSecrets.SetValue(ctx, ref, doOAuthRefreshKey, t.Refresh); err != nil {
		return err
	}
	exp := time.Now().Add(t.ExpiresIn).Unix()
	return rt.nodeProviderSecrets.SetValue(ctx, ref, doOAuthExpiresKey, strconv.FormatInt(exp, 10))
}

// doOAuthAccessToken returns a usable access token, refreshing it first when it is near expiry.
// The lock serializes refreshes per account because DigitalOcean rotates the refresh token on every use.
func (rt *Router) doOAuthAccessToken(ctx context.Context, a store.CloudAccount) (string, error) {
	unlock := rt.gitOAuthLocks.lock("do-oauth:" + a.ID)
	defer unlock()
	ref := cloudAccountSecretRef(a)
	access, err := rt.nodeProviderSecrets.Resolve(ctx, ref, store.NodeProviderTokenEnvKey)
	if err != nil {
		return "", err
	}
	rawExp, err := rt.nodeProviderSecrets.Resolve(ctx, ref, doOAuthExpiresKey)
	if err != nil {
		return "", err
	}
	exp, err := strconv.ParseInt(rawExp, 10, 64)
	if err != nil {
		return "", errors.New("stored token expiry is not readable")
	}
	if time.Now().Add(rt.doOAuthLead(ctx)).Before(time.Unix(exp, 0)) {
		return access, nil
	}
	refresh, err := rt.nodeProviderSecrets.Resolve(ctx, ref, doOAuthRefreshKey)
	if err != nil || refresh == "" {
		return "", errors.New("the DigitalOcean authorization expired, connect the account again")
	}
	clientID, secret := rt.doOAuthCreds(ctx)
	if clientID == "" || secret == "" {
		return "", errors.New("DigitalOcean OAuth is not configured")
	}
	tok, err := rt.doOAuthClient(ctx).Refresh(ctx, clientID, secret, refresh)
	if err != nil {
		return "", errors.New("the DigitalOcean authorization could not be refreshed, connect the account again")
	}
	if tok.Refresh == "" {
		tok.Refresh = refresh
	}
	if err := rt.saveDOOAuthTokens(ctx, a, tok); err != nil {
		return "", err
	}
	return tok.Access, nil
}

// revokeDOOAuthAccount revokes the token at DigitalOcean on delete or disable, then blanks the refresh token locally.
func (rt *Router) revokeDOOAuthAccount(ctx context.Context, a store.CloudAccount) {
	if !isDigitalOceanOAuthAccount(a) || rt.nodeProviderSecrets == nil {
		return
	}
	ref := cloudAccountSecretRef(a)
	access, err := rt.nodeProviderSecrets.Resolve(ctx, ref, store.NodeProviderTokenEnvKey)
	if err != nil {
		return
	}
	if err := rt.doOAuthClient(ctx).Revoke(ctx, access); err != nil {
		rt.logger.Warn("api: revoke digitalocean oauth token failed", slog.String("account", a.ID))
	}
	_ = rt.nodeProviderSecrets.SetValue(ctx, ref, doOAuthRefreshKey, "")
	_ = rt.nodeProviderSecrets.SetValue(ctx, ref, doOAuthExpiresKey, "0")
}
