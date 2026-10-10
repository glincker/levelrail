package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/upgradehistory"
	"github.com/GLINCKER/levelrail/internal/version"
)

const (
	upgradeHistoryLimit     = 200
	agentVersionChangeLimit = 100
)

// UpgradeHistoryStore is what the upgrade history routes need.
// ListUnacknowledgedUpgrades is the query the attention center consumes.
type UpgradeHistoryStore interface {
	ListUpgradeHistory(ctx context.Context, limit int) ([]store.UpgradeHistoryEntry, error)
	AcknowledgeUpgrade(ctx context.Context, id, byType, byID, byName string, now time.Time) (e store.UpgradeHistoryEntry, changed bool, err error)
	ListNodeAgentVersionChanges(ctx context.Context, limit int) ([]store.NodeAgentVersionChange, error)
}

// WithUpgradeHistory enables GET /api/v1/updates/history and the ack route.
// Without one the history is empty and acknowledging answers 501.
func WithUpgradeHistory(s UpgradeHistoryStore) Option {
	return func(rt *Router) { rt.upgradeHistory = s }
}

type upgradeHistoryItem struct {
	ID                string  `json:"id"`
	Kind              string  `json:"kind"`
	FromVersion       string  `json:"from_version"`
	ToVersion         string  `json:"to_version"`
	Channel           string  `json:"channel"`
	SchemaBefore      *int    `json:"schema_before"`
	SchemaAfter       *int    `json:"schema_after"`
	SchemaMoved       bool    `json:"schema_moved"`
	OccurredAt        string  `json:"occurred_at"`
	Initiator         string  `json:"initiator"`
	Method            string  `json:"method"`
	BackupName        string  `json:"backup_name"`
	Health            string  `json:"health"`
	Notes             string  `json:"notes"`
	NotesState        string  `json:"notes_state"`
	ReleaseURL        string  `json:"release_url"`
	CompareURL        string  `json:"compare_url"`
	Acknowledged      bool    `json:"acknowledged"`
	AckedBy           string  `json:"acked_by"`
	AckedAt           *string `json:"acked_at"`
	RollbackAvailable bool    `json:"rollback_available"`
}

type agentVersionChangeItem struct {
	NodeID      string `json:"node_id"`
	NodeName    string `json:"node_name"`
	FromVersion string `json:"from_version"`
	ToVersion   string `json:"to_version"`
	ObservedAt  string `json:"observed_at"`
}

type upgradeHistoryResource struct {
	CurrentVersion string                   `json:"current_version"`
	Unacknowledged int                      `json:"unacknowledged"`
	Entries        []upgradeHistoryItem     `json:"entries"`
	AgentChanges   []agentVersionChangeItem `json:"agent_changes"`
}

func (rt *Router) upgradeHistoryItem(e store.UpgradeHistoryEntry, retained map[string]bool) upgradeHistoryItem {
	links := upgradehistory.BuildLinks(rt.brand.RepoURL, e.FromVersion, e.ToVersion)
	item := upgradeHistoryItem{
		ID: e.ID, Kind: e.Kind, FromVersion: e.FromVersion, ToVersion: e.ToVersion, Channel: e.Channel,
		SchemaBefore: optInt(e.SchemaBefore), SchemaAfter: optInt(e.SchemaAfter),
		SchemaMoved: e.SchemaBefore >= 0 && e.SchemaAfter >= 0 && e.SchemaBefore != e.SchemaAfter,
		OccurredAt:  e.OccurredAt.UTC().Format(time.RFC3339), Initiator: e.Initiator, Method: e.Method,
		BackupName: e.BackupName, Health: e.Health, Notes: e.Notes, NotesState: e.NotesState,
		ReleaseURL: links.ReleaseURL, CompareURL: links.CompareURL,
		Acknowledged: e.Acknowledged(), AckedBy: e.AckedByName,
		RollbackAvailable: e.FromVersion != "" && e.FromVersion != version.Version && retained[e.FromVersion],
	}
	if e.AckedAt != nil {
		s := e.AckedAt.UTC().Format(time.RFC3339)
		item.AckedAt = &s
	}
	return item
}

