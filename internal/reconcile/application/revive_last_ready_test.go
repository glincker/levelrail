package application

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

type historyStore struct {
	fakeStore
	attempts []store.DeployAttempt
}

func (h *historyStore) ListDeployAttempts(context.Context, string) ([]store.DeployAttempt, error) {
	return h.attempts, nil
}

func TestReviveLastReady(t *testing.T) {
	desired := &store.DesiredService{Name: "web", Image: "app:v3", Port: 80}
	oldSvc := store.DesiredService{Name: "web", Image: "app:v2", Port: 80}
	failedSvc := store.DesiredService{Name: "web", Image: "app:v1", Port: 80}
	oldName := ContainerName("web", NameImage(oldSvc), "")
	failedName := ContainerName("web", NameImage(failedSvc), "")

	tests := []struct {
		name        string
		oldRunning  bool
		wantRestart bool
	}{
		{"live container dead, desired failed", false, true},
		{"live container still running", true, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rt := newFakeRuntime(8080)
			rt.createErr = errors.New("boom")
			rt.seed(oldName, tc.oldRunning)
			rt.containers[oldName].Image = "app:v2"
			rt.seed(failedName, false)
			rt.containers[failedName].Image = "app:v1"
			st := &historyStore{
				fakeStore: fakeStore{svc: desired},
				attempts: []store.DeployAttempt{
					{ID: "d3", Image: "app:v3", Status: store.DeployAttemptStatusFailed},
					{ID: "d2", Image: "app:v2", Status: store.DeployAttemptStatusSucceeded},
					{ID: "d1", Image: "app:v1", Status: store.DeployAttemptStatusFailed},
				},
			}
			c := New("web", st, rt, WithRestartBackoff(NewRestartBackoff(time.Hour, time.Hour, time.Hour)))
			_, _ = c.Reconcile(context.Background())
			if got := rt.containers[oldName].Running; got != true && tc.wantRestart {
				t.Errorf("last ready release not restarted")
			}
			if rt.containers[failedName].Running {
				t.Errorf("a failed release must never be revived")
			}
			// Half-succeeded: a second pass inside the backoff must not restart again.
			rt.mu.Lock()
			rt.containers[oldName].Running = false
			rt.mu.Unlock()
			before := rt.startCalls
			_, _ = c.Reconcile(context.Background())
			if tc.wantRestart && rt.startCalls != before {
				t.Errorf("restarted again inside backoff")
			}
		})
	}
}
