package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/glincker/levelrail/internal/janitor"
	"github.com/glincker/levelrail/internal/runtimesettings"
	"github.com/glincker/levelrail/internal/store"
)

const (
	bulkSampleSize              = 10
	bulkAuditNameLimit          = 20
	auditActionTokensBulkRevoke = "tokens.bulk_revoke" //nolint:gosec // audit action name
)

type bulkRevokeRequest struct {
	IDs          []string     `json:"ids"`
	Filter       *tokenFilter `json:"filter"`
	ConfirmCount *int         `json:"confirm_count"`
	DryRun       bool         `json:"dry_run"`
}

type bulkRevokeResponse struct {
	DryRun  bool     `json:"dry_run"`
	Count   int      `json:"count"`
	Sample  []string `json:"sample"`
	Revoked int      `json:"revoked"`
	Max     int      `json:"max"`
}

// selfTokenID is the id of the bearer token making the call, empty for a plain session.
func (rt *Router) selfTokenID(r *http.Request) string {
	raw, ok := bearerToken(r)
	if !ok {
		return ""
	}
	rec, err := rt.lookupBearerToken(r.Context(), raw)
	if err != nil {
		return ""
	}
	return rec.ID
}

// handleBulkRevokeTokens handles POST /api/v1/tokens/bulk-revoke, session only like the single revoke.
// A caller sees and changes only their own tokens unless they hold root.
func (rt *Router) handleBulkRevokeTokens(w http.ResponseWriter, r *http.Request) {
	var req bulkRevokeRequest
	if err := decodeJSONBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if (len(req.IDs) == 0) == (req.Filter == nil) {
		writeError(w, http.StatusBadRequest, "pass exactly one of ids or filter")
		return
	}
	callerID, _ := rt.currentSessionUserID(r)
	all, err := rt.libraryListTokens(r.Context(), callerID, rt.callerIsAdmin(r))
	if err != nil {
		rt.internalError(w, "api: bulk revoke: list failed", err)
		return
	}
	targets := rt.bulkRevokeTargets(r, req, all)
	limit := rt.tokenSettingInt(r.Context(), runtimesettings.KeyTokensBulkMax)
	resp := bulkRevokeResponse{DryRun: req.DryRun, Count: len(targets), Sample: []string{}, Max: limit}
	for _, t := range targets[:min(len(targets), bulkSampleSize)] {
		resp.Sample = append(resp.Sample, t.Name)
	}
	if limit > 0 && len(targets) > limit {
		writeJSON(w, http.StatusBadRequest, runtimeSettingError{Error: fmt.Sprintf("%d tokens match, more than tokens.bulk_max (%d): narrow the filter", len(targets), limit), Code: errCodeInvalidValue, Field: "filter"})
		return
	}
	if req.DryRun {
		writeJSON(w, http.StatusOK, resp)
		return
	}
	if req.Filter != nil && (req.ConfirmCount == nil || *req.ConfirmCount != len(targets)) {
		writeJSON(w, http.StatusConflict, resp)
		return
	}
	names := make([]string, 0, bulkAuditNameLimit)
	for _, t := range targets {
		if err := rt.libraryRevokeToken(r.Context(), t.ID); err != nil {
			rt.internalError(w, "api: bulk revoke failed", err, slog.String("token_id", t.ID), slog.Int("revoked", resp.Revoked))
			return
		}
		resp.Revoked++
		if len(names) < bulkAuditNameLimit {
			names = append(names, t.Name)
		}
	}
	rt.recordTokenBulkAudit(r.Context(), r, callerID, resp.Revoked, names, req)
	writeJSON(w, http.StatusOK, resp)
}

