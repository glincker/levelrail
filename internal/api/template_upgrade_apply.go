package api

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"sort"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/templateupgrade"
)

// Template upgrade knobs, all read from the environment.
const (
	envTemplateUpgradeBackupRequired = "APP_TEMPLATE_UPGRADE_BACKUP_REQUIRED"
	envTemplateUpgradeBackupMaxAge   = "APP_TEMPLATE_UPGRADE_BACKUP_MAX_AGE"
	envTemplateUpgradeCheckInterval  = "APP_TEMPLATE_UPGRADE_CHECK_INTERVAL"
	envTemplateUpgradePoll           = "APP_TEMPLATE_UPGRADE_WATCH_INTERVAL"
	envTemplateUpgradeTimeout        = "APP_TEMPLATE_UPGRADE_WATCH_TIMEOUT"
	envTemplateUpgradeStableFor      = "APP_TEMPLATE_UPGRADE_STABLE_FOR"
	envTemplateUpgradeFailPolls      = "APP_TEMPLATE_UPGRADE_FAIL_POLLS"
)

// Run states of an upgrade.
const (
	upgradeStateWatching       = "watching"
	upgradeStateSucceeded      = "succeeded"
	upgradeStateRolledBack     = "rolled_back"
	upgradeStateRefused        = "refused"
	upgradeStateRollbackFailed = "rollback_failed"
)

// Codes on a refused apply.
const (
	codeUpgradeBlocked      = "upgrade_blocked"
	codeUpgradeUpToDate     = "upgrade_up_to_date"
	codeUpgradeRiskAck      = "upgrade_risk_unacknowledged"
	codeUpgradeBackup       = "upgrade_backup_required"
	codeUpgradeInProgress   = "upgrade_in_progress"
	codeUpgradeUserServices = "upgrade_user_services"
)

type templateUpgradeSettings struct {
	backupRequired bool
	backupMaxAge   time.Duration
	recheck        time.Duration
	poll           time.Duration
	timeout        time.Duration
	stableFor      time.Duration
	failPolls      int
}

func templateUpgradeSettingsFromEnv() templateUpgradeSettings {
	return templateUpgradeSettings{
		backupRequired: envBoolDefault(envTemplateUpgradeBackupRequired, true),
		backupMaxAge:   envDurationOr(envTemplateUpgradeBackupMaxAge, 24*time.Hour),
		recheck:        envDurationOr(envTemplateUpgradeCheckInterval, time.Hour),
		poll:           envDurationOr(envTemplateUpgradePoll, 5*time.Second),
		timeout:        envDurationOr(envTemplateUpgradeTimeout, 10*time.Minute),
		stableFor:      envDurationOr(envTemplateUpgradeStableFor, 30*time.Second),
		failPolls:      envIntDefault(envTemplateUpgradeFailPolls, 6),
	}
}

// templateUpgradeRun is the progress of one apply, readable while it runs.
type templateUpgradeRun struct {
	AppID            string `json:"app_id"`
	State            string `json:"state"`
	Message          string `json:"message,omitempty"`
	FromRevision     int    `json:"from_revision"`
	ToRevision       int    `json:"to_revision"`
	RollbackRevision int    `json:"rollback_revision,omitempty"`
	StartedAt        string `json:"started_at"`
	FinishedAt       string `json:"finished_at,omitempty"`
}

type templateUpgradeRuns struct {
	mu   sync.Mutex
	runs map[string]*templateUpgradeRun
}

func (t *templateUpgradeRuns) start(run templateUpgradeRun) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	if cur := t.runs[run.AppID]; cur != nil && cur.State == upgradeStateWatching {
		return false
	}
	if t.runs == nil {
		t.runs = map[string]*templateUpgradeRun{}
	}
	t.runs[run.AppID] = &run
	return true
}

func (t *templateUpgradeRuns) update(app string, fn func(*templateUpgradeRun)) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if cur := t.runs[app]; cur != nil {
		fn(cur)
	}
}

func (t *templateUpgradeRuns) get(app string) (templateUpgradeRun, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if cur := t.runs[app]; cur != nil {
		return *cur, true
	}
	return templateUpgradeRun{}, false
}

type templateUpgradeRequest struct {
	AcceptRisk bool `json:"accept_risk"`
}

type templateUpgradeRefusal struct {
	Error   string                    `json:"error"`
	Code    string                    `json:"code"`
	Volumes []string                  `json:"volumes,omitempty"`
	Plan    *templateupgrade.Plan     `json:"plan,omitempty"`
	Blocks  []templateupgrade.Blocker `json:"blockers,omitempty"`
}

