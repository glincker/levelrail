package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/GLINCKER/levelrail/internal/alerting"
	"github.com/GLINCKER/levelrail/internal/dbupgrade"
	"github.com/GLINCKER/levelrail/internal/store"
)

// DatabaseUpgrader is the upgrade advisor, policy store and runner
// (internal/dbupgrade.Manager).
type DatabaseUpgrader interface {
	Overview(ctx context.Context, name string) (dbupgrade.Overview, error)
	EffectivePolicy(ctx context.Context, name string) (dbupgrade.Policy, bool, error)
	SetPolicy(ctx context.Context, name string, p dbupgrade.Policy, inherit bool, by string) error
	UpgradeNow(ctx context.Context, name, version, by string) (store.DBUpgradeRun, error)
	Summary(ctx context.Context, visible func(string) bool) ([]dbupgrade.SummaryItem, error)
}

// WithDatabaseUpgrader enables the database upgrade routes; without it they return 501.
func WithDatabaseUpgrader(u DatabaseUpgrader) Option {
	return func(rt *Router) { rt.dbUpgrader = u }
}

type dbUpgradePolicyResource struct {
	AutoUpgrade           string   `json:"auto_upgrade"`
	WindowCron            string   `json:"window_cron"`
	WindowDurationSeconds int64    `json:"window_duration_seconds"`
	WindowTimezone        string   `json:"window_timezone"`
	BackupBefore          bool     `json:"backup_before"`
	VerifyAfter           bool     `json:"verify_after"`
	RevertOnFailure       bool     `json:"revert_on_failure"`
	Notify                []string `json:"notify"`
	Inherited             bool     `json:"inherited"`
}

func toDBUpgradePolicyResource(p dbupgrade.Policy, inherited bool) dbUpgradePolicyResource {
	notify := p.Notify
	if notify == nil {
		notify = []string{}
	}
	return dbUpgradePolicyResource{
		AutoUpgrade: p.AutoUpgrade, WindowCron: p.WindowCron, WindowDurationSeconds: int64(p.WindowDuration / time.Second),
		WindowTimezone: p.WindowTimezone, BackupBefore: p.BackupBefore, VerifyAfter: p.VerifyAfter,
		RevertOnFailure: p.RevertOnFailure, Notify: notify, Inherited: inherited,
	}
}

type dbUpgradeRunResource struct {
	ID              string            `json:"id"`
	DatabaseName    string            `json:"database_name"`
	Engine          string            `json:"engine"`
	FromVersion     string            `json:"from_version"`
	ToVersion       string            `json:"to_version"`
	Kind            string            `json:"kind"`
	Source          string            `json:"source"`
	State           string            `json:"state"`
	Phase           string            `json:"phase,omitempty"`
	VerifyAfter     bool              `json:"verify_after"`
	RevertOnFailure bool              `json:"revert_on_failure"`
	BackupID        string            `json:"backup_id,omitempty"`
	VerificationID  string            `json:"verification_id,omitempty"`
	FromImageDigest string            `json:"from_image_digest,omitempty"`
	SnapshotVolume  string            `json:"snapshot_volume,omitempty"`
	RevertPath      string            `json:"revert_path,omitempty"`
	Reason          string            `json:"reason,omitempty"`
	RequestedBy     string            `json:"requested_by,omitempty"`
	Timings         map[string]string `json:"timings"`
	CreatedAt       string            `json:"created_at"`
	FinishedAt      string            `json:"finished_at,omitempty"`
}

func toDBUpgradeRunResource(r store.DBUpgradeRun) dbUpgradeRunResource {
	tm := r.Timings
	if tm == nil {
		tm = map[string]string{}
	}
	return dbUpgradeRunResource{
		ID: r.ID, DatabaseName: r.DatabaseName, Engine: r.Engine, FromVersion: r.FromVersion, ToVersion: r.ToVersion,
		Kind: r.Kind, Source: r.Source, State: r.State, Phase: r.Phase, VerifyAfter: r.VerifyAfter,
		RevertOnFailure: r.RevertOnFailure, BackupID: r.BackupID, VerificationID: r.VerificationID,
		FromImageDigest: r.FromImageDigest, SnapshotVolume: r.SnapshotVolume, RevertPath: r.RevertPath,
		Reason: r.Reason, RequestedBy: r.RequestedBy, Timings: tm, CreatedAt: r.CreatedAt, FinishedAt: r.FinishedAt,
	}
}

type dbUpgradesResource struct {
	Database   string                  `json:"database"`
	Engine     string                  `json:"engine"`
	Version    string                  `json:"version"`
	Advice     dbupgrade.Advice        `json:"advice"`
	Policy     dbUpgradePolicyResource `json:"policy"`
	WindowOpen bool                    `json:"window_open"`
	NextWindow string                  `json:"next_window,omitempty"`
	NextTarget *dbupgrade.Target       `json:"next_target,omitempty"`
	Blockers   []string                `json:"blockers"`
	Active     *dbUpgradeRunResource   `json:"active,omitempty"`
	History    []dbUpgradeRunResource  `json:"history"`
}

