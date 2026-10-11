package api

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/cpbackup"
	"github.com/GLINCKER/levelrail/internal/selfupgrade"
	"github.com/GLINCKER/levelrail/internal/upgradesched"
	"github.com/GLINCKER/levelrail/internal/version"
	"github.com/GLINCKER/levelrail/kit/upgrade"
)

// TestLiveScheduledUpgradeFromBlockerToApplied drives the whole path over
// HTTP with the real SQLite store and the real control plane backup manager
// (an actual database snapshot on disk): a missing backup blocks, Back up now
// clears it, a scheduled upgrade fires at its time through the same launcher
// Upgrade now uses, survives a "restart" (a second scheduler over the same
// database), and the attempt is recorded. Skipped under -short.
func TestLiveScheduledUpgradeFromBlockerToApplied(t *testing.T) {
	if testing.Short() {
		t.Skip("live path: real database snapshot and journal on disk")
	}
	t.Setenv("APP_UPGRADE_MIN_FREE_BYTES", "1")
	rt, cookie, launched := newSelfUpgradeRouter(t, true, nil)
	db, ok := rt.selfUpgrade.store.(UpgradeScheduleStore)
	if !ok {
		t.Fatal("store does not implement UpgradeScheduleStore")
	}
	sender := &captureSender{}
	WithUpgradeScheduling(db, sender, upgradesched.DefaultConfig())(rt)
	rt.cpBackups = cpbackup.NewManager(rt.selfUpgrade.store.(interface {
		SnapshotTo(context.Context, string) (int64, string, error)
	}), t.TempDir())
	rt.fetchLatestRelease = func(context.Context) (*githubRelease, error) {
		r := releaseWithAssets(upgrade.ChecksumsAsset, upgrade.SignatureAsset)
		r.TagName = "v1.1.0"
		return r, nil
	}

	preflight := func() updatePreflightResource {
		code, body := suDo(t, rt, cookie, http.MethodGet, "/api/v1/updates/preflight?refresh=1", "")
		if code != http.StatusOK {
			t.Fatalf("preflight %d: %s", code, body)
		}
		return decodeBody[updatePreflightResource](t, body)
	}

	pre := preflight()
	var backup upgrade.Check
	for _, c := range pre.Checks {
		if c.Code == "control_plane_backup" {
			backup = c
		}
	}
	if !pre.Blocked || backup.Status != upgrade.StatusFail || backup.Fix == nil || backup.Fix.Action != upgrade.FixBackupNow {
		t.Fatalf("a server with no backup must be blocked with a back up now fix: %+v", pre.Checks)
	}

	code, body := suDo(t, rt, cookie, http.MethodPost, "/api/v1/updates/backup-now", "")
	if code != http.StatusCreated {
		t.Fatalf("backup-now %d: %s", code, body)
	}
	res := decodeBody[upgradeBackupNowResponse](t, body)
	if res.Preflight.Blocked {
		t.Fatalf("backing up must clear the blocker: %+v", res.Preflight.Checks)
	}
	list, err := rt.cpBackups.List()
	if err != nil || len(list) != 1 {
		t.Fatalf("a real backup file must exist: %v %v", list, err)
	}
	if fi, err := os.Stat(filepath.Join(rt.dataDir)); err != nil || !fi.IsDir() {
		t.Fatalf("data dir: %v", err)
	}

	s := rt.UpgradeScheduler()
	if s == nil {
		t.Fatal("scheduler not configured")
	}
	clock := time.Now().UTC()
	s.Now = func() time.Time { return clock }
	runAt := clock.Add(time.Hour)
	ack := headingID()
	code, body = suDo(t, rt, cookie, http.MethodPost, "/api/v1/updates/schedule",
		`{"target":"v1.1.0","at":"`+runAt.Format(time.RFC3339)+`","ack":["`+ack+`"]}`)
	if code != http.StatusCreated {
		t.Fatalf("schedule %d: %s", code, body)
	}
	sc := decodeBody[upgradeScheduleResource](t, body)

	if err := s.Tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(*launched) != 0 {
		t.Fatalf("fired early: %v", *launched)
	}

	clock = runAt.Add(5 * time.Second)
	if err := s.Tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(*launched) != 1 {
		t.Fatalf("launched %d times, want 1", len(*launched))
	}
	args := strings.Join((*launched)[0], " ")
	for _, want := range []string{"--to v1.1.0", "--yes", "--initiator schedule:" + sc.ID, "--ack " + ack} {
		if !strings.Contains(args, want) {
			t.Fatalf("launch args %q missing %q", args, want)
		}
	}
	if list, _ := rt.cpBackups.List(); len(list) < 2 {
		t.Fatalf("a fresh backup must be taken at fire time, have %d", len(list))
	}

	// The host engine journals its attempt, the control plane restarts as the
	// new version, and a brand new scheduler over the same database settles it.
	err = selfupgrade.NewJournal(rt.dataDir).Save(selfupgrade.Attempt{
		ID: "su-live-1", FromVersion: "v1.0.0", ToVersion: "v1.1.0", Initiator: "schedule:" + sc.ID,
		Outcome: selfupgrade.OutcomeSucceeded, StartedAt: clock, FinishedAt: clock.Add(time.Minute),
	})
	if err != nil {
		t.Fatal(err)
	}
	version.Version = "v1.1.0"
	restarted := rt.upgradeScheduler2(t, db)
	restarted.Now = func() time.Time { return clock.Add(2 * time.Minute) }
	if err := restarted.Tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(*launched) != 1 {
		t.Fatalf("restart re-applied: %v", *launched)
	}

	code, body = suDo(t, rt, cookie, http.MethodGet, "/api/v1/updates/schedule", "")
	status := decodeBody[upgradeScheduleStatus](t, body)
	if code != http.StatusOK || status.Active != nil || len(status.Recent) == 0 || status.Recent[0].State != "applied" {
		t.Fatalf("schedule must end applied: %d %+v", code, status)
	}
	code, body = suDo(t, rt, cookie, http.MethodGet, "/api/v1/updates/self-upgrade/attempts", "")
	attempts := decodeBody[struct {
		Attempts []selfUpgradeAttemptItem `json:"attempts"`
	}](t, body)
	if code != http.StatusOK || len(attempts.Attempts) != 1 || attempts.Attempts[0].Initiator != "schedule:"+sc.ID || attempts.Attempts[0].Outcome != "succeeded" {
		t.Fatalf("attempt not recorded: %d %+v", code, attempts)
	}
}

// upgradeScheduler2 builds a second scheduler over the same stores, which is
// what a restarted control plane process looks like to the persisted state.
func (rt *Router) upgradeScheduler2(t *testing.T, st UpgradeScheduleStore) *upgradesched.Scheduler {
	t.Helper()
	return &upgradesched.Scheduler{
		Store: st, Host: upgradeHost{rt: rt}, Config: upgradesched.DefaultConfig(), Logger: rt.logger,
		Current: func() string { return version.Version },
		NewID:   upgradesched.NewScheduleID,
	}
}
