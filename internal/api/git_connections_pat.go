package api

import (
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

// createPATGitConnection saves a connection that authenticates with a personal
// access token instead of an OAuth application. The token is verified against
// the instance first, then stored encrypted; it is never logged or returned.
func (rt *Router) createPATGitConnection(w http.ResponseWriter, r *http.Request, adapter gitProviderAdapter, req createGitConnectionRequest) {
	ctx := r.Context()
	req.Token = strings.TrimSpace(req.Token)
	req.Label = strings.TrimSpace(req.Label)
	req.InstanceURL = strings.TrimRight(strings.TrimSpace(req.InstanceURL), "/")
	if req.InstanceURL == "" || req.Token == "" {
		writeError(w, http.StatusBadRequest, "instance_url and token are required")
		return
	}
	if err := requireHTTPOrHTTPSScheme(req.InstanceURL); err != nil {
		writeError(w, http.StatusBadRequest, "instance_url must use http or https")
		return
	}
	host := store.RepoHost(req.InstanceURL)
	if host == "" {
		writeError(w, http.StatusBadRequest, "instance_url must be a valid http or https address")
		return
	}
	if len(req.Label) > gitConnLabelMax {
		writeError(w, http.StatusBadRequest, "label is too long")
		return
	}
	id, err := store.NewGitConnectionID()
	if err != nil {
		rt.internalError(w, "api: generate git connection id failed", err)
		return
	}
	conn := store.GitConnection{
		ID: id, Provider: adapter.provider, Label: req.Label, Host: host, InstanceURL: req.InstanceURL,
		SecretsKey: store.GitConnectionSecretsKey(id), Scopes: adapter.scopes, CreatedAt: time.Now().UTC().Format(time.RFC3339),
	}
	acct, err := rt.lookupGitAccount(ctx, adapter, conn, req.Token)
	if err != nil || acct.ID == "" {
		rt.logger.Warn("api: verify git personal access token failed", slog.String("provider", adapter.provider), slog.String("instance_url", redactURLCredentials(req.InstanceURL)), slog.String("error", errString(err)))
		writeError(w, http.StatusBadGateway, adapter.label+" did not accept the token; check the instance URL and that the token can read repositories")
		return
	}
	if existing, found, err := rt.gitConns.FindGitConnectionByAccount(ctx, adapter.provider, host, acct.ID, "", ""); err == nil && found {
		writeError(w, http.StatusConflict, "this account is already connected as "+existing.Label)
		return
	}
	if conn.Label == "" {
		conn.Label = gitConnDefaultLabel(adapter.provider, host, acct.Login)
	}
	conn.AccountID, conn.AccountName = acct.ID, acct.Login
	if _, _, name, ok := rt.currentActor(r); ok {
		conn.CreatedBy = name
	}
	if err := rt.saveGitTokens(ctx, conn, gitOAuthTokens{Access: req.Token, Expires: time.Now().AddDate(bitbucketServerTokenExpiryYears, 0, 0)}); err != nil {
		_ = rt.gitConnSecrets.DeleteAll(ctx, conn.SecretsKey)
		rt.internalError(w, "api: store git connection token failed", err, slog.String("connection", id))
		return
	}
	if err := rt.gitConns.CreateGitConnection(ctx, conn); err != nil {
		_ = rt.gitConnSecrets.DeleteAll(ctx, conn.SecretsKey)
		rt.internalError(w, "api: save git connection failed", err, slog.String("connection", id), slog.String("instance_url", redactURLCredentials(req.InstanceURL)))
		return
	}
	created, err := rt.gitConns.GetGitConnection(ctx, id)
	if err != nil {
		rt.internalError(w, "api: reload git connection failed", err, slog.String("connection", id))
		return
	}
	rt.auditGitConn(ctx, r, "git_connection.authorized", id)
	rt.markGitConnOK(ctx, id)
	rt.logger.Info("api: git connection created", slog.String("connection", id), slog.String("provider", adapter.provider), slog.String("host", host))
	writeJSON(w, http.StatusCreated, rt.gitConnectionResourceFor(ctx, created))
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
