package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/selfupgrade"
	"github.com/GLINCKER/levelrail/kit/upgrade"
)

func newSelfUpgradeRouter(t *testing.T, canLaunch bool, notesErr error) (*Router, *http.Cookie, *[][]string) {
	t.Helper()
	setVersion(t, "v1.0.0")
	rt, db := newTestRouter(t)
	rt.dataDir = t.TempDir()
	var launched [][]string
	WithSelfUpgrade(db, func(_ context.Context, args []string) error {
		launched = append(launched, args)
		return nil
	})(rt)
	rt.selfUpgrade.canLaunch = func() (bool, string) {
		if canLaunch {
			return true, ""
		}
		return false, "not root"
	}
	rt.selfUpgrade.notes = func(context.Context) ([]upgrade.HistoryRelease, error) {
		if notesErr != nil {
			return nil, notesErr
		}
		return []upgrade.HistoryRelease{
			{Tag: "v1.2.0", Body: "<!-- levelrail-upgrade: {\"breaking\":[{\"id\":\"ports\",\"summary\":\"ingress ports move\",\"ack\":true}]} -->"},
			{Tag: "v1.1.0", Body: "## Breaking changes\n- env var renamed\n"},
			{Tag: "v1.0.0", Body: "## Breaking changes\n- ancient\n"},
		}, nil
	}
	return rt, loginTestSession(t, rt, db), &launched
}

func suDo(t *testing.T, rt *Router, cookie *http.Cookie, method, path, body string) (int, []byte) {
	t.Helper()
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, method, path, body))
	return rec.Code, rec.Body.Bytes()
}

func TestSelfUpgradePlan(t *testing.T) {
	rt, cookie, _ := newSelfUpgradeRouter(t, true, nil)
	code, body := suDo(t, rt, cookie, http.MethodGet, "/api/v1/updates/self-upgrade/plan?target=v1.2.0", "")
	if code != http.StatusOK {
		t.Fatalf("status %d: %s", code, body)
	}
	var plan selfUpgradePlanResource
	if err := json.Unmarshal(body, &plan); err != nil {
		t.Fatal(err)
	}
	if len(plan.Breaking) != 2 || plan.Breaking[0].Version != "v1.1.0" || plan.Breaking[1].ID != "ports" {
		t.Fatalf("breaking = %+v", plan.Breaking)
	}
	if !plan.CanApply || !strings.Contains(plan.Command, "--ack") {
		t.Fatalf("plan = %+v", plan)
	}

	tests := []struct {
		name   string
		target string
		want   int
	}{
		{"unsafe tag", "..%2Fetc", http.StatusBadRequest},
		{"not newer", "v1.0.0", http.StatusConflict},
		{"unknown release", "v7.7.7", http.StatusNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, body := suDo(t, rt, cookie, http.MethodGet, "/api/v1/updates/self-upgrade/plan?target="+tt.target, "")
			if code != tt.want {
				t.Fatalf("status %d, want %d: %s", code, tt.want, body)
			}
		})
	}
}

func TestSelfUpgradeStart(t *testing.T) {
	tests := []struct {
		name      string
		canLaunch bool
		notesErr  error
		body      string
		wantCode  int
		wantLaunc int
	}{
		{"missing acknowledgement", true, nil, `{"target":"v1.2.0","ack":["ports"]}`, http.StatusConflict, 0},
		{"all acknowledged", true, nil, `{"target":"v1.2.0","ack":["ports","` + headingID("v1.1.0", "env var renamed") + `"]}`, http.StatusAccepted, 1},
		{"host cannot launch", false, nil, `{"target":"v1.1.0","ack":["` + headingID("v1.1.0", "env var renamed") + `"]}`, http.StatusConflict, 0},
		{"notes unavailable refuses", true, errors.New("offline"), `{"target":"v1.2.0"}`, http.StatusBadGateway, 0},
		{"bad json", true, nil, `{`, http.StatusBadRequest, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rt, cookie, launched := newSelfUpgradeRouter(t, tt.canLaunch, tt.notesErr)
			code, body := suDo(t, rt, cookie, http.MethodPost, "/api/v1/updates/self-upgrade", tt.body)
			if code != tt.wantCode {
				t.Fatalf("status %d, want %d: %s", code, tt.wantCode, body)
			}
			if len(*launched) != tt.wantLaunc {
				t.Fatalf("launched %d times, want %d", len(*launched), tt.wantLaunc)
			}
			if tt.wantLaunc == 1 {
				args := strings.Join((*launched)[0], " ")
				if !strings.Contains(args, "--to v1.2.0") || !strings.Contains(args, "--yes") || !strings.Contains(args, "--initiator dashboard:") {
					t.Fatalf("args = %s", args)
				}
			}
		})
	}
}

func headingID(tag, summary string) string {
	for _, b := range selfupgrade.ParseBreaking(tag, "## Breaking changes\n- "+summary+"\n") {
		return b.ID
	}
	return ""
}

func TestSelfUpgradeAttemptsImportsJournal(t *testing.T) {
	rt, cookie, _ := newSelfUpgradeRouter(t, true, nil)
	j := selfupgrade.NewJournal(rt.dataDir)
	err := j.Save(selfupgrade.Attempt{
		ID: "su-abc", FromVersion: "v1.0.0", ToVersion: "v1.1.0", Outcome: selfupgrade.OutcomeRolledBack,
		FailedStep: selfupgrade.StepHealth, StartedAt: time.Now(), FinishedAt: time.Now(),
		Steps: []selfupgrade.StepRecord{{Name: selfupgrade.StepHealth, Status: selfupgrade.StatusFailed, At: time.Now()}},
	})
	if err != nil {
		t.Fatal(err)
	}
	code, body := suDo(t, rt, cookie, http.MethodGet, "/api/v1/updates/self-upgrade/attempts", "")
	if code != http.StatusOK {
		t.Fatalf("status %d: %s", code, body)
	}
	var out struct {
		Attempts []selfUpgradeAttemptItem `json:"attempts"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Attempts) != 1 || out.Attempts[0].Outcome != selfupgrade.OutcomeRolledBack || len(out.Attempts[0].Steps) != 1 {
		t.Fatalf("attempts = %+v", out.Attempts)
	}
	if left, _ := j.List(); len(left) != 0 {
		t.Fatalf("finished journal file not removed: %d left", len(left))
	}
}