func (rt *Router) upgraderOr501(w http.ResponseWriter) bool {
	if rt.dbUpgrader == nil {
		writeError(w, http.StatusNotImplemented, "database upgrades are not configured on this control plane")
		return false
	}
	return true
}

// writeUpgradeError maps the upgrader's typed errors onto status codes.
func (rt *Router) writeUpgradeError(w http.ResponseWriter, err error, logContext string) {
	switch {
	case errors.Is(err, dbupgrade.ErrNotFound):
		writeError(w, http.StatusNotFound, "database not found")
	case errors.Is(err, dbupgrade.ErrInvalid):
		writeError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, dbupgrade.ErrConflict):
		writeError(w, http.StatusConflict, err.Error())
	default:
		rt.internalError(w, logContext, err)
	}
}

// handleGetDatabaseUpgrades handles GET /api/v1/databases/{name}/upgrades:
// the advisor's targets, the effective policy and window, and run history.
func (rt *Router) handleGetDatabaseUpgrades(w http.ResponseWriter, r *http.Request) {
	if !rt.upgraderOr501(w) {
		return
	}
	ov, err := rt.dbUpgrader.Overview(r.Context(), r.PathValue("name"))
	if err != nil {
		rt.writeUpgradeError(w, err, "api: database upgrades: overview failed")
		return
	}
	out := dbUpgradesResource{
		Database: ov.Database, Engine: ov.Engine, Version: ov.Version, Advice: ov.Advice,
		Policy: toDBUpgradePolicyResource(ov.Policy, ov.PolicyInherited), WindowOpen: ov.WindowOpen,
		NextTarget: ov.NextTarget, Blockers: ov.Blockers, History: make([]dbUpgradeRunResource, 0, len(ov.History)),
	}
	if out.Blockers == nil {
		out.Blockers = []string{}
	}
	if !ov.NextWindow.IsZero() {
		out.NextWindow = ov.NextWindow.UTC().Format(time.RFC3339)
	}
	if ov.Active != nil {
		a := toDBUpgradeRunResource(*ov.Active)
		out.Active = &a
	}
	for _, h := range ov.History {
		out.History = append(out.History, toDBUpgradeRunResource(h))
	}
	writeJSON(w, http.StatusOK, out)
}

type putUpgradePolicyRequest struct {
	Inherit               bool     `json:"inherit"`
	AutoUpgrade           string   `json:"auto_upgrade"`
	WindowCron            string   `json:"window_cron"`
	WindowDurationSeconds int64    `json:"window_duration_seconds"`
	WindowTimezone        string   `json:"window_timezone"`
	BackupBefore          *bool    `json:"backup_before"`
	VerifyAfter           *bool    `json:"verify_after"`
	RevertOnFailure       *bool    `json:"revert_on_failure"`
	Notify                []string `json:"notify"`
}

func boolOr(p *bool, def bool) bool {
	if p == nil {
		return def
	}
	return *p
}

func (req putUpgradePolicyRequest) policy() dbupgrade.Policy {
	auto := req.AutoUpgrade
	if auto == "" {
		auto = store.DBAutoUpgradeOff
	}
	return dbupgrade.Policy{
		AutoUpgrade: auto, WindowCron: req.WindowCron, WindowDuration: time.Duration(req.WindowDurationSeconds) * time.Second,
		WindowTimezone: req.WindowTimezone, BackupBefore: boolOr(req.BackupBefore, true), VerifyAfter: boolOr(req.VerifyAfter, true),
		RevertOnFailure: boolOr(req.RevertOnFailure, true), Notify: req.Notify,
	}
}

// checkNotifyChannels refuses channel ids that do not exist.
func (rt *Router) checkNotifyChannels(ctx context.Context, ids []string) (string, error) {
	if len(ids) == 0 || rt.notificationChannels == nil {
		return "", nil
	}
	for _, id := range ids {
		if _, err := rt.notificationChannels.GetNotificationChannel(ctx, id); err != nil {
			if errors.Is(err, alerting.ErrNotificationChannelNotFound) {
				return "notification channel " + id + " does not exist", nil
			}
			return "", err
		}
	}
	return "", nil
}

func (rt *Router) putUpgradePolicy(w http.ResponseWriter, r *http.Request, name string) bool {
	var req putUpgradePolicyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return false
	}
	if msg, err := rt.checkNotifyChannels(r.Context(), req.Notify); err != nil {
		rt.internalError(w, "api: upgrade policy: check channels failed", err)
		return false
	} else if msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return false
	}
	if err := rt.dbUpgrader.SetPolicy(r.Context(), name, req.policy(), req.Inherit, rt.checkedByFromRequest(r)); err != nil {
		rt.writeUpgradeError(w, err, "api: upgrade policy: save failed")
		return false
	}
	return true
}

// handlePutDatabaseUpgradePolicy handles PUT /api/v1/databases/{name}/upgrade-policy.
// {"inherit": true} drops the database's own policy.
func (rt *Router) handlePutDatabaseUpgradePolicy(w http.ResponseWriter, r *http.Request) {
	if !rt.upgraderOr501(w) {
		return
	}
	name := r.PathValue("name")
	if !rt.putUpgradePolicy(w, r, name) {
		return
	}
	rt.writeEffectivePolicy(w, r, name)
}

