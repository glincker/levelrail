package reconcile

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/deploy"
	"github.com/GLINCKER/levelrail/internal/deploysafety"
	"github.com/GLINCKER/levelrail/internal/deploywatch"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/test/e2e/testenv"
)

// TestDeployWatch_Live_HealthyStaysCrashloopRollsBack proves the post-deploy
// watch on a real engine: a release that stays healthy through the whole
// watch window is left alone, and a release that passes the readiness gate
// then keeps crashing is rolled back to the previous image, with the
// rollback recorded in deploy history, the gate timeline and the audit log.
func TestDeployWatch_Live_HealthyStaysCrashloopRollsBack(t *testing.T) {
	if testing.Short() {
		t.Skip("live Docker test")
	}
	testenv.RequireFullLive(t)
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Minute)
	defer cancel()

	s := startSafetyStack(ctx, t)
	t.Cleanup(func() {
		if t.Failed() {
			t.Logf("notifications = %q", *s.notes)
			for _, v := range []string{"v2", "v5"} {
				t.Logf("%s attempt = %+v, timeline = %+v", v, s.attempt(context.Background(), t, v), s.events(context.Background(), v))
			}
		}
	})
	ship := func(version string, env map[string]string) {
		t.Helper()
		merged := map[string]string{"VERSION": version, "MODE": "ok"}
		for k, v := range env {
			merged[k] = v
		}
		svc := store.DesiredService{
			Name: safetyService, Port: 8080, Domains: []string{safetyDomain},
			Command: []string{"node", "-e", safetyAppScript},
			Env:     merged,
			Health: &store.ServiceHealth{Readiness: &store.ServiceProbe{
				Path: "/healthz", Interval: 300 * time.Millisecond, Timeout: time.Second,
			}},
		}
		trigger := deploy.ImageTrigger{Store: s.db, Nudger: s.engine, Source: store.DeployAttemptSourceImage}
		if _, err := trigger.Deploy(ctx, svc, safetyRepo+":"+version); err != nil {
			t.Fatalf("deploy %s: %v", version, err)
		}
	}
	client := ingressClient(s.addr)
	url := "https://" + safetyDomain + "/"
	serves := func(version string) func() bool {
		return func() bool {
			body, status, err := doGet(client, url)
			return err == nil && status == 200 && body == "ok-"+version
		}
	}

	ship("v1", nil)
	waitFor(t, 90*time.Second, "v1 to serve", serves("v1"))

	ship("v2", nil)
	waitFor(t, 90*time.Second, "v2 to serve", serves("v2"))
	waitFor(t, 120*time.Second, "the watch to pass v2", func() bool {
		for _, e := range s.events(ctx, "v2") {
			if e.Phase == deploysafety.PhaseWatch && e.Status == deploysafety.StatusPassed {
				return true
			}
		}
		return false
	})
	if a := s.attempt(ctx, t, "v2"); a.Status != store.DeployAttemptStatusSucceeded {
		t.Fatalf("healthy v2 attempt status = %q reason %q, want succeeded", a.Status, a.Reason)
	}
	if s.hasPhase(ctx, "v2", deploysafety.PhaseRollback) {
		t.Fatalf("healthy v2 was rolled back: %+v", s.events(ctx, "v2"))
	}
	if n := s.autoRollbackAudits(ctx, t); n != 0 {
		t.Fatalf("audit log has %d auto rollback entries after a healthy deploy, want 0", n)
	}

	// Crash after the stable window so the readiness gate passes and only
	// the post-deploy watch can catch it.
	ship("v5", map[string]string{"MODE": "crash", "CRASH_AFTER_MS": "6000"})
	waitFor(t, 120*time.Second, "the watch to roll v5 back", func() bool {
		a := s.attempt(ctx, t, "v5")
		return a.Status == store.DeployAttemptStatusFailed && s.hasPhase(ctx, "v5", deploysafety.PhaseRollback)
	})
	waitFor(t, 90*time.Second, "v2 to serve again", serves("v2"))

	failed := s.attempt(ctx, t, "v5")
	if !strings.HasPrefix(failed.Reason, "PostDeploy") {
		t.Errorf("v5 reason = %q, want a post-deploy watch reason", failed.Reason)
	}
	svc, err := s.db.GetDesiredService(ctx, safetyService)
	if err != nil || !strings.HasPrefix(svc.Image, "lrtest-rollback/") || svc.Env["VERSION"] != "v2" || svc.Env["MODE"] != "ok" {
		t.Errorf("desired after rollback = %+v (err %v), want v2's pinned image and config", svc, err)
	}

	attempts, err := s.db.ListDeployAttempts(ctx, safetyService)
	if err != nil {
		t.Fatalf("ListDeployAttempts() error = %v", err)
	}
	var rollbackAttempt *store.DeployAttempt
	for i := range attempts {
		if attempts[i].Source == store.DeployAttemptSourceAutoRollback {
			rollbackAttempt = &attempts[i]
		}
	}
	if rollbackAttempt == nil {
		t.Fatalf("deploy history %+v has no %s attempt", attempts, store.DeployAttemptSourceAutoRollback)
	}
	waitFor(t, 60*time.Second, "the rollback attempt to succeed", func() bool {
		a, err := s.db.GetDeployAttempt(ctx, rollbackAttempt.ID)
		return err == nil && a.Status == store.DeployAttemptStatusSucceeded
	})

	waitFor(t, 10*time.Second, "the audit entry", func() bool { return s.autoRollbackAudits(ctx, t) == 1 })
	entries, err := s.db.ListAuditEntries(ctx, 20, nil, store.AuditEntryFilter{Action: deploywatch.AuditActionAutoRollback})
	if err != nil || len(entries) != 1 {
		t.Fatalf("audit entries = %+v (err %v), want exactly one", entries, err)
	}
	if !strings.Contains(entries[0].Detail, failed.ID) {
		t.Errorf("audit detail %q does not name the failed deploy %s", entries[0].Detail, failed.ID)
	}
	if len(*s.notes) == 0 || !strings.Contains(strings.Join(*s.notes, "\n"), "rolled back automatically") {
		t.Errorf("notifications = %v, want an automatic rollback alert", *s.notes)
	}
}

func (s *safetyStack) autoRollbackAudits(ctx context.Context, t *testing.T) int {
	t.Helper()
	entries, err := s.db.ListAuditEntries(ctx, 50, nil, store.AuditEntryFilter{Action: deploywatch.AuditActionAutoRollback})
	if err != nil {
		t.Fatalf("ListAuditEntries() error = %v", err)
	}
	return len(entries)
}
