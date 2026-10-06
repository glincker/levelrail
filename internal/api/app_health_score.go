package api

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/alerting"
	"github.com/GLINCKER/levelrail/internal/store"
)

// Health score statuses. One shared set of constants, referenced by
// every category computation below and by the CLI's own rendering, so
// "pass"/"warn"/"fail" is never retyped as a bare string literal in two
// places that could silently drift.
const (
	HealthScoreStatusPass = "pass"
	HealthScoreStatusWarn = "warn"
	HealthScoreStatusFail = "fail"
)

// healthScoreCategory is one category's verdict: a status plus the
// single reason an operator needs, never a vague percentage. Each
// category's own compute function below owns picking Status/Reason from
// real store/alerting reads, nothing invented.
type healthScoreCategory struct {
	Key    string `json:"key"`
	Label  string `json:"label"`
	Status string `json:"status"`
	Reason string `json:"reason"`
}

// appHealthScoreResource is GET /api/v1/apps/{name}/health-score's wire
// shape: Status is the worst of Categories' own statuses, the same
// "one False outranks everything" rule summarizeAppConditions already
// applies to reconcile conditions.
type appHealthScoreResource struct {
	AppName    string                `json:"app_name"`
	Status     string                `json:"status"`
	Categories []healthScoreCategory `json:"categories"`
	ComputedAt time.Time             `json:"computed_at"`
}

// worstStatus folds a's own status into the running worst seen so far,
// fail outranking warn outranking pass, the same ranking
// overallHealthStatus applies across whole categories.
func worstStatus(a, b string) string {
	rank := map[string]int{HealthScoreStatusPass: 0, HealthScoreStatusWarn: 1, HealthScoreStatusFail: 2}
	if rank[b] > rank[a] {
		return b
	}
	return a
}

func overallHealthStatus(categories []healthScoreCategory) string {
	status := HealthScoreStatusPass
	for _, c := range categories {
		status = worstStatus(status, c.Status)
	}
	return status
}

// handleGetAppHealthScore handles GET /api/v1/apps/{name}/health-score:
// a live, read-only synthesis of signals this control plane already
// tracks separately for one app (deploys, crashloop, TLS, secrets,
// backups, health checks, alerting) into four pass/warn/fail
// categories. Computed fresh on every call, no cache and no new
// reconciler loop: a plain aggregation query, not AI.
func (rt *Router) handleGetAppHealthScore(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	ctx := r.Context()

	svc, err := rt.apps.GetDesiredService(ctx, name)
	if errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return
	}
	if err != nil {
		rt.internalError(w, "api: get app health score: load app", err)
		return
	}

	categories := []healthScoreCategory{
		rt.healthScoreDeploy(ctx, name),
		rt.healthScoreSecurity(ctx, *svc),
		rt.healthScoreResilience(ctx, *svc),
		rt.healthScoreObservability(ctx, name),
	}

	writeJSON(w, http.StatusOK, appHealthScoreResource{
		AppName:    name,
		Status:     overallHealthStatus(categories),
		Categories: categories,
		ComputedAt: time.Now().UTC(),
	})
}

// recentDeployAttemptWindow bounds how many of an app's most recent
// deploy attempts healthScoreDeploy's success-rate reads, the same
// "recent, not all-time" scope a health signal needs: an app with one
// bad deploy three months ago and twenty clean ones since is healthy
// today, not flagged forever.
const recentDeployAttemptWindow = 5

