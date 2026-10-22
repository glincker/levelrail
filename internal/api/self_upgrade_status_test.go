package api

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/selfupgrade"
	"github.com/GLINCKER/levelrail/kit/upgrade"
)

func TestSelfUpgradeStatusAndConcurrentStart(t *testing.T) {
	cases := []struct {
		name        string
		attempts    []selfupgrade.Attempt
		wantRunning string
		wantLast    string
		wantVerif   bool
		startStatus int
	}{
		{name: "idle", startStatus: http.StatusAccepted},
		{
			name: "running blocks a second start",
			attempts: []selfupgrade.Attempt{
				{ID: "run-1", FromVersion: "v1.0.0", ToVersion: "v1.2.0", Outcome: selfupgrade.OutcomeRunning, StartedAt: time.Now()},
			},
			wantRunning: "run-1",
			startStatus: http.StatusConflict,
		},
		{
			name: "finished success is verified and does not block",
			attempts: []selfupgrade.Attempt{
				{
					ID: "done-1", FromVersion: "v0.9.0", ToVersion: "v1.0.0", Outcome: selfupgrade.OutcomeSucceeded,
					StartedAt: time.Now().Add(-time.Minute), FinishedAt: time.Now(),
					Steps: []selfupgrade.StepRecord{{Name: selfupgrade.StepHealth, Status: "ok"}},
				},
			},
			wantLast:    "done-1",
			wantVerif:   true,
			startStatus: http.StatusAccepted,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rt, cookie, launched := newSelfUpgradeRouter(t, true, nil)
			rt.selfUpgrade.notes = func(context.Context) ([]upgrade.HistoryRelease, error) {
				return []upgrade.HistoryRelease{{Tag: "v1.2.0", Body: "plain notes"}}, nil
			}
			j := selfupgrade.NewJournal(rt.dataDir)
			for _, a := range tc.attempts {
				if err := j.Save(a); err != nil {
					t.Fatal(err)
				}
			}
			code, body := suDo(t, rt, cookie, http.MethodGet, "/api/v1/updates/self-upgrade/status", "")
			if code != http.StatusOK {
				t.Fatalf("status %d: %s", code, body)
			}
			var st selfUpgradeStatusResource
			if err := json.Unmarshal(body, &st); err != nil {
				t.Fatal(err)
			}
			if got := idOf(st.Running); got != tc.wantRunning {
				t.Errorf("running = %q, want %q", got, tc.wantRunning)
			}
			if got := idOf(st.LastFinished); got != tc.wantLast {
				t.Errorf("last finished = %q, want %q", got, tc.wantLast)
			}
			if st.Verified != tc.wantVerif {
				t.Errorf("verified = %v, want %v", st.Verified, tc.wantVerif)
			}
			code, body = suDo(t, rt, cookie, http.MethodPost, "/api/v1/updates/self-upgrade", `{"target":"v1.2.0","ack":[]}`)
			if code != tc.startStatus {
				t.Fatalf("start status %d, want %d: %s", code, tc.startStatus, body)
			}
			if code == http.StatusConflict {
				var c struct {
					ID string `json:"running_attempt_id"`
				}
				if err := json.Unmarshal(body, &c); err != nil || c.ID != tc.wantRunning {
					t.Errorf("conflict body %s", body)
				}
				if len(*launched) != 0 {
					t.Error("launcher ran despite a running upgrade")
				}
			}
		})
	}
}

func idOf(a *selfUpgradeAttemptItem) string {
	if a == nil {
		return ""
	}
	return a.ID
}
