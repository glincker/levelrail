package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/internal/selfupgrade"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/version"
	"github.com/GLINCKER/levelrail/kit/semver"
	"github.com/GLINCKER/levelrail/kit/upgrade"
)

const (
	selfUpgradeNotesLimit = 50
	selfUpgradeListLimit  = 50
	selfUpgradeNotesTTL   = 10 * time.Minute
	maxAckIDs             = 64
)

// SelfUpgradeStore is what the self-upgrade routes persist and read.
type SelfUpgradeStore interface {
	UpsertSelfUpgradeAttempt(ctx context.Context, a store.SelfUpgradeAttempt) error
	ListSelfUpgradeAttempts(ctx context.Context, limit int) ([]store.SelfUpgradeAttempt, error)
}

// SelfUpgradeLauncher starts the host command. Tests replace it.
type SelfUpgradeLauncher func(ctx context.Context, args []string) error

type selfUpgradeConfig struct {
	store     SelfUpgradeStore
	launch    SelfUpgradeLauncher
	notes     func(ctx context.Context) ([]upgrade.HistoryRelease, error)
	canLaunch func() (bool, string)
	mu        sync.Mutex
	cached    []upgrade.HistoryRelease
	cachedAt  time.Time
}

// WithSelfUpgrade enables the self-upgrade plan, start and attempts routes.
// launch may be nil to use systemd-run on a root host.
func WithSelfUpgrade(s SelfUpgradeStore, launch SelfUpgradeLauncher) Option {
	return func(rt *Router) {
		c := &selfUpgradeConfig{store: s, launch: launch}
		c.notes = func(ctx context.Context) ([]upgrade.HistoryRelease, error) {
			return upgrade.FetchHistory(ctx, githubRepo, "all", selfUpgradeNotesLimit)
		}
		c.canLaunch = hostCanLaunch
		if launch == nil {
			c.launch = systemdRunLauncher
		}
		rt.selfUpgrade = c
	}
}

func hostCanLaunch() (bool, string) {
	if os.Geteuid() != 0 {
		return false, "the control plane does not run as root on this host"
	}
	if _, err := os.Stat("/run/systemd/system"); err != nil {
		return false, "systemd is not the init system on this host"
	}
	if _, err := exec.LookPath("systemd-run"); err != nil {
		return false, "systemd-run is not installed"
	}
	return true, ""
}

func systemdRunLauncher(ctx context.Context, args []string) error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("locate own executable: %w", err)
	}
	if resolved, rerr := filepath.EvalSymlinks(exe); rerr == nil {
		exe = resolved
	}
	unit := fmt.Sprintf("%s-self-upgrade-%d", filepath.Base(exe), time.Now().Unix())
	full := append([]string{"--collect", "--quiet", "--unit=" + unit, exe, "self-upgrade"}, args...)
	// A transient unit, so stopping the control plane unit does not kill the
	// upgrade that is replacing it.
	out, err := exec.CommandContext(ctx, "systemd-run", full...).CombinedOutput() //nolint:gosec // fixed program, argv validated by the caller
	if err != nil {
		return fmt.Errorf("systemd-run: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func (c *selfUpgradeConfig) releases(ctx context.Context) ([]upgrade.HistoryRelease, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.cached != nil && time.Since(c.cachedAt) < selfUpgradeNotesTTL {
		return c.cached, nil
	}
	ctx, cancel := context.WithTimeout(ctx, historyFetchTimeout)
	defer cancel()
	list, err := c.notes(ctx)
	if err != nil {
		if c.cached != nil {
			return c.cached, nil
		}
		return nil, err
	}
	c.cached, c.cachedAt = list, time.Now()
	return list, nil
}

type selfUpgradeBreaking struct {
	ID          string `json:"id"`
	Version     string `json:"version"`
	Summary     string `json:"summary"`
	RequiresAck bool   `json:"requires_ack"`
}

type selfUpgradePlanResource struct {
	CurrentVersion string                `json:"current_version"`
	TargetVersion  string                `json:"target_version"`
	Breaking       []selfUpgradeBreaking `json:"breaking"`
	NotesAvailable bool                  `json:"notes_available"`
	CanApply       bool                  `json:"can_apply"`
	CannotApplyWhy string                `json:"cannot_apply_reason,omitempty"`
	Command        string                `json:"command"`
	Steps          []string              `json:"steps"`
}

var selfUpgradeSteps = []string{
	selfupgrade.StepDownload, selfupgrade.StepChecksum, selfupgrade.StepSignature, selfupgrade.StepProbe,
	selfupgrade.StepBackup, selfupgrade.StepMigrationCheck, selfupgrade.StepStop, selfupgrade.StepSwap,
	selfupgrade.StepStart, selfupgrade.StepHealth,
}