// healthScoreDeploy covers deploy success rate over the most recent
// finished attempts, plus whether a crashloop alert rule is currently
// firing for this app (alerting.Rule.Firing, persisted by the evaluator
// via UpdateState, read here with no need for the live in-memory
// RestartTracker). A firing crashloop outranks deploy history: a service
// stuck restarting right now matters more than yesterday's clean
// deploys.
func (rt *Router) healthScoreDeploy(ctx context.Context, name string) healthScoreCategory {
	cat := healthScoreCategory{Key: "deploy", Label: "Deploy health"}

	if rt.alertRules != nil {
		rules, err := rt.alertRules.ListRulesForResource(ctx, resourceIDForApp(name))
		if err != nil {
			rt.logger.Warn("api: health score: list rules for crashloop check failed", slog.String("app", name), slog.String("error", err.Error()))
		}
		for _, rl := range rules {
			if rl.Kind == alerting.KindCrashloop && rl.Firing {
				cat.Status = HealthScoreStatusFail
				cat.Reason = "app is currently crashlooping (restart threshold exceeded)"
				return cat
			}
		}
	}

	if rt.deployAttempts == nil {
		cat.Status = HealthScoreStatusWarn
		cat.Reason = "deploy history is not available on this control plane"
		return cat
	}
	attempts, err := rt.deployAttempts.ListDeployAttempts(ctx, name)
	if err != nil {
		rt.logger.Warn("api: health score: list deploy attempts failed", slog.String("app", name), slog.String("error", err.Error()))
		cat.Status = HealthScoreStatusWarn
		cat.Reason = "could not read deploy history"
		return cat
	}

	var finished []store.DeployAttempt
	for _, a := range attempts {
		if a.Status == store.DeployAttemptStatusSucceeded || a.Status == store.DeployAttemptStatusFailed {
			finished = append(finished, a)
		}
		if len(finished) == recentDeployAttemptWindow {
			break
		}
	}

	if len(finished) == 0 {
		cat.Status = HealthScoreStatusWarn
		cat.Reason = "no finished deploy attempts recorded yet"
		return cat
	}

	succeeded := 0
	for _, a := range finished {
		if a.Status == store.DeployAttemptStatusSucceeded {
			succeeded++
		}
	}

	switch {
	case finished[0].Status == store.DeployAttemptStatusFailed:
		cat.Status = HealthScoreStatusFail
		cat.Reason = "most recent deploy attempt failed"
	case succeeded < len(finished):
		cat.Status = HealthScoreStatusWarn
		cat.Reason = fmt.Sprintf("%d of the last %d deploy attempts failed", len(finished)-succeeded, len(finished))
	default:
		cat.Status = HealthScoreStatusPass
		cat.Reason = fmt.Sprintf("last %d deploy attempt(s) all succeeded", len(finished))
	}
	return cat
}

// healthScoreSecurity covers TLS cert status for this app's own domains
// (alerting.ListCertificates, the identical computation GET
// /api/v1/certificates and a kind=cert_expiry rule both already use) and
// whether every env var the app declared { secret: true, required: true }
// actually has a stored value (rt.secrets.Exists), the same check
// internal/deploy.Pipeline's validateEnv runs before a build-triggered
// deploy. Worst of the two findings wins.
func (rt *Router) healthScoreSecurity(ctx context.Context, svc store.DesiredService) healthScoreCategory {
	cat := healthScoreCategory{Key: "security", Label: "Security"}

	tlsStatus, tlsReason := rt.healthScoreTLS(ctx, svc.Domains)
	secretsStatus, secretsReason := rt.healthScoreSecrets(ctx, svc)

	cat.Status = worstStatus(tlsStatus, secretsStatus)
	switch {
	case tlsReason == "":
		cat.Reason = secretsReason
	case secretsReason == "":
		cat.Reason = tlsReason
	default:
		cat.Reason = tlsReason + "; " + secretsReason
	}
	return cat
}

