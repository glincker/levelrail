package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/internal/cpbackup"
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
	rollback  SelfUpgradeLauncher
	restore   SelfUpgradeLauncher
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
		c.rollback = func(ctx context.Context, args []string) error { return launchHostCommand(ctx, "rollback", args) }
		c.restore = func(ctx context.Context, args []string) error { return launchHostCommand(ctx, "restore-point", args) }
		if launch != nil {
			c.rollback = launch
			c.restore = launch
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

// hostEnvPassthrough are the non-secret variables the host command needs to
// find the same brand file, data directory and listen address as the service.
var hostEnvPassthrough = []string{
	"APP_DATA_DIR", "APP_BRAND_FILE", "APP_HTTP_ADDR", "APP_INSTALL_VERIFY",
	"APP_UPGRADE_RESTORE_POINTS", "APP_UPGRADE_RESTORE_POINTS_MAX_MB", "APP_UPGRADE_VERIFY_WINDOW",
	"LEVELRAIL_RELEASE_BASE_URL", "LEVELRAIL_INSECURE_MIRROR",
}

// systemdRunArgs builds the systemd-run argv. A transient unit starts with an
// empty environment and cwd "/", so without the working directory and the
// passthrough variables the host command cannot find brand.yaml and exits.
func systemdRunArgs(exe, unit, cwd string, getenv func(string) string, args []string) []string {
	return systemdRunArgsFor("self-upgrade", exe, unit, cwd, getenv, args)
}

func systemdRunArgsFor(sub, exe, unit, cwd string, getenv func(string) string, args []string) []string {
	full := []string{"--collect", "--quiet", "--unit=" + unit}
	if cwd != "" {
		full = append(full, "--working-directory="+cwd)
	}
	for _, k := range hostEnvPassthrough {
		if v := getenv(k); v != "" {
			full = append(full, "--setenv="+k+"="+v)
		}
	}
	full = append(full, exe, sub)
	return append(full, args...)
}

func systemdRunLauncher(ctx context.Context, args []string) error {
	return launchHostCommand(ctx, "self-upgrade", args)
}

func launchHostCommand(ctx context.Context, sub string, args []string) error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("locate own executable: %w", err)
	}
	if resolved, rerr := filepath.EvalSymlinks(exe); rerr == nil {
		exe = resolved
	}
	unit := fmt.Sprintf("%s-%s-%d", filepath.Base(exe), sub, time.Now().Unix())
	cwd, _ := os.Getwd()
	full := systemdRunArgsFor(sub, exe, unit, cwd, os.Getenv, args)
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

func (c *selfUpgradeConfig) expire() {
	c.mu.Lock()
	c.cachedAt = time.Time{}
	c.mu.Unlock()
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
	// SignaturePinned is false until a release signature has been verified
	// on this host; until then upgrades accept checksum-only releases.
	SignaturePinned        bool   `json:"signature_pinned"`
	SignaturePinnedVersion string `json:"signature_pinned_version,omitempty"`
	// Edge is set when the channel's target is the edge container image,
	// which this self-upgrade does not apply.
	Edge *edgeGuidance `json:"edge,omitempty"`
	// Schema fields are omitted when the live database or the target manifest is unreadable.
	DBSchemaVersion     *int `json:"db_schema_version,omitempty"`
	TargetSchemaVersion *int `json:"target_schema_version,omitempty"`
	// PendingMigrations is how many migrations the upgrade will run, when known.
	PendingMigrations *int     `json:"pending_migrations,omitempty"`
	RollbackCommands  []string `json:"rollback_commands,omitempty"`
	// Disk, downtime and backup describe what the upgrade touches, so a dry run is a full report.
	Disk            selfupgrade.DiskReport `json:"disk"`
	DowntimeSeconds int                    `json:"downtime_seconds"`
	Restart         string                 `json:"restart_behaviour"`
	Backup          selfUpgradeBackupPlan  `json:"backup"`
}

// selfUpgradeBackupPlan is the pre-upgrade backup the engine takes first.
type selfUpgradeBackupPlan struct {
	WillTake       bool   `json:"will_take"`
	Dir            string `json:"dir,omitempty"`
	RestoreCommand string `json:"restore_command,omitempty"`
}

