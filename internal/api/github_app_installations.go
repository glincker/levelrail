package api

import (
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/GLINCKER/levelrail/internal/store"
)

// gitHubAppInstallationResource is one entry of
// GET /api/v1/github-app/installations.
type gitHubAppInstallationResource struct {
	ID             int64  `json:"id"`
	InstallationID int64  `json:"installation_id"`
	AccountLogin   string `json:"account_login"`
	AccountType    string `json:"account_type"`
	ConnectedAt    string `json:"connected_at"`
	// SettingsURL opens this installation on GitHub, where the repository
	// access list is changed ("Missing a repository?"). Empty when the
	// connection has no instance URL.
	SettingsURL string `json:"settings_url,omitempty"`
}

// installationSettingsURL is where GitHub lets an account owner change which
// repositories one installation can see. A personal account and an
// organization have different settings roots.
func installationSettingsURL(instanceURL, accountType, login string, installationID int64) string {
	base := strings.TrimSuffix(instanceURL, "/")
	if base == "" || installationID <= 0 {
		return ""
	}
	id := strconv.FormatInt(installationID, 10)
	if accountType == "organization" {
		return base + "/organizations/" + url.PathEscape(login) + "/settings/installations/" + id
	}
	return base + "/settings/installations/" + id
}

// gitHubAppInstallationListResource adds AddOrgURL alongside the
// connected accounts/orgs: the frontend's "Add organization" button
// links straight to it rather than constructing the URL itself.
type gitHubAppInstallationListResource struct {
	Installations []gitHubAppInstallationResource `json:"installations"`
	// AddOrgURL is empty when the connection predates migrations/0282
	// (no slug recorded) and hasn't been re-registered since: GitHub's
	// installation API response has no slug field to backfill it from,
	// so the only fix is reconnecting the App (manifest flow records it
	// automatically) or pasting it in through the manual connect form.
	AddOrgURL string `json:"add_org_url,omitempty"`
	// AppPublic is false when GitHub reports the App private, in which
	// case it can only be installed on its owner's account; nil when
	// GitHub could not be asked.
	AppPublic *bool `json:"app_public,omitempty"`
	// MakePublicURL opens the App's Advanced settings, where the owner
	// can allow installs on any account.
	MakePublicURL string `json:"make_public_url,omitempty"`
}

// handleListGitHubAppInstallations handles
// GET /api/v1/github-app/installations: every connected account/org,
// plus the URL to connect another one. AbilityRoot, matching every other
// route that reads the connection itself (see handleGetGitHubAppStatus's
// own route comment in routes_platform.go) rather than
// AbilityReadSensitive: this lists real GitHub account/org names tied to
// platform configuration, not per-repo data.
func (rt *Router) handleListGitHubAppInstallations(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	installations, err := rt.githubApp.ListGitHubAppInstallations(ctx)
	if err != nil {
		rt.logger.Error("api: list github app installations failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	conn, connErr := rt.githubApp.GetGitHubAppConnection(ctx)
	instanceURL := ""
	if connErr == nil {
		instanceURL = conn.InstanceURL
	}

	out := make([]gitHubAppInstallationResource, 0, len(installations))
	for _, inst := range installations {
		out = append(out, gitHubAppInstallationResource{
			SettingsURL:    installationSettingsURL(instanceURL, inst.AccountType, inst.AccountLogin, inst.InstallationID),
			ID:             inst.ID,
			InstallationID: inst.InstallationID,
			AccountLogin:   inst.AccountLogin,
			AccountType:    inst.AccountType,
			ConnectedAt:    inst.ConnectedAt,
		})
	}

	resp := gitHubAppInstallationListResource{Installations: out}
	if connErr == nil && conn.Slug != nil && *conn.Slug != "" {
		resp.AddOrgURL = strings.TrimSuffix(conn.InstanceURL, "/") + "/apps/" + url.PathEscape(*conn.Slug) + "/installations/new"
		resp.AppPublic = rt.githubAppPublic(ctx, conn.InstanceURL, *conn.Slug)
		if resp.AppPublic != nil && !*resp.AppPublic {
			var ownerType, ownerLogin string
			if len(installations) > 0 {
				ownerType, ownerLogin = installations[0].AccountType, installations[0].AccountLogin
			}
			resp.MakePublicURL = githubAppMakePublicURL(conn.InstanceURL, *conn.Slug, ownerType, ownerLogin)
		}
	}
	writeJSON(w, http.StatusOK, resp)
}

// handleDeleteGitHubAppInstallation handles
// DELETE /api/v1/github-app/installations/{id}: disconnects one
// account/org, not the whole App (that's handleDisconnectGitHubApp).
// Blocked with 409 while any git source still points at a repo under
// this account, checked explicitly since repo URLs don't carry an
// installation id to key a foreign key off of.
func (rt *Router) handleDeleteGitHubAppInstallation(w http.ResponseWriter, r *http.Request) {
	idRaw := r.PathValue("id")
	id, err := strconv.ParseInt(idRaw, 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "id must be a positive integer")
		return
	}

	ctx := r.Context()
	installations, err := rt.githubApp.ListGitHubAppInstallations(ctx)
	if err != nil {
		rt.logger.Error("api: list github app installations for delete failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	var target *store.GitHubAppInstallation
	for i := range installations {
		if installations[i].ID == id {
			target = &installations[i]
			break
		}
	}
	if target == nil {
		writeError(w, http.StatusNotFound, "installation not found")
		return
	}

	conn, err := rt.githubApp.GetGitHubAppConnection(ctx)
	if err != nil && !errors.Is(err, store.ErrGitHubAppConnectionNotFound) {
		rt.logger.Error("api: get github app connection for delete installation failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	instanceURL := conn.InstanceURL
	if instanceURL == "" {
		instanceURL = "https://github.com"
	}

	sources, err := rt.gitSources.ListGitSources(ctx)
	if err != nil {
		rt.logger.Error("api: list git sources for delete installation check failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	var blocking []string
	for _, gs := range sources {
		owner, _, ok := githubOwnerRepoFromURL(gs.RepoURL, instanceURL)
		if ok && strings.EqualFold(owner, target.AccountLogin) {
			blocking = append(blocking, gs.ServiceName)
		}
	}
	if len(blocking) > 0 {
		writeError(w, http.StatusConflict, "still in use by: "+strings.Join(blocking, ", ")+"; disconnect or move those git sources first")
		return
	}

	if err := rt.githubApp.DeleteGitHubAppInstallation(ctx, id); err != nil {
		if errors.Is(err, store.ErrGitHubAppInstallationNotFound) {
			writeError(w, http.StatusNotFound, "installation not found")
			return
		}
		rt.logger.Error("api: delete github app installation failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