func (rt *Router) healthScoreTLS(ctx context.Context, domains []string) (status, reason string) {
	if len(domains) == 0 {
		return HealthScoreStatusPass, "no custom domains configured"
	}

	warningWindow := rt.certExpiryWarningWindow
	if warningWindow <= 0 {
		warningWindow = alerting.DefaultCertExpiryWarningWindow
	}
	infos, err := alerting.ListCertificates(ctx, rt.certs, warningWindow, time.Now(), rt.logger)
	if err != nil {
		rt.logger.Warn("api: health score: list certificates failed", slog.String("error", err.Error()))
		return HealthScoreStatusWarn, "could not read certificate status"
	}

	byDomain := map[string]alerting.CertInfo{}
	for _, info := range infos {
		byDomain[strings.ToLower(info.Domain)] = info
		for _, san := range info.SANs {
			if _, exists := byDomain[strings.ToLower(san)]; !exists {
				byDomain[strings.ToLower(san)] = info
			}
		}
	}

	var expired, expiring, unmonitored []string
	for _, d := range domains {
		info, ok := byDomain[strings.ToLower(d)]
		if !ok {
			unmonitored = append(unmonitored, d)
			continue
		}
		switch info.Status {
		case "expired":
			expired = append(expired, d)
		case "expiring_soon":
			expiring = append(expiring, d)
		}
	}

	switch {
	case len(expired) > 0:
		return HealthScoreStatusFail, fmt.Sprintf("TLS certificate expired for %s", strings.Join(expired, ", "))
	case len(expiring) > 0:
		return HealthScoreStatusWarn, fmt.Sprintf("TLS certificate expiring soon for %s", strings.Join(expiring, ", "))
	case len(unmonitored) > 0:
		return HealthScoreStatusWarn, fmt.Sprintf("no issued certificate found yet for %s", strings.Join(unmonitored, ", "))
	default:
		return HealthScoreStatusPass, fmt.Sprintf("TLS healthy for %d domain(s)", len(domains))
	}
}