// handleUpgradeHistory handles GET /api/v1/updates/history: every recorded
// control plane version transition, newest first. Read-only.
func (rt *Router) handleUpgradeHistory(w http.ResponseWriter, r *http.Request) {
	out := upgradeHistoryResource{
		CurrentVersion: version.Version, Entries: []upgradeHistoryItem{}, AgentChanges: []agentVersionChangeItem{},
	}
	if rt.upgradeHistory == nil {
		writeJSON(w, http.StatusOK, out)
		return
	}
	ctx := r.Context()
	entries, err := rt.upgradeHistory.ListUpgradeHistory(ctx, upgradeHistoryLimit)
	if err != nil {
		rt.logger.Error("api: list upgrade history failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	retained := map[string]bool{}
	for v := range rt.retainedByVersion() {
		retained[v] = true
	}
	for _, e := range entries {
		item := rt.upgradeHistoryItem(e, retained)
		if !item.Acknowledged {
			out.Unacknowledged++
		}
		out.Entries = append(out.Entries, item)
	}
	changes, err := rt.upgradeHistory.ListNodeAgentVersionChanges(ctx, agentVersionChangeLimit)
	if err != nil {
		rt.logger.Warn("api: list node agent versions failed", slog.String("error", err.Error()))
	}
	for _, c := range changes {
		out.AgentChanges = append(out.AgentChanges, agentVersionChangeItem{
			NodeID: c.NodeID, NodeName: c.NodeName, FromVersion: c.FromVersion, ToVersion: c.ToVersion,
			ObservedAt: c.ObservedAt.UTC().Format(time.RFC3339),
		})
	}
	writeJSON(w, http.StatusOK, out)
}

// handleAckUpgrade handles POST /api/v1/updates/history/{id}/ack. The audit
// middleware records the actor and path of this write.
func (rt *Router) handleAckUpgrade(w http.ResponseWriter, r *http.Request) {
	if rt.upgradeHistory == nil {
		writeError(w, http.StatusNotImplemented, "upgrade history is not configured")
		return
	}
	actorType, actorID, actorName, ok := rt.currentActor(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "cannot resolve the acknowledging user")
		return
	}
	id := r.PathValue("id")
	e, changed, err := rt.upgradeHistory.AcknowledgeUpgrade(r.Context(), id, actorType, actorID, actorName, time.Now())
	if errors.Is(err, store.ErrUpgradeHistoryNotFound) {
		writeError(w, http.StatusNotFound, "upgrade history entry not found")
		return
	}
	if err != nil {
		rt.logger.Error("api: acknowledge upgrade failed", slog.String("id", id), slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if changed {
		rt.auditUpgradeAck(r.Context(), r, actorType, actorID, actorName, e)
	}
	writeJSON(w, http.StatusOK, rt.upgradeHistoryItem(e, map[string]bool{}))
}

// auditUpgradeAck writes a descriptive audit entry next to the generic one
// the request middleware records for this write.
func (rt *Router) auditUpgradeAck(ctx context.Context, r *http.Request, actorType, actorID, actorName string, e store.UpgradeHistoryEntry) {
	id, err := store.NewAuditEntryID()
	if err != nil {
		rt.logger.Warn("api: upgrade ack audit id failed", slog.String("error", err.Error()))
		return
	}
	err = rt.auditLog.SaveAuditEntry(ctx, store.AuditEntry{
		ID: id, ActorType: actorType, ActorID: actorID, ActorName: actorName, Ability: AbilityWrite,
		Method: r.Method, Path: upgradehistory.AuditActionAcked + ": " + e.FromVersion + " -> " + e.ToVersion + " (id " + e.ID + ")",
		StatusCode: http.StatusOK, RemoteAddr: clientIP(r), CreatedAt: store.FormatAuditTime(time.Now()),
		ClientKind: clientKindFromUserAgent(r.Header.Get("User-Agent")),
	})
	if err != nil {
		rt.logger.Warn("api: upgrade ack audit failed", slog.String("id", e.ID), slog.String("error", err.Error()))
	}
}
