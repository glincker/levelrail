package api

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/alerting"
	"github.com/GLINCKER/levelrail/internal/cpbackup"
	"github.com/GLINCKER/levelrail/internal/upgradesched"
	"github.com/GLINCKER/levelrail/kit/upgrade"
)

type fakeBackupManager struct {
	mu   sync.Mutex
	list []cpbackup.Info
}

func (f *fakeBackupManager) Create(context.Context) (cpbackup.Info, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	info := cpbackup.Info{Name: "levelrail-test.db", SizeBytes: 10, CreatedAt: time.Now()}
	f.list = append([]cpbackup.Info{info}, f.list...)
	return info, nil
}

func (f *fakeBackupManager) List() ([]cpbackup.Info, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]cpbackup.Info(nil), f.list...), nil
}

func (f *fakeBackupManager) Open(string) (*os.File, cpbackup.Info, error) {
	return nil, cpbackup.Info{}, cpbackup.ErrNotFound
}
func (f *fakeBackupManager) Delete(string) error { return nil }
func (f *fakeBackupManager) Verify(context.Context, string) (cpbackup.VerifyResult, error) {
	return cpbackup.VerifyResult{}, nil
}

type captureSender struct {
	mu    sync.Mutex
	texts []string
}

func (c *captureSender) SendNotice(_ context.Context, _ alerting.NotificationChannel, text string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.texts = append(c.texts, text)
	return nil
}

func newScheduleRouter(t *testing.T) (*Router, *http.Cookie, *[][]string, *fakeBackupManager) {
	t.Helper()
	rt, cookie, launched := newSelfUpgradeRouter(t, true, nil)
	db, ok := rt.selfUpgrade.store.(UpgradeScheduleStore)
	if !ok {
		t.Fatal("test store does not implement UpgradeScheduleStore")
	}
	WithUpgradeScheduling(db, &captureSender{}, upgradesched.DefaultConfig())(rt)
	mgr := &fakeBackupManager{list: []cpbackup.Info{{Name: "old.db", CreatedAt: time.Now().Add(-40 * time.Hour)}}}
	rt.cpBackups = mgr
	rt.fetchLatestRelease = func(context.Context) (*githubRelease, error) {
		r := releaseWithAssets(upgrade.ChecksumsAsset, upgrade.SignatureAsset)
		r.TagName = "v1.2.0"
		return r, nil
	}
	return rt, cookie, launched, mgr
}

func decodeBody[T any](t *testing.T, body []byte) T {
	t.Helper()
	var v T
	if err := json.Unmarshal(body, &v); err != nil {
		t.Fatalf("decode: %v; body = %s", err, body)
	}
	return v
}

func TestStaleBackupBlockerHasAFixThatClearsIt(t *testing.T) {
	rt, cookie, _, _ := newScheduleRouter(t)

	code, body := suDo(t, rt, cookie, http.MethodGet, "/api/v1/updates/preflight", "")
	if code != http.StatusOK {
		t.Fatalf("status %d: %s", code, body)
	}
	pre := decodeBody[updatePreflightResource](t, body)
	var backup *upgrade.Check
	for i := range pre.Checks {
		if pre.Checks[i].Code == "control_plane_backup" {
			backup = &pre.Checks[i]
		}
	}
	if backup == nil || backup.Status != upgrade.StatusFail || backup.Fix == nil || backup.Fix.Action != upgrade.FixBackupNow {
		t.Fatalf("stale backup must block with a back_up_now fix: %+v", backup)
	}
	if !pre.Blocked {
		t.Fatal("preflight must be blocked by the stale backup")
	}

	code, body = suDo(t, rt, cookie, http.MethodPost, "/api/v1/updates/backup-now", "")
	if code != http.StatusCreated {
		t.Fatalf("backup-now status %d: %s", code, body)
	}
	res := decodeBody[upgradeBackupNowResponse](t, body)
	for _, c := range res.Preflight.Checks {
		if c.Code == "control_plane_backup" && c.Status != upgrade.StatusOK {
			t.Fatalf("backup check after backing up = %s (%s)", c.Status, c.Message)
		}
	}
}

