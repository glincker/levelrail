package api

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	exportTTL           = 15 * time.Minute
	exportBuildTimeout  = time.Minute
	exportCooldown      = time.Minute
	exportActivityLimit = 5000

	accountExportPending = "pending"
	accountExportReady   = "ready"
	accountExportFailed  = "failed"
)

type accountExport struct {
	id      string
	userID  string
	status  string
	data    []byte
	expires time.Time
}

type accountExportResource struct {
	ID        string    `json:"id"`
	Status    string    `json:"status"`
	SizeBytes int       `json:"size_bytes,omitempty"`
	ExpiresAt time.Time `json:"expires_at"`
}

func (e *accountExport) resource() accountExportResource {
	return accountExportResource{ID: e.id, Status: e.status, SizeBytes: len(e.data), ExpiresAt: e.expires}
}

// exportFor returns the caller's own job, dropping expired ones; another
// user's id reads as missing.
func (s *accountSelfState) exportFor(id, userID string, now time.Time) (accountExport, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for k, e := range s.exports {
		if now.After(e.expires) {
			delete(s.exports, k)
		}
	}
	e, ok := s.exports[id]
	if !ok || e.userID != userID {
		return accountExport{}, false
	}
	return *e, true
}

// startExport replaces the user's older jobs with a fresh pending one.
func (s *accountSelfState) startExport(id, userID string, now time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.exports == nil {
		s.exports = map[string]*accountExport{}
	}
	for k, e := range s.exports {
		if e.userID == userID || now.After(e.expires) {
			delete(s.exports, k)
		}
	}
	s.exports[id] = &accountExport{id: id, userID: userID, status: accountExportPending, expires: now.Add(exportTTL)}
}

func (s *accountSelfState) finishExport(id, status string, data []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if e, ok := s.exports[id]; ok {
		e.status, e.data = status, data
	}
}

// handleStartExport handles POST /api/v1/auth/export: bundles the caller's own
// profile, sessions, tokens (never secrets) and activity in the background.
func (rt *Router) handleStartExport(w http.ResponseWriter, r *http.Request) {
	userID, viaSession, ok := rt.accountCaller(w, r, AbilityReadSensitive)
	if !ok {
		return
	}
	now := time.Now()
	if !rt.accountSelf.takeExport(userID, now) {
		w.Header().Set("Retry-After", strconv.Itoa(int(exportCooldown.Seconds())))
		writeCodedError(w, http.StatusTooManyRequests, "export_cooldown", "an export was requested a moment ago: wait a minute before asking again")
		return
	}
	id, err := randomTokenID()
	if err != nil {
		rt.internalError(w, "api: export: id failed", err)
		return
	}
	rt.accountSelf.startExport(id, userID, now)
	rt.recordAccountAction(r, userID, viaSession, accountEventExportRequested, http.StatusAccepted)
	go rt.buildAccountExport(id, userID)
	e, _ := rt.accountSelf.exportFor(id, userID, now)
	writeJSON(w, http.StatusAccepted, e.resource())
}

func (rt *Router) buildAccountExport(id, userID string) {
	ctx, cancel := context.WithTimeout(context.Background(), exportBuildTimeout)
	defer cancel()
	data, err := rt.zipAccountData(ctx, userID)
	if err != nil {
		rt.logger.Error("api: account export failed", slog.String("user_id", userID), slog.String("error", err.Error()))
		rt.accountSelf.finishExport(id, accountExportFailed, nil)
		return
	}
	rt.accountSelf.finishExport(id, accountExportReady, data)
}

// handleExportStatus handles GET /api/v1/auth/export/{id}.
func (rt *Router) handleExportStatus(w http.ResponseWriter, r *http.Request) {
	userID, _, ok := rt.accountCaller(w, r, AbilityReadSensitive)
	if !ok {
		return
	}
	e, found := rt.accountSelf.exportFor(r.PathValue("id"), userID, time.Now())
	if !found {
		writeError(w, http.StatusNotFound, "export not found or expired")
		return
	}
	writeJSON(w, http.StatusOK, e.resource())
}

// handleDownloadExport handles GET /api/v1/auth/export/{id}/download.
func (rt *Router) handleDownloadExport(w http.ResponseWriter, r *http.Request) {
	userID, viaSession, ok := rt.accountCaller(w, r, AbilityReadSensitive)
	if !ok {
		return
	}
	e, found := rt.accountSelf.exportFor(r.PathValue("id"), userID, time.Now())
	if !found {
		writeError(w, http.StatusNotFound, "export not found or expired")
		return
	}
	if e.status != accountExportReady {
		writeCodedError(w, http.StatusConflict, "export_"+e.status, "the export is not ready")
		return
	}
	rt.recordAccountAction(r, userID, viaSession, accountEventExportDownloaded, http.StatusOK)
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", `attachment; filename="`+rt.brand.ShortName+`-account-export.zip"`)
	w.Header().Set("Content-Length", strconv.Itoa(len(e.data)))
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(e.data)
}

