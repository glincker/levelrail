package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/GLINCKER/levelrail/internal/store"
)

// Capability summary for one git provider, enough for the git-source picker to choose its controls without a query per provider.
type gitProviderResource struct {
	Provider           string `json:"provider"`
	Connected          bool   `json:"connected"`
	CanListBranches    bool   `json:"can_list_branches"`
	CanRegisterWebhook bool   `json:"can_register_webhook"`
	CanAuthClone       bool   `json:"can_auth_clone"`
	// Connection is the last known health of the connection: ok, needs_reauthorize, revoked or rate_limited.
	Connection gitConnResource `json:"connection"`
	// Connections counts configured connections; Authorized counts those with a token.
	Connections int `json:"connections"`
	Authorized  int `json:"authorized"`
}

// Replaces three separate provider status calls. AbilityReadSensitive, since it discloses only connection state and capabilities.
func (rt *Router) handleListGitProviders(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	out := []gitProviderResource{rt.githubProviderStatus(ctx)}
	for _, p := range connectionGitProviders {
		out = append(out, rt.gitProviderStatusFor(ctx, p))
	}
	for i := range out {
		out[i].Connection = rt.gitProviderHealth(ctx, out[i].Provider)
	}
	writeJSON(w, http.StatusOK, out)
}

// Connected means the App is connected and installed, since repo, branch, and webhook calls need the installation.
// CanAuthClone does not distinguish github.com from GitHub Enterprise Server; that clone-auth gap is tracked separately.
func (rt *Router) githubProviderStatus(ctx context.Context) gitProviderResource {
	res := gitProviderResource{Provider: "github"}
	conn, err := rt.githubApp.GetGitHubAppConnection(ctx)
	if err != nil {
		if !errors.Is(err, store.ErrGitHubAppConnectionNotFound) {
			rt.logger.Error("api: get github app connection for git-providers status failed", slog.String("error", err.Error()))
		}
		return res
	}
	res.Connected = conn.InstallationID != nil
	res.Connections, res.Authorized = 1, 1
	res.CanListBranches = res.Connected
	res.CanRegisterWebhook = res.Connected
	res.CanAuthClone = res.Connected
	return res
}

// gitProviderHealth is the worst recorded health among a provider's connections.
func (rt *Router) gitProviderHealth(ctx context.Context, provider string) gitConnResource {
	if provider == gitProviderGitHub {
		return rt.gitConnHealth(ctx, provider)
	}
	conns, err := rt.gitConns.ListGitConnections(ctx, provider)
	worst := gitConnResource{Status: store.GitConnOK}
	if err != nil {
		return worst
	}
	for _, c := range conns {
		if h := rt.gitConnHealth(ctx, c.ID); gitConnSeverity(h.Status) > gitConnSeverity(worst.Status) {
			worst = h
		}
	}
	return worst
}

func gitConnSeverity(status string) int {
	switch status {
	case store.GitConnRevoked:
		return 3
	case store.GitConnNeedsReauthorize:
		return 2
	case store.GitConnRateLimited:
		return 1
	}
	return 0
}