func TestHalfPublishedReleaseIsRetryableNotScary(t *testing.T) {
	rt, cookie, _, mgr := newScheduleRouter(t)
	mgr.list = []cpbackup.Info{{Name: "fresh.db", CreatedAt: time.Now()}}
	publishing := true
	rt.fetchLatestRelease = func(context.Context) (*githubRelease, error) {
		r := releaseWithAssets(upgrade.ChecksumsAsset)
		r.TagName, r.PublishedAt = "v1.2.0", time.Now().Add(-3*time.Minute).UTC().Format(time.RFC3339)
		if !publishing {
			r = releaseWithAssets(upgrade.ChecksumsAsset, upgrade.SignatureAsset)
			r.TagName, r.PublishedAt = "v1.2.0", time.Now().Add(-3*time.Minute).UTC().Format(time.RFC3339)
		}
		return r, nil
	}

	_, body := suDo(t, rt, cookie, http.MethodGet, "/api/v1/updates/preflight", "")
	pre := decodeBody[updatePreflightResource](t, body)
	var sig upgrade.Check
	for _, c := range pre.Checks {
		if c.Code == "release_signature" {
			sig = c
		}
	}
	if sig.Status != upgrade.StatusPending || sig.Kind != upgrade.KindReleaseIncomplete || !sig.Retryable {
		t.Fatalf("half published release must read as pending and retryable: %+v", sig)
	}
	if !pre.Blocked || !pre.Waiting {
		t.Fatalf("blocked=%v waiting=%v", pre.Blocked, pre.Waiting)
	}
	if !strings.Contains(sig.Message, "still being published") {
		t.Fatalf("message = %q", sig.Message)
	}

	publishing = false
	_, body = suDo(t, rt, cookie, http.MethodGet, "/api/v1/updates/preflight?refresh=1", "")
	pre = decodeBody[updatePreflightResource](t, body)
	if pre.Blocked {
		t.Fatalf("re-check after publishing finished must clear: %+v", pre.Checks)
	}
}

func TestUpgradeScheduleLifecycle(t *testing.T) {
	rt, cookie, launched, _ := newScheduleRouter(t)
	at := time.Now().Add(6 * time.Hour).UTC().Format(time.RFC3339)

	code, body := suDo(t, rt, cookie, http.MethodPost, "/api/v1/updates/schedule", `{"target":"v1.1.0","at":"`+at+`","ack":["`+headingID()+`"]}`)
	if code != http.StatusCreated {
		t.Fatalf("create status %d: %s", code, body)
	}
	sc := decodeBody[upgradeScheduleResource](t, body)
	if sc.State != "pending" || sc.TargetVersion != "v1.1.0" || sc.Source != "manual" {
		t.Fatalf("schedule = %+v", sc)
	}

	code, body = suDo(t, rt, cookie, http.MethodGet, "/api/v1/updates/schedule", "")
	status := decodeBody[upgradeScheduleStatus](t, body)
	if code != http.StatusOK || !status.Available || status.Active == nil || status.Active.ID != sc.ID || status.ServerTimezone == "" {
		t.Fatalf("status = %d %+v", code, status)
	}

	if code, _ := suDo(t, rt, cookie, http.MethodPost, "/api/v1/updates/schedule", `{"target":"v1.1.0","at":"`+at+`","ack":["`+headingID()+`"]}`); code != http.StatusConflict {
		t.Fatalf("second schedule status %d, want 409", code)
	}

	code, body = suDo(t, rt, cookie, http.MethodDelete, "/api/v1/updates/schedule", "")
	if code != http.StatusOK || decodeBody[upgradeScheduleResource](t, body).State != "cancelled" {
		t.Fatalf("cancel = %d %s", code, body)
	}
	if code, _ := suDo(t, rt, cookie, http.MethodDelete, "/api/v1/updates/schedule", ""); code != http.StatusNotFound {
		t.Fatalf("second cancel status %d, want 404", code)
	}
	if len(*launched) != 0 {
		t.Fatalf("scheduling must not launch anything: %v", *launched)
	}
}

func TestUpgradeScheduleRefusals(t *testing.T) {
	rt, cookie, _, _ := newScheduleRouter(t)
	future := time.Now().Add(6 * time.Hour).UTC().Format(time.RFC3339)
	tests := []struct {
		name string
		body string
		want int
	}{
		{"breaking change needs ack", `{"target":"v1.2.0","at":"` + future + `"}`, http.StatusConflict},
		{"past time", `{"target":"v1.1.0","at":"2020-01-01T00:00:00Z","ack":["` + headingID() + `"]}`, http.StatusBadRequest},
		{"unknown release", `{"target":"v7.7.7","at":"` + future + `"}`, http.StatusNotFound},
		{"no time given", `{"target":"v1.1.0"}`, http.StatusBadRequest},
		{"bad timezone", `{"target":"v1.1.0","at_local":"2030-01-01T03:00","timezone":"Mars/Base"}`, http.StatusBadRequest},
		{"no maintenance window", `{"target":"v1.1.0","window":true,"ack":["` + headingID() + `"]}`, http.StatusConflict},
		{"bad json", `{`, http.StatusBadRequest},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			code, body := suDo(t, rt, cookie, http.MethodPost, "/api/v1/updates/schedule", tc.body)
			if code != tc.want {
				t.Fatalf("status %d, want %d: %s", code, tc.want, body)
			}
		})
	}
	code, body := suDo(t, rt, cookie, http.MethodPost, "/api/v1/updates/schedule", `{"target":"v1.2.0","at":"`+future+`"}`)
	if code != http.StatusConflict || !strings.Contains(string(body), "missing_ack") {
		t.Fatalf("ack refusal must list the ids: %d %s", code, body)
	}
}

