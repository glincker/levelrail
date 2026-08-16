package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/githubapp"
	"github.com/GLINCKER/levelrail/internal/store"
)

// handleStartGitHubAppRegistration handles
// GET /api/v1/github-app/register/start: the entry point for GitHub's
// manifest flow. Unlike every other handler in this package, this one's
// response body is not JSON: GitHub's manifest flow requires an actual
// browser form POST to https://github.com/settings/apps/new, submitted
// by the browser itself, not a server-to-server call this handler could
// make instead (https://docs.github.com/en/apps/sharing-github-apps/registering-a-github-app-from-a-manifest).
// So this handler renders a minimal, self-submitting HTML page and the
// frontend is expected to navigate the browser here as a real,
// full-page GET (window.location.href, not fetch), the same way it must
// navigate to GitHub's own confirmation page and back.
//
// Preconditions checked before anything else: a master key must be
// configured (rt.githubAppSecrets != nil, since the very next step
// needs somewhere to store the App's credentials), no App may already
// be connected (re-registering over an existing connection would orphan
// the old App on GitHub's side with nothing here still pointing at it),
// and a primary domain must be configured (githubAppBaseURL's own
// requirement: GitHub needs a real, reachable callback URL).
func (rt *Router) handleStartGitHubAppRegistration(w http.ResponseWriter, r *http.Request) {
	if rt.githubAppSecrets == nil {
		writeError(w, http.StatusNotImplemented, "the github app connection requires a master key to be configured on this control plane")
		return
	}

	ctx := r.Context()
	if _, err := rt.githubApp.GetGitHubAppConnection(ctx); err == nil {
		writeError(w, http.StatusConflict, "a github app is already connected; disconnect it first before registering a new one")
		return
	} else if !errors.Is(err, store.ErrGitHubAppConnectionNotFound) {
		rt.logger.Error("api: check existing github app connection failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	baseURL, err := rt.githubAppBaseURL(ctx)
	if err != nil {
		if errors.Is(err, errGitHubAppNoPrimaryDomain) {
			writeError(w, http.StatusConflict, "set a primary domain in ingress settings before connecting a github app: github needs a real, reachable callback url")
			return
		}
		rt.logger.Error("api: get ingress settings for github app registration failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	state, err := rt.githubAppState.begin()
	if err != nil {
		rt.logger.Error("api: begin github app registration failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	appName := githubapp.AppDisplayName(rt.brand.Name, strings.TrimPrefix(baseURL, "https://"))
	manifestJSON, err := json.Marshal(githubapp.BuildManifest(appName, baseURL, rt.githubAppManifestConfig))
	if err != nil {
		rt.logger.Error("api: marshal github app manifest failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeGitHubAppManifestForm(w, state, manifestJSON)
}

// writeGitHubAppManifestForm renders the auto-submitting form GitHub's
// manifest flow expects: a hidden "manifest" field posted to
// github.com/settings/apps/new?state=<state>. No external CSS/JS, no
// template engine dependency (there is no existing html/template use
// anywhere in this codebase to extend, and this is exactly one page
// with exactly two dynamic values), both dynamic values passed through
// html.EscapeString before being placed inside a double-quoted HTML
// attribute: state is crypto/rand base64url output (already
// URL-and-HTML-safe alphabet) and manifestJSON is server-generated from
// brand.Brand.Name plus the operator's own configured primary domain
// (not attacker-controlled request input at this point), so there is no
// real injection surface today, but both are still escaped as a matter
// of course rather than trusted to stay that way.
func writeGitHubAppManifestForm(w http.ResponseWriter, state string, manifestJSON []byte) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	actionURL := "https://github.com/settings/apps/new?state=" + url.QueryEscape(state)
	_, err := fmt.Fprintf(w, `<!doctype html>
<html>
<head><meta charset="utf-8"><title>Connecting GitHub App</title></head>
<body>
<form id="gh-app-manifest-form" action="%s" method="post">
<input type="hidden" name="manifest" value="%s">
</form>
<p>Redirecting to GitHub to finish creating the App...</p>
<script>document.getElementById('gh-app-manifest-form').submit();</script>
</body>
</html>
`, html.EscapeString(actionURL), html.EscapeString(string(manifestJSON)))
	if err != nil {
		// The status line and headers are already flushed by this point
		// (w.WriteHeader above), so there is nothing left to do but log;
		// the browser just gets a truncated page. Same "log, don't
		// propagate" shape writeJSON's own encode-failure branch already
		// establishes for the identical situation.
		slog.Default().Error("api: write github app manifest form failed", slog.String("error", err.Error()))
	}
}

// handleGitHubAppCallback handles GET /api/v1/github-app/callback: the
// redirect_url GitHub sends the operator's browser back to once they
// confirm App creation on GitHub's own manifest review page, carrying
// ?code=...&state=.... This is the one moment the App's real
// credentials (client_secret, webhook_secret, the PEM private key)
// exist in plaintext outside GitHub; they are encrypted through
// rt.githubAppSecrets before anything else touches them, and this
// handler's own error paths never log a credential value, the code
// parameter, or the raw exchange response body: only
// err.Error() (internal/githubapp.Client wraps a truncated,
// non-credential-bearing GitHub error body on failure; the success path
// never gets logged at all beyond the generic flow completing).
//
// code and state are validated present before use; state is further
// checked against the single pending value handleStartGitHubAppRegistration
// generated (githubAppRegistrationState.consume), rejecting a missing,
// unknown, expired, or already-used state with a 400, never a panic.
func (rt *Router) handleGitHubAppCallback(w http.ResponseWriter, r *http.Request) {
	if rt.githubAppSecrets == nil {
		writeError(w, http.StatusNotImplemented, "the github app connection requires a master key to be configured on this control plane")
		return
	}

	q := r.URL.Query()
	code := strings.TrimSpace(q.Get("code"))
	state := strings.TrimSpace(q.Get("state"))
	if code == "" {
		writeError(w, http.StatusBadRequest, "missing code parameter")
		return
	}
	if state == "" {
		writeError(w, http.StatusBadRequest, "missing state parameter")
		return
	}
	if !rt.githubAppState.consume(state) {
		writeError(w, http.StatusBadRequest, "state parameter is invalid, expired, or already used")
		return
	}

	ctx := r.Context()
	creds, err := rt.githubAppClient.ExchangeManifestCode(ctx, code)
	if err != nil {
		rt.logger.Error("api: exchange github app manifest code failed", slog.String("error", err.Error()))
		writeError(w, http.StatusBadGateway, "failed to exchange the manifest code with github")
		return
	}

	secretsKey := store.GitHubAppSecretsKey()
	if err := rt.githubAppSecrets.SetValue(ctx, secretsKey, "client_secret", creds.ClientSecret); err != nil {
		rt.logger.Error("api: store github app client_secret failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if err := rt.githubAppSecrets.SetValue(ctx, secretsKey, "webhook_secret", creds.WebhookSecret); err != nil {
		rt.logger.Error("api: store github app webhook_secret failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if err := rt.githubAppSecrets.SetValue(ctx, secretsKey, "private_key", creds.PrivateKeyPEM); err != nil {
		rt.logger.Error("api: store github app private_key failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	if err := rt.githubApp.SaveGitHubAppConnection(ctx, store.GitHubAppConnection{
		AppID:     creds.AppID,
		ClientID:  creds.ClientID,
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	}); err != nil {
		rt.logger.Error("api: save github app connection failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	// Sends the operator straight into GitHub's own "choose repositories
	// to install on" page for the App they just created: the natural
	// next step in this onboarding, so they don't have to go find it
	// themselves. GitHub redirects back to setup_url (this control
	// plane's own /api/v1/github-app/installed, handleGitHubAppInstalled
	// below) once they finish that step.
	installURL := "https://github.com/apps/" + url.PathEscape(creds.Slug) + "/installations/new"
	http.Redirect(w, r, installURL, http.StatusFound)
}

// handleGitHubAppInstalled handles GET /api/v1/github-app/installed:
// the setup_url GitHub's manifest declared
// (githubapp.BuildManifest), which GitHub redirects the operator's
// browser to after they pick an account/org and repositories to install
// the App on, carrying ?installation_id=...&setup_action=....
//
// installation_id is validated present and parses as a positive
// integer before use, a clean 400 on failure rather than a panic. Once
// parsed, this handler does not simply trust it: it signs a fresh
// App-level JWT (githubapp.SignAppJWT, using the App's own stored
// private key, decrypted in memory only) and calls GitHub's
// GET /app/installations/{id} to look the installation up for real,
// then checks the returned InstallationInfo.AppID against this
// connection's own stored AppID before recording anything. Without that
// check, a crafted or stale installation_id belonging to a completely
// different GitHub App could get recorded as if it belonged to this
// one; GitHub's redirect itself carries no signature over
// installation_id that would rule that out on its own.
func (rt *Router) handleGitHubAppInstalled(w http.ResponseWriter, r *http.Request) {
	if rt.githubAppSecrets == nil {
		writeError(w, http.StatusNotImplemented, "the github app connection requires a master key to be configured on this control plane")
		return
	}

	raw := strings.TrimSpace(r.URL.Query().Get("installation_id"))
	if raw == "" {
		writeError(w, http.StatusBadRequest, "missing installation_id parameter")
		return
	}
	installationID, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || installationID <= 0 {
		writeError(w, http.StatusBadRequest, "installation_id must be a positive integer")
		return
	}

	ctx := r.Context()
	conn, err := rt.githubApp.GetGitHubAppConnection(ctx)
	if errors.Is(err, store.ErrGitHubAppConnectionNotFound) {
		writeError(w, http.StatusConflict, "no github app is registered on this control plane yet")
		return
	}
	if err != nil {
		rt.logger.Error("api: get github app connection for installation callback failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	privateKeyPEM, err := rt.githubAppSecrets.Resolve(ctx, store.GitHubAppSecretsKey(), "private_key")
	if err != nil {
		rt.logger.Error("api: resolve github app private key for installation callback failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	appJWT, err := githubapp.SignAppJWT(conn.AppID, []byte(privateKeyPEM), time.Now())
	if err != nil {
		rt.logger.Error("api: sign github app jwt for installation callback failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	info, err := rt.githubAppClient.GetInstallation(ctx, appJWT, installationID)
	if err != nil {
		// A real, reachable failure mode, not just a hypothetical: an
		// org that requires admin approval to install an App sends this
		// same redirect for a still-pending request, and the
		// installation doesn't exist as a real, readable installation
		// yet. Reported as a clean 502 either way; this can't be
		// distinguished from any other GitHub API failure here.
		rt.logger.Error("api: get github app installation failed", slog.String("error", err.Error()), slog.Int64("installation_id", installationID))
		writeError(w, http.StatusBadGateway, "failed to look up the installation with github; it may still be pending approval")
		return
	}
	if info.AppID != conn.AppID {
		rt.logger.Warn("api: github app installation callback reported an installation for a different app",
			slog.Int64("installation_id", installationID),
			slog.Int64("reported_app_id", info.AppID),
			slog.Int64("expected_app_id", conn.AppID))
		writeError(w, http.StatusBadRequest, "this installation does not belong to this control plane's github app")
		return
	}

	if err := rt.githubApp.UpdateGitHubAppInstallation(ctx, installationID, info.AccountLogin); err != nil {
		rt.logger.Error("api: record github app installation failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	baseURL, err := rt.githubAppBaseURL(ctx)
	if err != nil {
		// PrimaryDomain being unset here would be surprising
		// (registration itself required it to start), but this handler
		// doesn't assume that's impossible: a plain JSON success body is
		// a safe fallback over a redirect built from an empty base URL.
		writeJSON(w, http.StatusOK, map[string]string{"status": "installed"})
		return
	}
	http.Redirect(w, r, baseURL+"/settings/github-app?github_app=installed", http.StatusFound)
}