const (
	envUpgradeDowntime = "APP_UPGRADE_EST_DOWNTIME_SECONDS"
	// fallbackDowntimeSeconds matches the restart-and-health window of a normal upgrade.
	fallbackDowntimeSeconds = 15
	restartBehaviour        = "the control plane stops and starts once; app containers keep running and agents reconnect when it is back"
)

func (rt *Router) fillPlanHost(plan *selfUpgradePlanResource) {
	plan.DowntimeSeconds = fallbackDowntimeSeconds
	if n, err := strconv.Atoi(os.Getenv(envUpgradeDowntime)); err == nil && n > 0 {
		plan.DowntimeSeconds = n
	}
	plan.Restart = restartBehaviour
	plan.Backup.WillTake = true
	if rt.dataDir == "" {
		return
	}
	var exeSize int64
	if exe, err := os.Executable(); err == nil {
		if fi, err := os.Stat(exe); err == nil {
			exeSize = fi.Size()
		}
	}
	var dbSize int64
	if fi, err := os.Stat(filepath.Join(rt.dataDir, liveDBFile)); err == nil {
		dbSize = fi.Size()
	}
	var free int64
	freeErr := errors.New("data dir free space unavailable")
	if f := rt.dataDirFreeBytes(); f != nil {
		free, freeErr = f()
	}
	plan.Disk = selfupgrade.PlanDisk(free, freeErr, dbSize, exeSize, os.Getenv("APP_UPGRADE_MIN_FREE_BYTES"))
	plan.Backup.Dir = filepath.Join(rt.dataDir, cpbackup.DirName)
	plan.Backup.RestoreCommand = "sudo systemctl stop levelrail && sudo levelrail restore-db " + filepath.Join(plan.Backup.Dir, "NAME")
}

const errEdgeNotApplicable = "edge builds are container images, not release binaries: pull the image instead, or switch the channel to beta or stable"

var selfUpgradeSteps = []string{
	selfupgrade.StepDownload, selfupgrade.StepChecksum, selfupgrade.StepSignature, selfupgrade.StepProbe,
	selfupgrade.StepBackup, selfupgrade.StepMigrationCheck, selfupgrade.StepStop, selfupgrade.StepSwap,
	selfupgrade.StepStart, selfupgrade.StepHealth,
}