func TestUpgradeScheduleAtLocalUsesTheNamedTimezone(t *testing.T) {
	rt, cookie, _, _ := newScheduleRouter(t)
	when := time.Now().UTC().AddDate(0, 0, 2).Format("2006-01-02") + "T03:00"
	code, body := suDo(t, rt, cookie, http.MethodPost, "/api/v1/updates/schedule",
		`{"target":"v1.1.0","at_local":"`+when+`","timezone":"Asia/Tokyo","ack":["`+headingID()+`"]}`)
	if code != http.StatusCreated {
		t.Fatalf("status %d: %s", code, body)
	}
	sc := decodeBody[upgradeScheduleResource](t, body)
	got, err := time.Parse(time.RFC3339, sc.RunAt)
	if err != nil {
		t.Fatal(err)
	}
	if got.UTC().Hour() != 18 || sc.Timezone != "Asia/Tokyo" {
		t.Fatalf("03:00 in Tokyo is 18:00 UTC the day before, got %s tz %s", sc.RunAt, sc.Timezone)
	}
}

func TestUpgradePolicyValidationAndRoundTrip(t *testing.T) {
	rt, cookie, _, _ := newScheduleRouter(t)

	code, body := suDo(t, rt, cookie, http.MethodGet, "/api/v1/updates/auto", "")
	pol := decodeBody[upgradePolicyResource](t, body)
	if code != http.StatusOK || pol.Mode != "off" || pol.Effective != "off" {
		t.Fatalf("default policy must be off: %d %+v", code, pol)
	}

	bad := []string{
		`{"mode":"any"}`,
		`{"mode":"yolo","window_cron":"0 3 * * *"}`,
		`{"mode":"any","window_cron":"nope"}`,
		`{"mode":"any","window_cron":"0 3 * * *","timezone":"Mars/Base"}`,
		`{"mode":"any","window_cron":"0 3 * * *","min_release_age_days":999}`,
		`{"mode":"any","window_cron":"0 3 * * *","window_duration_minutes":1}`,
	}
	for _, b := range bad {
		if code, body := suDo(t, rt, cookie, http.MethodPut, "/api/v1/updates/auto", b); code != http.StatusBadRequest {
			t.Errorf("%s: status %d, want 400 (%s)", b, code, body)
		}
	}

	code, body = suDo(t, rt, cookie, http.MethodPut, "/api/v1/updates/auto",
		`{"mode":"patch","window_cron":"0 3 * * *","window_duration_minutes":90,"timezone":"Europe/Berlin","min_release_age_days":2,"paused":false}`)
	if code != http.StatusOK {
		t.Fatalf("status %d: %s", code, body)
	}
	pol = decodeBody[upgradePolicyResource](t, body)
	if pol.Mode != "patch" || pol.WindowDurationMinutes != 90 || pol.Timezone != "Europe/Berlin" || pol.NextWindow == nil || pol.UpdatedBy == "" {
		t.Fatalf("policy = %+v", pol)
	}

	code, body = suDo(t, rt, cookie, http.MethodPut, "/api/v1/updates/auto",
		`{"mode":"patch","window_cron":"0 3 * * *","window_duration_minutes":90,"timezone":"Europe/Berlin","min_release_age_days":2,"paused":true}`)
	if pol = decodeBody[upgradePolicyResource](t, body); code != http.StatusOK || pol.Effective != "paused" || pol.Mode != "patch" {
		t.Fatalf("pause must keep the mode and report paused: %d %+v", code, pol)
	}
}

func TestUpgradeSchedulingRoutesRequireAuth(t *testing.T) {
	rt, _, _, _ := newScheduleRouter(t)
	assertRoutesRequireAuth(t, rt, []routeCase{
		{http.MethodPost, "/api/v1/updates/schedule"},
		{http.MethodDelete, "/api/v1/updates/schedule"},
		{http.MethodGet, "/api/v1/updates/schedule"},
		{http.MethodGet, "/api/v1/updates/auto"},
		{http.MethodPut, "/api/v1/updates/auto"},
		{http.MethodPost, "/api/v1/updates/backup-now"},
	})
}