func (rt *Router) healthScoreSecrets(ctx context.Context, svc store.DesiredService) (status, reason string) {
	var required []string
	for _, ref := range svc.SecretEnv {
		if ref.Required {
			required = append(required, ref.Name)
		}
	}
	if len(required) == 0 {
		return HealthScoreStatusPass, ""
	}
	if rt.secrets == nil {
		return HealthScoreStatusWarn, fmt.Sprintf("secrets manager not configured; %d required secret(s) declared", len(required))
	}

	var missing []string
	for _, envKey := range required {
		exists, err := rt.secrets.Exists(ctx, svc.Name, envKey)
		if err != nil {
			rt.logger.Warn("api: health score: check secret existence failed", slog.String("app", svc.Name), slog.String("env_key", envKey), slog.String("error", err.Error()))
			continue
		}
		if !exists {
			missing = append(missing, envKey)
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		return HealthScoreStatusFail, fmt.Sprintf("missing required secret(s): %s", strings.Join(missing, ", "))
	}
	return HealthScoreStatusPass, fmt.Sprintf("all %d required secret(s) are set", len(required))
}

// healthScoreResilience covers whether a readiness or liveness probe is
// configured (store.DesiredService.Health, internal/spec.Health's
// storage home) and, for every named volume, whether a backup schedule
// exists and its most recent attempt succeeded
// (ServiceVolumeBackupScheduleStore/ServiceVolumeBackupHistoryStore,
// the same two reads the volume backup panel already uses).
func (rt *Router) healthScoreResilience(ctx context.Context, svc store.DesiredService) healthScoreCategory {
	cat := healthScoreCategory{Key: "resilience", Label: "Resilience"}

	hasHealthCheck := svc.Health != nil && (svc.Health.Readiness != nil || svc.Health.Liveness != nil)

	if len(svc.Volumes) == 0 {
		if !hasHealthCheck {
			cat.Status = HealthScoreStatusWarn
			cat.Reason = "no readiness or liveness health check configured"
			return cat
		}
		cat.Status = HealthScoreStatusPass
		cat.Reason = "health checks configured; no persistent volumes to back up"
		return cat
	}

	var unscheduled, neverRun, failed []string
	for _, vol := range svc.Volumes {
		_, err := rt.serviceVolumeBackupSchedule.GetServiceVolumeBackupSchedule(ctx, svc.Name, vol.Name)
		if errors.Is(err, store.ErrServiceVolumeBackupNotFound) {
			unscheduled = append(unscheduled, vol.Name)
			continue
		}
		if err != nil {
			rt.logger.Warn("api: health score: get volume backup schedule failed", slog.String("app", svc.Name), slog.String("volume", vol.Name), slog.String("error", err.Error()))
			continue
		}

		history, err := rt.serviceVolumeBackupHistory.ListServiceVolumeBackupHistory(ctx, svc.Name, vol.Name, 1, nil)
		if err != nil {
			rt.logger.Warn("api: health score: list volume backup history failed", slog.String("app", svc.Name), slog.String("volume", vol.Name), slog.String("error", err.Error()))
			continue
		}
		if len(history) == 0 {
			neverRun = append(neverRun, vol.Name)
			continue
		}
		if history[0].Status == store.BackupStatusFailed {
			failed = append(failed, vol.Name)
		}
	}

	switch {
	case len(failed) > 0:
		cat.Status = HealthScoreStatusFail
		cat.Reason = fmt.Sprintf("most recent backup failed for volume(s): %s", strings.Join(failed, ", "))
	case len(unscheduled) > 0:
		cat.Status = HealthScoreStatusWarn
		cat.Reason = fmt.Sprintf("no backup schedule configured for volume(s): %s", strings.Join(unscheduled, ", "))
	case len(neverRun) > 0:
		cat.Status = HealthScoreStatusWarn
		cat.Reason = fmt.Sprintf("backup scheduled but never run for volume(s): %s", strings.Join(neverRun, ", "))
	case !hasHealthCheck:
		cat.Status = HealthScoreStatusWarn
		cat.Reason = "backups healthy, but no readiness or liveness health check configured"
	default:
		cat.Status = HealthScoreStatusPass
		cat.Reason = "health checks configured and all volume backups healthy"
	}
	return cat
}

// healthScoreObservability covers whether this app has any alert rule
// at all, and whether at least one enabled rule actually has somewhere
// to notify (a channel_id, or a legacy notify_url/notify_kind pair): a
// rule with neither fires silently into nothing, the exact "bolted-on
// observability" gap this project set out to avoid in section 4.8's own
// rationale.
func (rt *Router) healthScoreObservability(ctx context.Context, name string) healthScoreCategory {
	cat := healthScoreCategory{Key: "observability", Label: "Observability"}

	if rt.alertRules == nil {
		cat.Status = HealthScoreStatusWarn
		cat.Reason = "alerting is not configured on this control plane"
		return cat
	}

	rules, err := rt.alertRules.ListRulesForResource(ctx, resourceIDForApp(name))
	if err != nil {
		rt.logger.Warn("api: health score: list alert rules failed", slog.String("app", name), slog.String("error", err.Error()))
		cat.Status = HealthScoreStatusWarn
		cat.Reason = "could not read alert rules"
		return cat
	}

	if len(rules) == 0 {
		cat.Status = HealthScoreStatusWarn
		cat.Reason = "no alert rules configured for this app"
		return cat
	}

	wired := 0
	enabled := 0
	for _, rl := range rules {
		if !rl.Enabled {
			continue
		}
		enabled++
		if rl.ChannelID != "" || rl.NotifyURL != "" {
			wired++
		}
	}

	switch {
	case enabled == 0:
		cat.Status = HealthScoreStatusWarn
		cat.Reason = fmt.Sprintf("%d alert rule(s) configured, but all are disabled", len(rules))
	case wired == 0:
		cat.Status = HealthScoreStatusWarn
		cat.Reason = "alert rules exist but none have a notification channel attached"
	default:
		cat.Status = HealthScoreStatusPass
		cat.Reason = fmt.Sprintf("%d enabled alert rule(s), notifications wired", enabled)
	}
	return cat
}