// fillPlanSchema adds the schema versions, the migration count and the exact
// rollback commands so an operator can judge an upgrade before running it.
func (rt *Router) fillPlanSchema(ctx context.Context, plan *selfUpgradePlanResource, r upgrade.HistoryRelease) {
	db := rt.liveSchemaVersion(ctx)
	target := rt.releaseHist.schemaFor(ctx, r)
	plan.DBSchemaVersion, plan.TargetSchemaVersion = optInt(db), optInt(target)
	if db >= 0 && target >= 0 {
		if n, err := store.MigrationsBetween(db, target); err == nil {
			plan.PendingMigrations = &n
		}
	}
	plan.RollbackCommands = []string{
		"levelrail-cli upgrade --rollback-plan " + version.Version,
		"sudo levelrail rollback",
	}
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
	rt.fillPlanHost(&plan)
	releases, err := rt.selfUpgrade.releases(ctx)
	if err == nil {
		notes := make([]selfupgrade.ReleaseNotes, 0, len(releases))
		known := false
		for _, r := range releases {
			notes = append(notes, selfupgrade.ReleaseNotes{Tag: r.Tag, Body: r.Body})
			known = known || r.Tag == target
			if r.Tag == target && rt.releaseHist != nil {
				rt.fillPlanSchema(ctx, &plan, r)
			}
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
	if rt.dataDir != "" {
		pin, pinned, _ := selfupgrade.ReadPin(rt.dataDir)
		plan.SignaturePinned, plan.SignaturePinnedVersion = pinned, pin.Version
	}
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
		if g := buildEdgeGuidance(release); g != nil {
			writeJSON(w, http.StatusOK, edgePlan(release.Tag, g))
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
	// SkipBackup upgrades even when the pre-upgrade backup fails.
	SkipBackup bool `json:"skip_backup,omitempty"`
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
	if isEdgeTag(req.Target) {
		writeError(w, http.StatusBadRequest, errEdgeNotApplicable)
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
	if id := rt.runningSelfUpgradeID(); id != "" {
		writeJSON(w, http.StatusConflict, map[string]any{"error": "an upgrade is already running", "running_attempt_id": id})
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
	if req.SkipBackup {
		args = append(args, "--skip-backup")
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

type selfUpgradeLogItem struct {
	At   string `json:"at"`
	Step string `json:"step"`
	Line string `json:"line"`
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
	CurrentStep string                `json:"current_step"`
	Log         []selfUpgradeLogItem  `json:"log"`
	StartedAt   string                `json:"started_at"`
	FinishedAt  string                `json:"finished_at"`
	// DatabaseRestored and RolledBackMinutes say whether a rollback put the
	// database snapshot back and how many minutes of changes it discarded.
	DatabaseRestored  bool `json:"database_restored"`
	RolledBackMinutes int  `json:"rolled_back_minutes"`
	// Failure types the failed step and names the buttons that can fix it.
	Failure *selfupgrade.StepFailure `json:"failure,omitempty"`
}

// handleSelfUpgradeAttempts handles GET /api/v1/updates/self-upgrade/attempts.
// It first folds any host journal into the database so a run in progress or
// a rolled-back one shows up without waiting for the next boot.
func (rt *Router) handleSelfUpgradeAttempts(w http.ResponseWriter, r *http.Request) {
	if rt.selfUpgrade == nil {
		writeJSON(w, http.StatusOK, map[string]any{"attempts": []selfUpgradeAttemptItem{}})
		return
	}
	live := map[string]selfupgrade.Attempt{}
	if rt.dataDir != "" {
		j := selfupgrade.NewJournal(rt.dataDir)
		if _, err := selfupgrade.ImportJournal(r.Context(), j, rt.selfUpgrade.store); err != nil {
			rt.logger.Warn("api: import self-upgrade journal failed", slog.String("error", err.Error()))
		}
		if list, err := j.List(); err == nil {
			for _, a := range list {
				live[a.ID] = a
			}
		}
	}
	rows, err := rt.selfUpgrade.store.ListSelfUpgradeAttempts(r.Context(), selfUpgradeListLimit)
	if err != nil {
		rt.internalError(w, "api: list self-upgrade attempts failed", err)
		return
	}
	out := make([]selfUpgradeAttemptItem, 0, len(rows))
	for _, row := range rows {
		item := toSelfUpgradeItem(row)
		if a, ok := live[row.ID]; ok {
			item.CurrentStep = a.Current
			for _, l := range a.Log {
				item.Log = append(item.Log, selfUpgradeLogItem{At: l.At.UTC().Format(time.RFC3339), Step: l.Step, Line: l.Line})
			}
		}
		item.Failure = rt.attemptFailure(item)
		out = append(out, item)
	}
	writeJSON(w, http.StatusOK, map[string]any{"attempts": out})
}

func toSelfUpgradeItem(row store.SelfUpgradeAttempt) selfUpgradeAttemptItem {
	item := selfUpgradeAttemptItem{
		ID: row.ID, FromVersion: row.FromVersion, ToVersion: row.ToVersion, FromSchema: row.FromSchema, ToSchema: row.ToSchema,
		Initiator: row.Initiator, Outcome: row.Outcome, FailedStep: row.FailedStep, Error: row.Error, BackupName: row.BackupName,
		Acked: []string{}, Steps: []selfUpgradeStepItem{}, Log: []selfUpgradeLogItem{}, StartedAt: row.StartedAt, FinishedAt: row.FinishedAt,
		DatabaseRestored: row.DatabaseRestored, RolledBackMinutes: max(row.RolledBackMinutes, 0),
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

func edgePlan(tag string, g *edgeGuidance) selfUpgradePlanResource {
	cmd := g.RecreateCommand
	if g.Installable {
		cmd = g.PullCommand + " && " + g.RecreateCommand
	}
	return selfUpgradePlanResource{
		CurrentVersion: version.Version, TargetVersion: tag, Breaking: []selfUpgradeBreaking{}, Steps: []string{},
		CannotApplyWhy: errEdgeNotApplicable, Command: cmd, Edge: g,
	}
}
