package api

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/GLINCKER/levelrail/internal/reconcile/database"
	"github.com/GLINCKER/levelrail/internal/store"
)

// MajorUpgradeStore is the store surface the major upgrade routes read.
type MajorUpgradeStore interface {
	ListMajorUpgrades(ctx context.Context, databaseName string) ([]store.MajorUpgrade, error)
	GetMajorUpgrade(ctx context.Context, id string) (store.MajorUpgrade, error)
	ListMajorUpgradeSnapshots(ctx context.Context) ([]store.MajorUpgrade, error)
}

// MajorUpgrader runs guarded major version upgrades (internal/backup's
// MajorUpgradeRunner).
type MajorUpgrader interface {
	RunMajorUpgrade(ctx context.Context, historyID, databaseName, toVersion string) error
	Rollback(ctx context.Context, upgradeID string) error
	DiscardSnapshot(ctx context.Context, upgradeID string) error
}

// WithMajorUpgrader enables the major upgrade routes; without it they return 501.
func WithMajorUpgrader(u MajorUpgrader) Option {
	return func(rt *Router) { rt.majorUpgrader = u }
}

type majorUpgradeRequest struct {
	Version string `json:"version"`
	Confirm string `json:"confirm"`
}

type confirmRequest struct {
	Confirm string `json:"confirm"`
}

type majorUpgradeResource struct {
	ID             string `json:"id"`
	DatabaseName   string `json:"database_name"`
	FromVersion    string `json:"from_version"`
	ToVersion      string `json:"to_version"`
	Status         string `json:"status"`
	Phase          string `json:"phase,omitempty"`
	SnapshotVolume string `json:"snapshot_volume,omitempty"`
	Error          string `json:"error,omitempty"`
	StartedAt      string `json:"started_at"`
	FinishedAt     string `json:"finished_at,omitempty"`
}

func toMajorUpgradeResource(u store.MajorUpgrade) majorUpgradeResource {
	return majorUpgradeResource{
		ID: u.ID, DatabaseName: u.DatabaseName, FromVersion: u.FromVersion, ToVersion: u.ToVersion,
		Status: u.Status, Phase: u.Phase, SnapshotVolume: u.SnapshotVolume, Error: u.Error,
		StartedAt: u.StartedAt, FinishedAt: u.FinishedAt,
	}
}

// checkMajorUpgrade validates a major upgrade request against the database's
// current state and returns why it is refused, or "".
func checkMajorUpgrade(d *store.DesiredDatabase, toVersion string) string {
	if d.Engine != store.EnginePostgres {
		return "guarded major upgrades are only available for postgres; other engines: back up and restore into a new database"
	}
	if !databaseVersionPattern.MatchString(toVersion) {
		return "version must be an image tag such as \"17\""
	}
	from, fromOK := majorVersion(d.Version)
	to, toOK := majorVersion(toVersion)
	if !fromOK || !toOK {
		return fmt.Sprintf("cannot compare %q with %q: use numeric versions", d.Version, toVersion)
	}
	if err := database.ValidateEngineVersion(d.Engine, toVersion); err != nil {
		return err.Error()
	}
	_, fromVariant, _ := database.ParsePgvectorVersion(d.Version)
	_, toVariant, _ := database.ParsePgvectorVersion(toVersion)
	if fromVariant && !toVariant {
		return "moving from the pgvector image back to plain Postgres is not supported: the dump references the vector extension"
	}
	switch {
	case to == from && !fromVariant && toVariant:
	case to == from:
		return "the major version is unchanged: use PUT /version for a minor or patch change"
	case to < from:
		return "downgrading across majors is not supported; roll back an earlier upgrade instead"
	}
	if d.PITREnabled {
		return "point-in-time restore is enabled: its base backups and WAL cannot be replayed on another major version. Disable it first, upgrade, then enable it and take a new base backup"
	}
	return ""
}

// handleMajorUpgrade handles POST /api/v1/databases/{name}/major-upgrade.
// The database is offline for the duration; the old data is kept as a
// rollback snapshot. AbilityRoot like every restore.
func (rt *Router) handleMajorUpgrade(w http.ResponseWriter, r *http.Request) {
	if rt.majorUpgrader == nil {
		writeError(w, http.StatusNotImplemented, "major upgrades are not configured on this control plane (no master key set)")
		return
	}
	name := r.PathValue("name")
	d, ok := rt.loadDatabaseForRunner(w, r, name, "api: major upgrade: load database failed")
	if !ok {
		return
	}
	var req majorUpgradeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Confirm != name {
		writeError(w, http.StatusBadRequest, "confirm must equal the database name: the database is offline during an upgrade")
		return
	}
	if msg := checkMajorUpgrade(d, req.Version); msg != "" {
		writeError(w, http.StatusConflict, msg)
		return
	}
	existing, err := rt.majorUpgrades.ListMajorUpgrades(r.Context(), name)
	if err != nil {
		rt.internalError(w, "api: major upgrade: list history failed", err, slog.String("name", name))
		return
	}
	for _, u := range existing {
		if u.Status == store.BackupStatusRunning {
			writeError(w, http.StatusConflict, "an upgrade of this database is already running: "+u.ID)
			return
		}
	}
	if err := rt.checkNoRestoreInFlight(r.Context(), name); err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}

	id, err := randomMajorUpgradeID()
	if err != nil {
		rt.internalError(w, "api: major upgrade: generate id failed", err)
		return
	}
	go func() { //nolint:gosec // deliberately not r.Context(): cancelled when the handler returns, same reasoning as handleTriggerRestore
		if err := rt.majorUpgrader.RunMajorUpgrade(context.Background(), id, name, req.Version); err != nil {
			rt.logger.Error("api: major upgrade failed", slog.String("error", err.Error()), slog.String("id", id), slog.String("database", name))
		}
	}()
	writeJSON(w, http.StatusAccepted, majorUpgradeResource{ID: id, DatabaseName: name, FromVersion: d.Version, ToVersion: req.Version, Status: store.BackupStatusRunning, Phase: "preflight"})
}

