package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/gitprovider"
	"github.com/GLINCKER/levelrail/internal/runtimesettings"
	"github.com/GLINCKER/levelrail/internal/store"
)

// gitlabTokenRotator is the optional GitLab surface that replaces a personal access token.
type gitlabTokenRotator interface {
	RotateSelf(ctx context.Context, instanceURL, accessToken string, expiresAt time.Time) (token, newExpiry string, err error)
}

const gitRotateSaveAttempts = 3

type rotateGitConnectionRequest struct {
	// ExpiresAt is an optional YYYY-MM-DD date for the new token; the host may cap it.
	ExpiresAt string `json:"expires_at"`
}

// gitRotateResource is the masked result of a rotation: the secret is never returned.
type gitRotateResource struct {
	Rotated    bool                  `json:"rotated"`
	ExpiresAt  string                `json:"expires_at,omitempty"`
	Connection gitConnectionResource `json:"connection"`
}

func (rt *Router) gitRotateExpiry(ctx context.Context, req rotateGitConnectionRequest, now time.Time) (time.Time, string) {
	if strings.TrimSpace(req.ExpiresAt) != "" {
		t, err := time.Parse("2006-01-02", strings.TrimSpace(req.ExpiresAt))
		if err != nil || !t.After(now) {
			return time.Time{}, "expires_at must be a future date as YYYY-MM-DD"
		}
		return t, ""
	}
	days := gitRotateDaysFallback
	if rt.runtimeSettings != nil {
		days = rt.runtimeSettings.Int(ctx, runtimesettings.KeyGitRotateLifetimeDays)
	}
	return now.AddDate(0, 0, days), ""
}

// handleRotateGitConnection handles POST /api/v1/git-connections/{id}/rotate:
// replaces a pasted GitLab personal access token through the host and stores
// the new secret and expiry. The response carries metadata only.
func (rt *Router) handleRotateGitConnection(w http.ResponseWriter, r *http.Request) {
	c, ok := rt.loadGitConnection(w, r)
	if !ok {
		return
	}
	if rt.gitConnSecrets == nil {
		writeError(w, http.StatusNotImplemented, errGitMasterKeyRequired)
		return
	}
	var req rotateGitConnectionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	ctx := r.Context()
	info := rt.gitCredentialInfo(ctx, c)
	switch {
	case !info.Authorized:
		writeError(w, http.StatusConflict, "this connection is not authorized yet; authorize it first")
		return
	case info.Kind == gitCredOAuth:
		writeError(w, http.StatusConflict, "an authorized connection renews its own access token and cannot be rotated; use Re-authorize if it stopped working")
		return
	case !info.CanRotate:
		writeError(w, http.StatusConflict, "this provider cannot replace a pasted token through its API; create a new token on the host and reconnect")
		return
	}
	expiry, msg := rt.gitRotateExpiry(ctx, req, time.Now())
	if msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}
	newExpiry, err := rt.rotateGitLabToken(ctx, c, expiry)
	if err != nil {
		rt.writeGitRotateError(w, c, err)
		return
	}
	rt.auditGitConn(ctx, r, "git_connection.rotated", c.ID)
	rt.logger.Info("api: git token rotated", slog.String("connection", c.ID), slog.String("provider", c.Provider))
	writeJSON(w, http.StatusOK, gitRotateResource{Rotated: true, ExpiresAt: newExpiry, Connection: rt.gitConnectionResourceFor(ctx, c)})
}

// rotateGitLabToken rotates under the connection's lock and saves the result.
// The old token dies at the host the moment it rotates, so the save retries
// before giving up.
func (rt *Router) rotateGitLabToken(ctx context.Context, c store.GitConnection, expiry time.Time) (string, error) {
	rotator, ok := rt.gitlabAppClient.(gitlabTokenRotator)
	if !ok {
		return "", errGitRotateUnsupported
	}
	unlock := rt.gitOAuthLocks.lock(c.ID)
	defer unlock()
	old, err := rt.gitConnSecrets.Resolve(ctx, c.SecretsKey, gitConnAccessTokenKey)
	if err != nil {
		return "", err
	}
	newToken, newExpiry, err := rotator.RotateSelf(ctx, c.InstanceURL, old, expiry)
	if err != nil {
		return "", err
	}
	if newToken == "" {
		return "", errors.New("the host returned no token")
	}
	exp := parseGitExpiry(newExpiry)
	var saveErr error
	for range gitRotateSaveAttempts {
		if saveErr = rt.saveRotatedGitToken(ctx, c, newToken, exp); saveErr == nil {
			break
		}
	}
	if saveErr != nil {
		return "", errGitRotateLost
	}
	rt.markGitConnOK(ctx, c.ID)
	rt.clearGitExpiryNotice(ctx, c.ID)
	gitAccessCacheMu.Lock()
	delete(gitAccessCache, c.ID)
	gitAccessCacheMu.Unlock()
	return newExpiry, nil
}

// saveRotatedGitToken writes the expiry before the token so a half save never leaves a fresh token with a stale date.
func (rt *Router) saveRotatedGitToken(ctx context.Context, c store.GitConnection, token string, exp time.Time) error {
	expRaw := ""
	if !exp.IsZero() {
		expRaw = strconv.FormatInt(exp.Unix(), 10)
	}
	if err := rt.gitConnSecrets.SetValue(ctx, c.SecretsKey, gitConnCredExpiresKey, expRaw); err != nil {
		return err
	}
	return rt.gitConnSecrets.SetValue(ctx, c.SecretsKey, gitConnAccessTokenKey, token)
}

// clearGitExpiryNotice re-arms the reminders for the new expiry.
func (rt *Router) clearGitExpiryNotice(ctx context.Context, id string) {
	rt.applyGitState(ctx, nil, []string{gitExpiryStatePrefix + id})
}

var (
	errGitRotateUnsupported = errors.New("api: git rotate is not supported")
	errGitRotateLost        = errors.New("api: git rotate could not save the new token")
)

func (rt *Router) writeGitRotateError(w http.ResponseWriter, c store.GitConnection, err error) {
	var apiErr *gitprovider.APIError
	switch {
	case errors.Is(err, errGitRotateUnsupported):
		writeError(w, http.StatusConflict, "this provider cannot replace a pasted token through its API")
	case errors.Is(err, errGitRotateLost):
		rt.logger.Error("api: git rotate could not save the new token; the connection must be reconnected", slog.String("connection", c.ID))
		writeError(w, http.StatusInternalServerError, "the host rotated the token but it could not be saved; create a new token and reconnect this connection")
	case errors.As(err, &apiErr) && (apiErr.StatusCode == http.StatusForbidden || apiErr.StatusCode == http.StatusUnauthorized):
		writeError(w, http.StatusConflict, "the host refused the rotation; the token needs the self_rotate scope")
	case errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusNotFound:
		writeError(w, http.StatusConflict, "this GitLab version cannot rotate a token through its API; create a new token and reconnect")
	default:
		rt.logger.Warn("api: git token rotation failed", slog.String("connection", c.ID), slog.String("error", gitAccessErrText(err)))
		writeError(w, http.StatusBadGateway, "the provider did not rotate the token")
	}
}