func (rt *Router) buildSelfUpgradePlan(ctx context.Context, target string) (selfUpgradePlanResource, int, string) {
	plan := selfUpgradePlanResource{CurrentVersion: version.Version, TargetVersion: target, Breaking: []selfUpgradeBreaking{}, Steps: selfUpgradeSteps}
	if !selfupgrade.ValidTag(target) {
		return plan, http.StatusBadRequest, "target must be a release tag such as v1.2.3"
	}
	if cmp, ok := semver.Compare(target, version.Version); ok && cmp <= 0 {
		return plan, http.StatusConflict, "target is not newer than the running version; use rollback to go back"
	}
	plan.Command = "sudo levelrail self-upgrade --to " + target
	releases, err := rt.selfUpgrade.releases(ctx)
	if err == nil {
		notes := make([]selfupgrade.ReleaseNotes, 0, len(releases))
		known := false
		for _, r := range releases {
			notes = append(notes, selfupgrade.ReleaseNotes{Tag: r.Tag, Body: r.Body})
			known = known || r.Tag == target
		}
		if !known {
			return plan, http.StatusNotFound, "that release was not found"
		}
		plan.NotesAvailable = true
		for _, b := range selfupgrade.BreakingBetween(notes, version.Version, target) {
			plan.Breaking = append(plan.Breaking, selfUpgradeBreaking{ID: b.ID, Version: b.Version, Summary: b.Summary, RequiresAck: b.RequiresAck})
		}
	}
	plan.CanApply, plan.CannotApplyWhy = rt.selfUpgrade.canLaunch()
	if len(plan.Breaking) > 0 {
		ids := make([]string, 0, len(plan.Breaking))
		for _, b := range plan.Breaking {
			if b.RequiresAck {
				ids = append(ids, b.ID)
			}
		}
		if len(ids) > 0 {
			plan.Command += " --ack " + strings.Join(ids, ",")
		}
	}
	return plan, http.StatusOK, ""
}

// handleSelfUpgradePlan handles GET /api/v1/updates/self-upgrade/plan?target=:
// the breaking changes between the running version and target, and whether
// this host can apply the upgrade from the dashboard.
func (rt *Router) handleSelfUpgradePlan(w http.ResponseWriter, r *http.Request) {
	if rt.selfUpgrade == nil {
		writeError(w, http.StatusNotImplemented, "self-upgrade is not configured")
		return
	}
	target := strings.TrimSpace(r.URL.Query().Get("target"))
	if target == "" {
		release, known := rt.latestReleaseForCurrentChannel(r.Context())
		if !known || release == nil {
			writeError(w, http.StatusBadGateway, "the latest release could not be fetched")
			return
		}
		target = release.Tag
	}
	plan, status, msg := rt.buildSelfUpgradePlan(r.Context(), target)
	if status != http.StatusOK {
		writeError(w, status, msg)
		return
	}
	writeJSON(w, http.StatusOK, plan)
}

type selfUpgradeStartRequest struct {
	Target string   `json:"target"`
	Ack    []string `json:"ack"`
}

// handleSelfUpgradeStart handles POST /api/v1/updates/self-upgrade. Root only.
// It validates the target and the acknowledgements, then hands the work to a
// transient host unit: the control plane being replaced cannot do it itself.
func (rt *Router) handleSelfUpgradeStart(w http.ResponseWriter, r *http.Request) {
	if rt.selfUpgrade == nil {
		writeError(w, http.StatusNotImplemented, "self-upgrade is not configured")
		return
	}
	var req selfUpgradeStartRequest
	if err := decodeJSONBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if len(req.Ack) > maxAckIDs {
		writeError(w, http.StatusBadRequest, "too many acknowledgements")
		return
	}
	plan, status, msg := rt.buildSelfUpgradePlan(r.Context(), strings.TrimSpace(req.Target))
	if status != http.StatusOK {
		writeError(w, status, msg)
		return
	}
	if !plan.NotesAvailable {
		writeError(w, http.StatusBadGateway, "release notes could not be fetched, so breaking changes cannot be checked. Run the command on the host with --skip-notes-check if you accept that")
		return
	}
	if !plan.CanApply {
		writeJSON(w, http.StatusConflict, map[string]any{"error": "this host cannot apply the upgrade from the dashboard: " + plan.CannotApplyWhy, "command": plan.Command})
		return
	}
	breaking := make([]selfupgrade.Breaking, 0, len(plan.Breaking))
	for _, b := range plan.Breaking {
		breaking = append(breaking, selfupgrade.Breaking{ID: b.ID, RequiresAck: b.RequiresAck})
	}
	if missing := selfupgrade.MissingAcks(breaking, req.Ack); len(missing) > 0 {
		ids := make([]string, 0, len(missing))
		for _, m := range missing {
			ids = append(ids, m.ID)
		}
		writeJSON(w, http.StatusConflict, map[string]any{"error": "breaking changes need acknowledgement", "missing_ack": ids})
		return
	}
	if running, _ := selfupgrade.NewJournal(rt.dataDir).Running(); len(running) > 0 {
		writeError(w, http.StatusConflict, "an earlier upgrade did not finish; run self-upgrade --recover on the host")
		return
	}
	_, actorID, actorName, ok := rt.currentActor(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "cannot resolve the requesting user")
		return
	}
	initiator := "dashboard:" + sanitizeInitiator(firstNonEmpty(actorName, actorID))
	args := []string{"--to", plan.TargetVersion, "--yes", "--initiator", initiator, "--json"}
	if len(req.Ack) > 0 {
		args = append(args, "--ack", strings.Join(req.Ack, ","))
	}
	if err := rt.selfUpgrade.launch(r.Context(), args); err != nil {
		rt.logger.Error("api: start self-upgrade failed", slog.String("target", plan.TargetVersion), slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "could not start the upgrade on the host")
		return
	}
	rt.logger.Info("api: self-upgrade started", slog.String("from", version.Version), slog.String("to", plan.TargetVersion), slog.String("initiator", initiator))
	writeJSON(w, http.StatusAccepted, map[string]any{"status": "started", "target_version": plan.TargetVersion})
}

