package deploywatch

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/internal/deploysafety"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/telemetry"
)

// AuditSink writes audit log rows. *store.DB satisfies it.
type AuditSink interface {
	SaveAuditEntry(ctx context.Context, e store.AuditEntry) error
}

// AuditActionAutoRollback is the audit action for a rollback the watch started.
const AuditActionAutoRollback = "deploy.auto_rollback"

type baseline struct {
	p95Ms    float64
	requests float64
}

// softState holds per-release hysteresis counters and baselines in memory;
// losing them on restart only delays a soft signal, never fires one early.
type softState struct {
	mu        sync.Mutex
	streaks   map[string]int
	baselines map[string]baseline
}

func (s *softState) forget(attemptID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.streaks, attemptID)
	delete(s.baselines, attemptID)
}

func (s *softState) baseline(attemptID string) (baseline, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, ok := s.baselines[attemptID]
	return b, ok
}

func (s *softState) setBaseline(attemptID string, b baseline) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.baselines == nil {
		s.baselines = map[string]baseline{}
	}
	s.baselines[attemptID] = b
}

// bump records one more consecutive regressed tick and returns the streak;
// a clean tick resets it.
func (s *softState) bump(attemptID string, regressed bool) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.streaks == nil {
		s.streaks = map[string]int{}
	}
	if !regressed {
		delete(s.streaks, attemptID)
		return 0
	}
	s.streaks[attemptID]++
	return s.streaks[attemptID]
}

// observeLatency fills the p95 fields. The previous release's baseline is
// read once per release, over the watch window before cutover.
func (w *Watcher) observeLatency(ctx context.Context, service, attemptID string, cfg deploysafety.Effective, cutover, from, now time.Time, obs *deploysafety.WatchObservation) {
	cur, err := telemetry.SummarizeRequests(ctx, w.Metrics, service, now.Sub(from), now)
	if err != nil || !cur.HasTraffic {
		return
	}
	obs.P95Ms = cur.P95Ms
	base, ok := w.soft.baseline(attemptID)
	if !ok {
		prev, err := telemetry.SummarizeRequests(ctx, w.Metrics, service, cfg.WatchWindow, cutover)
		if err != nil {
			return
		}
		base = baseline{p95Ms: prev.P95Ms, requests: prev.Requests}
		w.soft.setBaseline(attemptID, base)
	}
	obs.BaselineP95Ms, obs.BaselineRequests = base.p95Ms, base.requests
}

// softSignal handles a latency regression: it must hold for WatchSoftTicks
// evaluations in a row, then notifies, and rolls back only when the app
// opted in with mode auto plus soft rollback.
func (w *Watcher) softSignal(ctx context.Context, svc *store.DesiredService, attempt *store.DeployAttempt, cfg deploysafety.Effective, obs deploysafety.WatchObservation, st watchState) {
	regressed, message := deploysafety.LatencyRegressed(obs, cfg)
	if streak := w.soft.bump(attempt.ID, regressed); !regressed || streak < cfg.WatchSoftTicks {
		return
	}
	if cfg.WatchMode == deploysafety.WatchModeAuto && cfg.WatchSoftRollback {
		w.trip(ctx, svc, attempt, cfg, deploysafety.ReasonWatchLatency, message)
		return
	}
	if st.softNotified {
		return
	}
	w.record(ctx, attempt, deploysafety.PhaseWatch, deploysafety.StatusInfo, deploysafety.ReasonWatchLatency, message+"; not rolling back, soft rollback is off for this app")
	w.notify(ctx, svc.Name, svc.Image, fmt.Sprintf("%s: %s; not rolling back, soft rollback is off for this app", deploysafety.ReasonWatchLatency, message))
}

func (w *Watcher) audit(ctx context.Context, app, failedDeploy, targetDeploy, detail string) {
	if w.Audit == nil {
		return
	}
	id, err := store.NewAuditEntryID()
	if err != nil {
		return
	}
	body, _ := json.Marshal(map[string]string{"app": app, "failed_deploy": failedDeploy, "rolled_back_to": targetDeploy, "why": detail})
	entry := store.AuditEntry{
		ID: id, ActorType: "system", ActorID: "deploywatch", ActorName: "post-deploy watch", Ability: "deploy",
		Method: "POST", Path: "/api/v1/apps/" + app + "/deploys/" + targetDeploy + "/rollback", StatusCode: 200,
		CreatedAt: store.FormatAuditTime(w.clock()), ClientKind: "system", Action: AuditActionAutoRollback, Detail: string(body),
	}
	if err := w.Audit.SaveAuditEntry(ctx, entry); err != nil {
		w.logger().Warn("deploywatch: audit write failed", slog.String("service", app), slog.String("error", err.Error()))
	}
}