// handleApplyTemplateUpgrade handles POST /api/v1/apps/{name}/template-upgrade:
// re-plans, refuses on blockers or a stale backup, deploys the merged
// template through the normal compose path and watches the outcome.
func (rt *Router) handleApplyTemplateUpgrade(w http.ResponseWriter, r *http.Request) {
	appID := r.PathValue("name")
	var req templateUpgradeRequest
	if err := decodeJSONBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body: "+err.Error())
		return
	}
	st, e := rt.loadTemplateUpgrade(r, appID)
	if e != nil {
		rt.writeTemplateUpgradeErr(w, appID, e)
		return
	}
	cfg := templateUpgradeSettingsFromEnv()
	plan := st.plan
	switch {
	case !plan.HasChanges:
		writeJSON(w, http.StatusConflict, templateUpgradeRefusal{Error: "the app already matches the template", Code: codeUpgradeUpToDate})
		return
	case len(plan.Blockers) > 0:
		writeJSON(w, http.StatusConflict, templateUpgradeRefusal{Error: "the upgrade has blockers that need manual steps", Code: codeUpgradeBlocked, Blocks: plan.Blockers, Plan: &plan})
		return
	case !plan.Automatic && !req.AcceptRisk:
		writeJSON(w, http.StatusConflict, templateUpgradeRefusal{Error: "the upgrade is high risk and needs accept_risk", Code: codeUpgradeRiskAck, Plan: &plan})
		return
	}
	if cfg.backupRequired {
		stale, err := rt.volumesWithoutFreshBackup(r.Context(), appID, cfg.backupMaxAge)
		if err != nil {
			rt.internalError(w, "api: template upgrade: check backups failed", err, slog.String("app", appID))
			return
		}
		if len(stale) > 0 {
			writeJSON(w, http.StatusConflict, templateUpgradeRefusal{Error: "take a backup of the app volumes first", Code: codeUpgradeBackup, Volumes: stale})
			return
		}
	}
	merged, err := templateupgrade.Merge(st.latest.Compose, st.tpl, plan)
	if errors.Is(err, templateupgrade.ErrUserServices) {
		writeJSON(w, http.StatusConflict, templateUpgradeRefusal{Error: err.Error(), Code: codeUpgradeUserServices})
		return
	}
	if err != nil {
		rt.internalError(w, "api: template upgrade: merge failed", err, slog.String("app", appID))
		return
	}
	run := templateUpgradeRun{AppID: appID, State: upgradeStateWatching, FromRevision: st.latest.Revision, StartedAt: time.Now().UTC().Format(time.RFC3339)}
	if !rt.templateUpgrades.start(run) {
		writeJSON(w, http.StatusConflict, templateUpgradeRefusal{Error: "an upgrade of this app is already running", Code: codeUpgradeInProgress})
		return
	}
	resp, ok := rt.deployComposeBody(w, r, appID, []byte(merged), composeDeployOpts{
		profiles: st.latest.Profiles, source: store.ComposeRevisionSourceTemplate, templateID: st.latest.TemplateID,
	})
	if !ok {
		rt.templateUpgrades.update(appID, func(cur *templateUpgradeRun) {
			cur.State, cur.Message, cur.FinishedAt = upgradeStateRefused, "the deploy was refused, nothing changed", time.Now().UTC().Format(time.RFC3339)
		})
		return
	}
	rt.templateUpgrades.update(appID, func(cur *templateUpgradeRun) { cur.ToRevision = resp.Revision })
	bg := r.Clone(context.WithoutCancel(r.Context()))
	go rt.watchTemplateUpgrade(bg, st, cfg)
	cur, _ := rt.templateUpgrades.get(appID)
	writeJSON(w, http.StatusAccepted, cur)
}

// handleGetTemplateUpgradeRun handles GET /api/v1/apps/{name}/template-upgrade/run.
func (rt *Router) handleGetTemplateUpgradeRun(w http.ResponseWriter, r *http.Request) {
	run, ok := rt.templateUpgrades.get(r.PathValue("name"))
	if !ok {
		writeError(w, http.StatusNotFound, "no template upgrade has run for this app since the control plane started")
		return
	}
	writeJSON(w, http.StatusOK, run)
}

// volumesWithoutFreshBackup lists "service/volume" for every named volume of
// the app that lacks a successful backup newer than maxAge.
func (rt *Router) volumesWithoutFreshBackup(ctx context.Context, appID string, maxAge time.Duration) ([]string, error) {
	services, err := rt.appGroups.ListServicesByApp(ctx, appID)
	if err != nil {
		return nil, err
	}
	var stale []string
	cutoff := time.Now().Add(-maxAge)
	for _, svc := range services {
		for _, v := range svc.Volumes {
			hist, err := rt.serviceVolumeBackupHistory.ListServiceVolumeBackupHistory(ctx, svc.Name, v.Name, 10, nil)
			if err != nil {
				return nil, err
			}
			if !hasFreshBackup(hist, cutoff) {
				stale = append(stale, svc.Name+"/"+v.Name)
			}
		}
	}
	sort.Strings(stale)
	return stale, nil
}