func sanitizeInitiator(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '.', r == '_', r == '@', r == '-':
			b.WriteRune(r)
		}
	}
	out := b.String()
	if len(out) > 48 {
		out = out[:48]
	}
	if out == "" {
		return "unknown"
	}
	return out
}

type selfUpgradeStepItem struct {
	Name       string `json:"name"`
	Status     string `json:"status"`
	Detail     string `json:"detail"`
	At         string `json:"at"`
	DurationMS int64  `json:"duration_ms"`
}

type selfUpgradeAttemptItem struct {
	ID          string                `json:"id"`
	FromVersion string                `json:"from_version"`
	ToVersion   string                `json:"to_version"`
	FromSchema  int                   `json:"from_schema"`
	ToSchema    int                   `json:"to_schema"`
	Initiator   string                `json:"initiator"`
	Outcome     string                `json:"outcome"`
	FailedStep  string                `json:"failed_step"`
	Error       string                `json:"error"`
	BackupName  string                `json:"backup_name"`
	Acked       []string              `json:"acked"`
	Steps       []selfUpgradeStepItem `json:"steps"`
	StartedAt   string                `json:"started_at"`
	FinishedAt  string                `json:"finished_at"`
}

// handleSelfUpgradeAttempts handles GET /api/v1/updates/self-upgrade/attempts.
// It first folds any host journal into the database so a run in progress or
// a rolled-back one shows up without waiting for the next boot.
func (rt *Router) handleSelfUpgradeAttempts(w http.ResponseWriter, r *http.Request) {
	if rt.selfUpgrade == nil {
		writeJSON(w, http.StatusOK, map[string]any{"attempts": []selfUpgradeAttemptItem{}})
		return
	}
	if rt.dataDir != "" {
		if _, err := selfupgrade.ImportJournal(r.Context(), selfupgrade.NewJournal(rt.dataDir), rt.selfUpgrade.store); err != nil {
			rt.logger.Warn("api: import self-upgrade journal failed", slog.String("error", err.Error()))
		}
	}
	rows, err := rt.selfUpgrade.store.ListSelfUpgradeAttempts(r.Context(), selfUpgradeListLimit)
	if err != nil {
		rt.logger.Error("api: list self-upgrade attempts failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	out := make([]selfUpgradeAttemptItem, 0, len(rows))
	for _, row := range rows {
		out = append(out, toSelfUpgradeItem(row))
	}
	writeJSON(w, http.StatusOK, map[string]any{"attempts": out})
}

func toSelfUpgradeItem(row store.SelfUpgradeAttempt) selfUpgradeAttemptItem {
	item := selfUpgradeAttemptItem{
		ID: row.ID, FromVersion: row.FromVersion, ToVersion: row.ToVersion, FromSchema: row.FromSchema, ToSchema: row.ToSchema,
		Initiator: row.Initiator, Outcome: row.Outcome, FailedStep: row.FailedStep, Error: row.Error, BackupName: row.BackupName,
		Acked: []string{}, Steps: []selfUpgradeStepItem{}, StartedAt: row.StartedAt, FinishedAt: row.FinishedAt,
	}
	_ = json.Unmarshal([]byte(row.AckedJSON), &item.Acked)
	var steps []selfupgrade.StepRecord
	if json.Unmarshal([]byte(row.StepsJSON), &steps) == nil {
		for _, s := range steps {
			item.Steps = append(item.Steps, selfUpgradeStepItem{
				Name: s.Name, Status: s.Status, Detail: s.Detail, At: s.At.UTC().Format(time.RFC3339), DurationMS: s.DurationMS,
			})
		}
	}
	return item
}