func (rt *Router) writeEffectivePolicy(w http.ResponseWriter, r *http.Request, name string) {
	p, inherited, err := rt.dbUpgrader.EffectivePolicy(r.Context(), name)
	if err != nil {
		rt.internalError(w, "api: upgrade policy: reload failed", err, slog.String("name", name))
		return
	}
	writeJSON(w, http.StatusOK, toDBUpgradePolicyResource(p, inherited))
}

// handleGetPlatformUpgradePolicy handles GET /api/v1/settings/database-upgrades.
func (rt *Router) handleGetPlatformUpgradePolicy(w http.ResponseWriter, r *http.Request) {
	if !rt.upgraderOr501(w) {
		return
	}
	rt.writeEffectivePolicy(w, r, store.DBUpgradePlatformDefault)
}

// handlePutPlatformUpgradePolicy handles PUT /api/v1/settings/database-upgrades.
func (rt *Router) handlePutPlatformUpgradePolicy(w http.ResponseWriter, r *http.Request) {
	if !rt.upgraderOr501(w) {
		return
	}
	if !rt.putUpgradePolicy(w, r, store.DBUpgradePlatformDefault) {
		return
	}
	rt.writeEffectivePolicy(w, r, store.DBUpgradePlatformDefault)
}

type upgradeNowRequest struct {
	Version string `json:"version"`
	Confirm string `json:"confirm"`
}

// handleDatabaseUpgradeNow handles POST /api/v1/databases/{name}/upgrade-now:
// the scheduled runner's exact steps, started immediately. confirm must
// equal the database name, since the database restarts.
func (rt *Router) handleDatabaseUpgradeNow(w http.ResponseWriter, r *http.Request) {
	if !rt.upgraderOr501(w) {
		return
	}
	name := r.PathValue("name")
	var req upgradeNowRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Confirm != name {
		writeError(w, http.StatusBadRequest, "confirm must equal the database name: the database restarts during the upgrade")
		return
	}
	if !databaseVersionPattern.MatchString(req.Version) {
		writeError(w, http.StatusBadRequest, "version must be an image tag such as \"16.4\"")
		return
	}
	run, err := rt.dbUpgrader.UpgradeNow(r.Context(), name, req.Version, rt.checkedByFromRequest(r))
	if err != nil {
		rt.writeUpgradeError(w, err, "api: upgrade now failed")
		return
	}
	writeJSON(w, http.StatusAccepted, toDBUpgradeRunResource(run))
}

type dbUpgradeSummaryItem struct {
	Database    string   `json:"database"`
	Engine      string   `json:"engine"`
	Version     string   `json:"version"`
	Support     string   `json:"support"`
	EOL         string   `json:"eol,omitempty"`
	Security    bool     `json:"security"`
	Advisories  []string `json:"advisories,omitempty"`
	Available   int      `json:"available"`
	ActiveState string   `json:"active_state,omitempty"`
	LastState   string   `json:"last_state,omitempty"`
	LastReason  string   `json:"last_reason,omitempty"`
}

type dbUpgradeSummaryResource struct {
	Items         []dbUpgradeSummaryItem `json:"items"`
	SecurityCount int                    `json:"security_count"`
	EOLCount      int                    `json:"eol_count"`
}

func (rt *Router) upgradeSummary(r *http.Request) (dbUpgradeSummaryResource, error) {
	visible, err := rt.databaseVisibilityFilter(r)
	if err != nil {
		return dbUpgradeSummaryResource{}, err
	}
	items, err := rt.dbUpgrader.Summary(r.Context(), visible)
	if err != nil {
		return dbUpgradeSummaryResource{}, err
	}
	out := dbUpgradeSummaryResource{Items: make([]dbUpgradeSummaryItem, 0, len(items))}
	for _, it := range items {
		row := dbUpgradeSummaryItem{Database: it.Database, Engine: it.Engine, Version: it.Version, Support: it.Support,
			EOL: it.EOL, Security: it.Security, Advisories: it.Advisories, Available: it.Available}
		if it.Active != nil {
			row.ActiveState = it.Active.State
		}
		if it.Last != nil {
			row.LastState, row.LastReason = it.Last.State, it.Last.Reason
		}
		if it.Security {
			out.SecurityCount++
		}
		if it.Support == dbupgrade.SupportEOL {
			out.EOLCount++
		}
		out.Items = append(out.Items, row)
	}
	return out, nil
}

// handleDatabaseUpgradeSummary handles GET /api/v1/databases/upgrade-summary,
// limited to the databases the caller may see.
func (rt *Router) handleDatabaseUpgradeSummary(w http.ResponseWriter, r *http.Request) {
	if !rt.upgraderOr501(w) {
		return
	}
	out, err := rt.upgradeSummary(r)
	if err != nil {
		rt.internalError(w, "api: database upgrade summary failed", err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// SetDatabaseUpgrader is WithDatabaseUpgrader for wiring after NewRouter.
func (rt *Router) SetDatabaseUpgrader(u DatabaseUpgrader) { rt.dbUpgrader = u }