type exportProfile struct {
	ID          string     `json:"id"`
	Email       string     `json:"email"`
	DisplayName string     `json:"display_name"`
	CreatedAt   time.Time  `json:"created_at"`
	LastLoginAt *time.Time `json:"last_login_at,omitempty"`
}

type exportProvider struct {
	Provider string    `json:"provider"`
	LinkedAt time.Time `json:"linked_at"`
}

func (rt *Router) zipAccountData(ctx context.Context, userID string) ([]byte, error) {
	user, err := rt.auth.GetUserByID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("load user: %w", err)
	}
	files := map[string]any{
		"profile.json": exportProfile{ID: user.ID, Email: user.Email, DisplayName: user.DisplayName, CreatedAt: user.CreatedAt, LastLoginAt: user.LastLoginAt},
	}
	if db := rt.accountDB(); db != nil {
		prefs, err := db.GetUserPreferences(ctx, userID)
		if err != nil {
			return nil, fmt.Errorf("preferences: %w", err)
		}
		files["preferences.json"] = toPreferencesResource(prefs)
	}
	if files["sessions.json"], err = rt.exportSessions(ctx, userID); err != nil {
		return nil, err
	}
	tokens, err := rt.libraryListTokens(ctx, userID, false)
	if err != nil {
		return nil, fmt.Errorf("tokens: %w", err)
	}
	files["tokens.json"] = tokens
	if rt.oauthIdentities != nil {
		ids, err := rt.oauthIdentities.ListOAuthIdentitiesForUser(ctx, userID)
		if err != nil {
			return nil, fmt.Errorf("linked providers: %w", err)
		}
		providers := make([]exportProvider, 0, len(ids))
		for _, i := range ids {
			providers = append(providers, exportProvider{Provider: i.Provider, LinkedAt: i.CreatedAt})
		}
		files["linked_providers.json"] = providers
	}
	if db := rt.teamAdmin(); db != nil {
		roles, err := db.UserTeamRoles(ctx, userID)
		if err != nil {
			return nil, fmt.Errorf("teams: %w", err)
		}
		files["teams.json"] = roles
	}
	rows, err := rt.auditLog.ListAuditEntries(ctx, exportActivityLimit, nil, store.AuditEntryFilter{OwnerUserID: userID})
	if err != nil {
		return nil, fmt.Errorf("activity: %w", err)
	}
	activity := make([]myActivityEntry, 0, len(rows))
	for _, e := range rows {
		activity = append(activity, toMyActivityEntry(e))
	}
	files["activity.json"] = activity
	files["manifest.json"] = map[string]any{
		"generated_at": time.Now().UTC(), "contains_secrets": false, "activity_limit": exportActivityLimit,
	}
	return zipJSON(files)
}

func (rt *Router) exportSessions(ctx context.Context, userID string) ([]sessionResource, error) {
	views, err := rt.libSessions.ListUserSessions(ctx, userID, "")
	if err != nil {
		return nil, fmt.Errorf("sessions: %w", err)
	}
	out := make([]sessionResource, 0, len(views))
	for _, v := range views {
		out = append(out, sessionResource{ID: v.ID, Browser: browserLabel(v.UserAgent), IP: safeSignInIP(v.IP), Network: ipNetworkPrefix(v.IP),
			CreatedAt: v.CreatedAt, LastSeenAt: v.LastSeenAt, ExpiresAt: v.ExpiresAt})
	}
	return out, nil
}

func zipJSON(files map[string]any) ([]byte, error) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, v := range files {
		raw, err := json.MarshalIndent(v, "", "  ")
		if err != nil {
			return nil, fmt.Errorf("encode %s: %w", name, err)
		}
		f, err := zw.Create(name)
		if err != nil {
			return nil, fmt.Errorf("zip %s: %w", name, err)
		}
		if _, err := f.Write(raw); err != nil {
			return nil, fmt.Errorf("zip %s: %w", name, err)
		}
	}
	if err := zw.Close(); err != nil {
		return nil, fmt.Errorf("close zip: %w", err)
	}
	return buf.Bytes(), nil
}