func hasFreshBackup(hist []store.BackupHistory, cutoff time.Time) bool {
	for _, h := range hist {
		if h.Status != store.BackupStatusSucceeded {
			continue
		}
		if t, err := time.Parse(time.RFC3339, h.FinishedAt); err == nil && t.After(cutoff) {
			return true
		}
	}
	return false
}

// watchTemplateUpgrade follows the new revision until it is stable, failed
// or timed out; any failure re-applies the revision that was running before.
func (rt *Router) watchTemplateUpgrade(r *http.Request, st templateUpgradeState, cfg templateUpgradeSettings) {
	appID := st.plan.AppID
	started, _ := rt.templateUpgrades.get(appID)
	since, _ := time.Parse(time.RFC3339, started.StartedAt)
	ctx, cancel := context.WithTimeout(r.Context(), cfg.timeout)
	defer cancel()

	reason := rt.waitForUpgradeOutcome(ctx, appID, since, cfg)
	if reason == "" {
		rt.templateUpgrades.update(appID, func(cur *templateUpgradeRun) {
			cur.State, cur.FinishedAt = upgradeStateSucceeded, time.Now().UTC().Format(time.RFC3339)
		})
		rt.logger.Info("api: template upgrade succeeded", slog.String("app", appID))
		return
	}
	rt.logger.Warn("api: template upgrade failed, rolling back", slog.String("app", appID), slog.String("reason", reason))
	rb := &captureWriter{header: http.Header{}}
	resp, ok := rt.deployComposeBody(rb, r, appID, []byte(st.latest.Compose), composeDeployOpts{
		profiles: st.latest.Profiles, source: store.ComposeRevisionSourceRollback, templateID: st.latest.TemplateID,
	})
	rt.templateUpgrades.update(appID, func(cur *templateUpgradeRun) {
		cur.FinishedAt = time.Now().UTC().Format(time.RFC3339)
		if ok {
			cur.State, cur.RollbackRevision, cur.Message = upgradeStateRolledBack, resp.Revision, reason
			return
		}
		cur.State, cur.Message = upgradeStateRollbackFailed, reason+"; rollback was refused: "+rb.body.String()
	})
}

// waitForUpgradeOutcome returns "" once the stack is running and stable, or
// the reason it is not: a failed deploy attempt, a crash loop or a timeout.
func (rt *Router) waitForUpgradeOutcome(ctx context.Context, appID string, since time.Time, cfg templateUpgradeSettings) string {
	tick := time.NewTicker(cfg.poll)
	defer tick.Stop()
	var bad int
	var stableSince time.Time
	for {
		select {
		case <-ctx.Done():
			return "the stack did not become ready in time"
		case <-tick.C:
		}
		phase, services, err := rt.stackPhase(ctx, appID)
		if err != nil {
			continue
		}
		if name := rt.failedAttemptSince(ctx, services, since); name != "" {
			return "the deploy of " + name + " failed"
		}
		switch phase {
		case composePhaseRunning:
			bad = 0
			if stableSince.IsZero() {
				stableSince = time.Now()
			}
			if time.Since(stableSince) >= cfg.stableFor {
				return ""
			}
		case composePhaseFailed, composePhaseDegraded:
			stableSince = time.Time{}
			bad++
			if bad >= cfg.failPolls {
				return "the stack is crash looping or unhealthy"
			}
		default:
			stableSince, bad = time.Time{}, 0
		}
	}
}

func (rt *Router) stackPhase(ctx context.Context, appID string) (string, []store.DesiredService, error) {
	services, err := rt.appGroups.ListServicesByApp(ctx, appID)
	if err != nil {
		return "", nil, err
	}
	names := make([]string, len(services))
	for i, s := range services {
		names[i] = applicationControllerName(s.Name)
	}
	conds, err := rt.deploys.GetConditionsForControllers(ctx, names)
	if err != nil {
		return "", nil, err
	}
	views := make([]composeServiceStatus, 0, len(services))
	for _, s := range services {
		views = append(views, composeServiceView(appID, s, conds[applicationControllerName(s.Name)]))
	}
	return composeStackPhase(views), services, nil
}

func (rt *Router) failedAttemptSince(ctx context.Context, services []store.DesiredService, since time.Time) string {
	if rt.deployAttempts == nil {
		return ""
	}
	for _, s := range services {
		list, err := rt.deployAttempts.ListDeployAttempts(ctx, s.Name)
		if err != nil || len(list) == 0 {
			continue
		}
		if a := list[0]; a.Status == store.DeployAttemptStatusFailed && !a.StartedAt.Before(since) {
			return s.Name
		}
	}
	return ""
}

// captureWriter lets the rollback reuse the handler-shaped deploy path
// without a live HTTP response.
type captureWriter struct {
	header http.Header
	status int
	body   bytes.Buffer
}

func (c *captureWriter) Header() http.Header { return c.header }
func (c *captureWriter) WriteHeader(s int)   { c.status = s }
func (c *captureWriter) Write(b []byte) (int, error) {
	return c.body.Write(b)
}