// bulkRevokeTargets resolves the request to live tokens, never the caller's own token.
func (rt *Router) bulkRevokeTargets(r *http.Request, req bulkRevokeRequest, all []tokenResource) []tokenResource {
	self := rt.selfTokenID(r)
	now := time.Now()
	window := rt.tokenExpiryWindow(r.Context())
	want := make(map[string]bool, len(req.IDs))
	for _, id := range req.IDs {
		want[id] = true
	}
	out := []tokenResource{}
	for _, t := range all {
		if t.ID == self || t.Status == tokenStatusRevoked {
			continue
		}
		if req.Filter != nil && !req.Filter.matches(t, now, window) {
			continue
		}
		if req.Filter == nil && !want[t.ID] {
			continue
		}
		out = append(out, t)
	}
	return out
}

// recordTokenBulkAudit writes one row with the count, the filter and the first names, never a secret.
func (rt *Router) recordTokenBulkAudit(ctx context.Context, r *http.Request, callerID string, count int, names []string, req bulkRevokeRequest) {
	id, err := store.NewAuditEntryID()
	if err != nil {
		rt.logger.Warn("api: bulk revoke audit id failed", slog.String("error", err.Error()))
		return
	}
	detail, _ := json.Marshal(map[string]any{"count": count, "names": names, "filter": req.Filter, "ids_requested": len(req.IDs)})
	entry := store.AuditEntry{
		ID: id, ActorType: auditActorSession, ActorID: callerID, Ability: AbilityWrite, Method: r.Method, Path: r.URL.Path,
		StatusCode: http.StatusOK, RemoteAddr: clientIP(r), CreatedAt: store.FormatAuditTime(time.Now()),
		ClientKind: clientKindFromUserAgent(r.Header.Get("User-Agent")), Action: auditActionTokensBulkRevoke, Detail: string(detail),
	}
	if err := rt.auditLog.SaveAuditEntry(ctx, entry); err != nil {
		rt.logger.Warn("api: save bulk revoke audit failed", slog.String("error", err.Error()))
	}
}

type tokenPruneRequest struct {
	OlderThanDays *int `json:"older_than_days"`
	DryRun        bool `json:"dry_run"`
}

type tokenPruneResponse struct {
	DryRun  bool `json:"dry_run"`
	Revoked int  `json:"revoked"`
	Expired int  `json:"expired"`
}

// handlePruneTokens handles POST /api/v1/tokens/prune (root): it deletes revoked and expired
// tokens past the window and never a valid one.
func (rt *Router) handlePruneTokens(w http.ResponseWriter, r *http.Request) {
	var req tokenPruneRequest
	if err := decodeJSONBody(r, &req); err != nil || req.OlderThanDays == nil {
		writeError(w, http.StatusBadRequest, "older_than_days is required")
		return
	}
	days := *req.OlderThanDays
	if days < 0 || days > maxTokenCleanupDays {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("older_than_days must be between 0 and %d", maxTokenCleanupDays))
		return
	}
	purger, ok := rt.tokens.(janitor.TokenPurger)
	if !ok {
		writeError(w, http.StatusNotImplemented, "token cleanup is not available on this control plane")
		return
	}
	batch, maxBatches := tokenCleanupBounds()
	resp := tokenPruneResponse{DryRun: req.DryRun}
	var err error
	if resp.Revoked, err = janitor.PurgeTokens(r.Context(), purger, store.TokenPurgeRevoked, days, time.Now(), batch, maxBatches, req.DryRun); err == nil {
		resp.Expired, err = janitor.PurgeTokens(r.Context(), purger, store.TokenPurgeExpired, days, time.Now(), batch, maxBatches, req.DryRun)
	}
	if err != nil {
		rt.internalError(w, "api: token prune failed", err)
		return
	}
	if n := resp.Revoked + resp.Expired; n > 0 && !req.DryRun {
		rt.recordTokenCleanupAudit(r.Context(), janitor.AuditTokensCleanup, fmt.Sprintf(`{"deleted":%d,"trigger":"prune"}`, n))
	}
	writeJSON(w, http.StatusOK, resp)
}