// checkNoRestoreInFlight refuses an upgrade while a restore of the same
// database is running, since both stop and rewrite its data volume.
func (rt *Router) checkNoRestoreInFlight(ctx context.Context, name string) error {
	history, err := rt.restoreHistory.ListRestoreHistory(ctx, name)
	if err != nil {
		return fmt.Errorf("check running restores: %w", err)
	}
	for _, h := range history {
		if h.Status == store.BackupStatusRunning {
			return errors.New("a restore of this database is running: wait for it to finish")
		}
	}
	return nil
}

// handleListMajorUpgrades handles GET /api/v1/databases/{name}/major-upgrades.
func (rt *Router) handleListMajorUpgrades(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if _, ok := rt.loadDatabaseForRunner(w, r, name, "api: list major upgrades: load database failed"); !ok {
		return
	}
	list, err := rt.majorUpgrades.ListMajorUpgrades(r.Context(), name)
	if err != nil {
		rt.internalError(w, "api: list major upgrades failed", err, slog.String("name", name))
		return
	}
	out := make([]majorUpgradeResource, 0, len(list))
	for _, u := range list {
		out = append(out, toMajorUpgradeResource(u))
	}
	writeJSON(w, http.StatusOK, out)
}

func (rt *Router) loadUpgradeForDatabase(w http.ResponseWriter, r *http.Request) (store.MajorUpgrade, bool) {
	name := r.PathValue("name")
	u, err := rt.majorUpgrades.GetMajorUpgrade(r.Context(), r.PathValue("id"))
	if errors.Is(err, store.ErrMajorUpgradeNotFound) || (err == nil && u.DatabaseName != name) {
		writeError(w, http.StatusNotFound, "upgrade not found")
		return store.MajorUpgrade{}, false
	}
	if err != nil {
		rt.internalError(w, "api: load major upgrade failed", err, slog.String("name", name))
		return store.MajorUpgrade{}, false
	}
	return u, true
}

// handleRollbackMajorUpgrade handles
// POST /api/v1/databases/{name}/major-upgrades/{id}/rollback: restores the
// pre-upgrade data. Writes made since the upgrade are lost.
func (rt *Router) handleRollbackMajorUpgrade(w http.ResponseWriter, r *http.Request) {
	if rt.majorUpgrader == nil {
		writeError(w, http.StatusNotImplemented, "major upgrades are not configured on this control plane (no master key set)")
		return
	}
	name := r.PathValue("name")
	u, ok := rt.loadUpgradeForDatabase(w, r)
	if !ok {
		return
	}
	var req confirmRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Confirm != name {
		writeError(w, http.StatusBadRequest, "confirm must equal the database name: rolling back discards every write since the upgrade")
		return
	}
	if u.Status == store.BackupStatusRunning {
		writeError(w, http.StatusConflict, "the upgrade is still running")
		return
	}
	if u.SnapshotVolume == "" {
		writeError(w, http.StatusConflict, "this upgrade has no rollback snapshot (discarded, or never taken)")
		return
	}
	go func() { //nolint:gosec // see handleMajorUpgrade
		if err := rt.majorUpgrader.Rollback(context.Background(), u.ID); err != nil {
			rt.logger.Error("api: major upgrade rollback failed", slog.String("error", err.Error()), slog.String("id", u.ID), slog.String("database", name))
		}
	}()
	writeJSON(w, http.StatusAccepted, toMajorUpgradeResource(u))
}

// handleDiscardMajorUpgradeSnapshot handles
// DELETE /api/v1/databases/{name}/major-upgrades/{id}/snapshot: frees the
// disk held by the rollback snapshot, ending the ability to roll back.
func (rt *Router) handleDiscardMajorUpgradeSnapshot(w http.ResponseWriter, r *http.Request) {
	if rt.majorUpgrader == nil {
		writeError(w, http.StatusNotImplemented, "major upgrades are not configured on this control plane (no master key set)")
		return
	}
	u, ok := rt.loadUpgradeForDatabase(w, r)
	if !ok {
		return
	}
	if u.Status == store.BackupStatusRunning {
		writeError(w, http.StatusConflict, "the upgrade is still running")
		return
	}
	if err := rt.majorUpgrader.DiscardSnapshot(r.Context(), u.ID); err != nil {
		rt.internalError(w, "api: discard major upgrade snapshot failed", err, slog.String("id", u.ID))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func randomMajorUpgradeID() (string, error) {
	buf := make([]byte, 9)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("api: generate major upgrade id: %w", err)
	}
	return "mu_" + base64.RawURLEncoding.EncodeToString(buf), nil
}
