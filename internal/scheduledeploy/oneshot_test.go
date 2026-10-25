package scheduledeploy

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

type fakeOneShotStore struct {
	rows   []store.ScheduledDeploy
	status map[string]string
	reason map[string]string
}

func (f *fakeOneShotStore) ListDueScheduledDeploys(_ context.Context, now, stale time.Time) ([]store.ScheduledDeploy, error) {
	var out []store.ScheduledDeploy
	for _, r := range f.rows {
		cur := f.status[r.ID]
		if (cur == store.ScheduledDeployPending && !r.RunAt.After(now)) || (cur == store.ScheduledDeployRunning && !r.RunAt.After(stale)) {
			r.Status = cur
			out = append(out, r)
		}
	}
	return out, nil
}

func (f *fakeOneShotStore) ClaimScheduledDeploy(_ context.Context, id string, _ time.Time) error {
	if f.status[id] != store.ScheduledDeployPending {
		return store.ErrScheduledDeployNotPending
	}
	f.status[id] = store.ScheduledDeployRunning
	return nil
}

func (f *fakeOneShotStore) FinishScheduledDeploy(_ context.Context, id, status, reason, _ string, _ time.Time) error {
	f.status[id], f.reason[id] = status, reason
	return nil
}

type fakeExec struct {
	calls []string
	err   error
	appr  string
}

func (e *fakeExec) ExecuteScheduledDeploy(_ context.Context, d store.ScheduledDeploy) (Outcome, error) {
	e.calls = append(e.calls, d.ID)
	return Outcome{ApprovalID: e.appr, Message: "ok"}, e.err
}

type fakeNotify struct{ statuses []string }

func (n *fakeNotify) NotifyScheduledDeploy(_ context.Context, _ store.ScheduledDeploy, status, _ string) {
	n.statuses = append(n.statuses, status)
}

func TestOneShotRunnerTick(t *testing.T) {
	runAt := time.Date(2026, 12, 1, 3, 0, 0, 0, time.UTC)
	tests := []struct {
		name       string
		now        time.Time
		initial    string
		freeze     bool
		execErr    error
		approval   string
		want       string
		wantCalls  int
		wantNotify bool
	}{
		{name: "not due", now: runAt.Add(-time.Minute), initial: store.ScheduledDeployPending, want: store.ScheduledDeployPending},
		{name: "due fires", now: runAt.Add(time.Second), initial: store.ScheduledDeployPending, want: store.ScheduledDeployFired, wantCalls: 1},
		{name: "within grace after restart", now: runAt.Add(4 * time.Minute), initial: store.ScheduledDeployPending, want: store.ScheduledDeployFired, wantCalls: 1},
		{name: "beyond grace is missed", now: runAt.Add(10 * time.Minute), initial: store.ScheduledDeployPending, want: store.ScheduledDeployMissed, wantNotify: true},
		{name: "canceled is ignored", now: runAt.Add(time.Second), initial: store.ScheduledDeployCanceled, want: store.ScheduledDeployCanceled},
		{name: "frozen is skipped", now: runAt.Add(time.Second), initial: store.ScheduledDeployPending, freeze: true, want: store.ScheduledDeploySkippedFreeze, wantNotify: true},
		{name: "exec error fails", now: runAt.Add(time.Second), initial: store.ScheduledDeployPending, execErr: errors.New("boom"), want: store.ScheduledDeployFailed, wantCalls: 1, wantNotify: true},
		{name: "approval pending", now: runAt.Add(time.Second), initial: store.ScheduledDeployPending, approval: "appr_1", want: store.ScheduledDeployPendingApproval, wantCalls: 1},
		{name: "stale running closed not retried", now: runAt.Add(2 * time.Hour), initial: store.ScheduledDeployRunning, want: store.ScheduledDeployFailed, wantNotify: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			st := &fakeOneShotStore{
				rows:   []store.ScheduledDeploy{{ID: "d1", ServiceName: "web", RunAt: runAt}},
				status: map[string]string{"d1": tc.initial}, reason: map[string]string{},
			}
			ex, nt := &fakeExec{err: tc.execErr, appr: tc.approval}, &fakeNotify{}
			r := &OneShotRunner{Store: st, Exec: ex, Notify: nt, Now: func() time.Time { return tc.now }, Grace: 5 * time.Minute, StaleRun: 30 * time.Minute}
			if tc.freeze {
				r.Freeze = &fakeFreezeStore{windows: []store.DeployFreezeWindow{{ID: "fw_1", Scope: store.DeployFreezeScopeApp("web"), Cron: "0 0 * * *", Duration: 24 * time.Hour, Reason: "launch"}}}
			}
			_ = r.Tick(context.Background())
			if st.status["d1"] != tc.want {
				t.Errorf("status = %q (%s), want %q", st.status["d1"], st.reason["d1"], tc.want)
			}
			if len(ex.calls) != tc.wantCalls {
				t.Errorf("exec calls = %d, want %d", len(ex.calls), tc.wantCalls)
			}
			if (len(nt.statuses) > 0) != tc.wantNotify {
				t.Errorf("notified = %v, want %v", nt.statuses, tc.wantNotify)
			}
			_ = r.Tick(context.Background())
			if len(ex.calls) > 1 {
				t.Errorf("second tick re-executed: %d calls", len(ex.calls))
			}
		})
	}
}
