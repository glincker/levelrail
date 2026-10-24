package api

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/GLINCKER/levelrail/internal/janitor"
	"github.com/GLINCKER/levelrail/internal/runtimesettings"
	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	envTokenCleanupBatch      = "APP_TOKEN_CLEANUP_BATCH"       //nolint:gosec // env var name
	envTokenCleanupMaxBatches = "APP_TOKEN_CLEANUP_MAX_BATCHES" //nolint:gosec // env var name
	defaultTokenCleanupBatch  = 200
	defaultTokenCleanupBatchN = 25
	maxTokenCleanupDays       = 3650
)

type tokenCleanupRequest struct {
	// OlderThanDays 0 removes every revoked token.
	OlderThanDays int `json:"older_than_days"`
}

type tokenCleanupResponse struct {
	Deleted int `json:"deleted"`
}

// tokenCleanupBounds reads the per-run batch limits from the environment.
func tokenCleanupBounds() (batch, maxBatches int) {
	return envIntDefault(envTokenCleanupBatch, defaultTokenCleanupBatch), envIntDefault(envTokenCleanupMaxBatches, defaultTokenCleanupBatchN)
}

// tokenWindows reads the live retention settings for the scheduled stage.
func (rt *Router) tokenWindows(ctx context.Context) janitor.TokenWindows {
	return janitor.TokenWindows{
		Revoked: rt.runtimeSettings.Int(ctx, runtimesettings.KeyRevokedTokenDays),
		Expired: rt.runtimeSettings.Int(ctx, runtimesettings.KeyExpiredTokenDays),
	}
}

// recordTokenCleanupAudit writes one row per cleanup run. The detail is a
// count and trigger only, never a token id, name or hash.
func (rt *Router) recordTokenCleanupAudit(ctx context.Context, action, detail string) {
	id, err := store.NewAuditEntryID()
	if err != nil {
		rt.logger.Warn("api: token cleanup audit id failed", slog.String("error", err.Error()))
		return
	}
	entry := store.AuditEntry{
		ID: id, ActorType: auditActorSystem, ActorID: "token-cleanup", ActorName: "token cleanup",
		Ability: AbilityRoot, Method: http.MethodDelete, Path: "/api/v1/auth/tokens", StatusCode: http.StatusOK,
		CreatedAt: store.FormatAuditTime(time.Now()), ClientKind: "system", Action: action, Detail: detail,
	}
	if err := rt.auditLog.SaveAuditEntry(ctx, entry); err != nil {
		rt.logger.Warn("api: save token cleanup audit failed", slog.String("error", err.Error()))
	}
}

// handleCleanupTokens handles POST /api/v1/auth/tokens/cleanup (AbilityRoot).
// It removes revoked tokens older than older_than_days now, whatever the
// scheduled setting says; an active token is never touched.
func (rt *Router) handleCleanupTokens(w http.ResponseWriter, r *http.Request) {
	var req tokenCleanupRequest
	if err := decodeJSONBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.OlderThanDays < 0 || req.OlderThanDays > maxTokenCleanupDays {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("older_than_days must be between 0 and %d", maxTokenCleanupDays))
		return
	}
	purger, ok := rt.tokens.(janitor.TokenPurger)
	if !ok {
		writeError(w, http.StatusNotImplemented, "token cleanup is not available on this control plane")
		return
	}
	batch, maxBatches := tokenCleanupBounds()
	n, err := janitor.PurgeTokens(r.Context(), purger, store.TokenPurgeRevoked, req.OlderThanDays, time.Now(), batch, maxBatches, false)
	if err != nil {
		rt.internalError(w, "api: token cleanup failed", err)
		return
	}
	if n > 0 {
		rt.recordTokenCleanupAudit(r.Context(), janitor.AuditTokensCleanup, fmt.Sprintf(`{"deleted":%d,"trigger":"manual"}`, n))
	}
	writeJSON(w, http.StatusOK, tokenCleanupResponse{Deleted: n})
}
