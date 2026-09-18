package alerting

import (
	"context"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

// fakeBackupSource is an in-memory BackupSource, the same
// hand-written-fake pattern every other package in this codebase uses
// instead of a mocking framework.
type fakeBackupSource struct {
	databases     map[string]store.DesiredDatabase
	volumes       map[string]store.ServiceVolumeBackupConfig
	dbHistory     map[string][]store.BackupHistory
	volumeHistory map[string][]store.BackupHistory
}

func newFakeBackupSource() *fakeBackupSource {
	return &fakeBackupSource{
		databases:     make(map[string]store.DesiredDatabase),
		volumes:       make(map[string]store.ServiceVolumeBackupConfig),
		dbHistory:     make(map[string][]store.BackupHistory),
		volumeHistory: make(map[string][]store.BackupHistory),
	}
}

func (f *fakeBackupSource) GetDesiredDatabase(_ context.Context, name string) (*store.DesiredDatabase, error) {
	d, ok := f.databases[name]
	if !ok {
		return nil, store.ErrDatabaseNotFound
	}
	return &d, nil
}

func (f *fakeBackupSource) ListBackupHistory(_ context.Context, databaseName string, limit int, _ *time.Time) ([]store.BackupHistory, error) {
	h := f.dbHistory[databaseName]
	if len(h) > limit {
		h = h[:limit]
	}
	return h, nil
}

func (f *fakeBackupSource) GetServiceVolumeBackupSchedule(_ context.Context, serviceName, volumeName string) (store.ServiceVolumeBackupConfig, error) {
	c, ok := f.volumes[volumeScheduleKeyForTest(serviceName, volumeName)]
	if !ok {
		return store.ServiceVolumeBackupConfig{}, store.ErrServiceVolumeBackupNotFound
	}
	return c, nil
}

func (f *fakeBackupSource) ListServiceVolumeBackupHistory(_ context.Context, serviceName, volumeName string, limit int, _ *time.Time) ([]store.BackupHistory, error) {
	h := f.volumeHistory[volumeScheduleKeyForTest(serviceName, volumeName)]
	if len(h) > limit {
		h = h[:limit]
	}
	return h, nil
}

func volumeScheduleKeyForTest(serviceName, volumeName string) string {
	return serviceName + "/" + volumeName
}

// dailyAt3AM is a fixed schedule every test below reuses: interval
// between consecutive fires is exactly 24h regardless of anchor, which
// is what makes the overdue-deadline arithmetic predictable to assert on.
const dailyAt3AM = "0 3 * * *"

func TestEvaluateBackupMissing_NoDatabase_NotFiringNoError(t *testing.T) {
	source := newFakeBackupSource()
	r := Rule{ID: "r1", Kind: KindBackupMissing, BackupResourceKind: store.BackupResourceKindDatabase, BackupDatabaseName: "gone", Enabled: true}

	got, notice, err := EvaluateBackupMissing(context.Background(), source, r, 0, time.Now())
	if err != nil {
		t.Fatalf("EvaluateBackupMissing() error = %v, want nil for a deleted database", err)
	}
	if got.Firing {
		t.Error("Firing = true, want false: watched database no longer exists")
	}
	if notice != "" {
		t.Errorf("notice = %q, want empty", notice)
	}
}

func TestEvaluateBackupMissing_NoScheduleConfigured_NotFiring(t *testing.T) {
	source := newFakeBackupSource()
	source.databases["main"] = store.DesiredDatabase{Name: "main", BackupSchedule: ""}
	r := Rule{ID: "r1", Kind: KindBackupMissing, BackupResourceKind: store.BackupResourceKindDatabase, BackupDatabaseName: "main", Enabled: true}

	got, notice, err := EvaluateBackupMissing(context.Background(), source, r, 0, time.Now())
	if err != nil {
		t.Fatalf("EvaluateBackupMissing() error = %v", err)
	}
	if got.Firing {
		t.Error("Firing = true, want false: no schedule configured, nothing expected")
	}
	if notice != "" {
		t.Errorf("notice = %q, want empty", notice)
	}
}

func TestEvaluateBackupMissing_NoHistoryYet_NotFiring(t *testing.T) {
	source := newFakeBackupSource()
	source.databases["main"] = store.DesiredDatabase{Name: "main", BackupSchedule: dailyAt3AM}
	r := Rule{ID: "r1", Kind: KindBackupMissing, BackupResourceKind: store.BackupResourceKindDatabase, BackupDatabaseName: "main", Enabled: true}

	got, notice, err := EvaluateBackupMissing(context.Background(), source, r, 0, time.Now())
	if err != nil {
		t.Fatalf("EvaluateBackupMissing() error = %v", err)
	}
	if got.Firing {
		t.Error("Firing = true, want false: schedule just created, nothing had a chance to run yet")
	}
	if notice != "" {
		t.Errorf("notice = %q, want empty", notice)
	}
}

func TestEvaluateBackupMissing_RecentSuccess_NotFiring(t *testing.T) {
	now := time.Now().UTC()
	source := newFakeBackupSource()
	source.databases["main"] = store.DesiredDatabase{Name: "main", BackupSchedule: dailyAt3AM}
	source.dbHistory["main"] = []store.BackupHistory{
		{ID: "bkh_1", DatabaseName: "main", Status: store.BackupStatusSucceeded, StartedAt: now.Add(-2 * time.Hour).Format(time.RFC3339)},
	}
	r := Rule{ID: "r1", Kind: KindBackupMissing, BackupResourceKind: store.BackupResourceKindDatabase, BackupDatabaseName: "main", Enabled: true}

	got, notice, err := EvaluateBackupMissing(context.Background(), source, r, time.Hour, now)
	if err != nil {
		t.Fatalf("EvaluateBackupMissing() error = %v", err)
	}
	if got.Firing {
		t.Error("Firing = true, want false: last success is well within the schedule's own interval")
	}
	if notice != "" {
		t.Errorf("notice = %q, want empty", notice)
	}
	if got.LastValue == nil {
		t.Fatal("LastValue is nil, want the hours since last success")
	}
}

func TestEvaluateBackupMissing_StaleSuccess_Fires(t *testing.T) {
	now := time.Now().UTC()
	source := newFakeBackupSource()
	source.databases["main"] = store.DesiredDatabase{Name: "main", BackupSchedule: dailyAt3AM}
	// Last success was 3 days ago: well past a daily schedule's own
	// 24h interval plus any reasonable grace period.
	source.dbHistory["main"] = []store.BackupHistory{
		{ID: "bkh_1", DatabaseName: "main", Status: store.BackupStatusSucceeded, StartedAt: now.Add(-72 * time.Hour).Format(time.RFC3339)},
	}
	r := Rule{ID: "r1", Kind: KindBackupMissing, BackupResourceKind: store.BackupResourceKindDatabase, BackupDatabaseName: "main", Enabled: true}

	got, notice, err := EvaluateBackupMissing(context.Background(), source, r, time.Hour, now)
	if err != nil {
		t.Fatalf("EvaluateBackupMissing() error = %v", err)
	}
	if !got.Firing {
		t.Error("Firing = false, want true: last success is 3 days old against a daily schedule")
	}
	if notice == "" {
		t.Error("notice is empty, want a non-empty overdue summary when firing")
	}
}

func TestEvaluateBackupMissing_OnlyFailedAttempts_FiresUsingOldestAsAnchor(t *testing.T) {
	now := time.Now().UTC()
	source := newFakeBackupSource()
	source.databases["main"] = store.DesiredDatabase{Name: "main", BackupSchedule: dailyAt3AM}
	source.dbHistory["main"] = []store.BackupHistory{
		{ID: "bkh_3", DatabaseName: "main", Status: store.BackupStatusFailed, StartedAt: now.Add(-1 * time.Hour).Format(time.RFC3339)},
		{ID: "bkh_2", DatabaseName: "main", Status: store.BackupStatusFailed, StartedAt: now.Add(-25 * time.Hour).Format(time.RFC3339)},
		{ID: "bkh_1", DatabaseName: "main", Status: store.BackupStatusFailed, StartedAt: now.Add(-49 * time.Hour).Format(time.RFC3339)},
	}
	r := Rule{ID: "r1", Kind: KindBackupMissing, BackupResourceKind: store.BackupResourceKindDatabase, BackupDatabaseName: "main", Enabled: true}

	got, notice, err := EvaluateBackupMissing(context.Background(), source, r, time.Hour, now)
	if err != nil {
		t.Fatalf("EvaluateBackupMissing() error = %v", err)
	}
	if !got.Firing {
		t.Error("Firing = false, want true: every recent attempt failed, anchored on the oldest one")
	}
	if notice == "" {
		t.Error("notice is empty, want a non-empty summary when firing")
	}
}

// TestEvaluateBackupMissing_PerRuleForDurationOverridesDefault checks
// that r.ForDuration, when set, takes priority over the passed-in
// engine-wide default grace period (Rule's own doc comment on why
// ForDuration is reused for this kind).
func TestEvaluateBackupMissing_PerRuleForDurationOverridesDefault(t *testing.T) {
	now := time.Now().UTC()
	source := newFakeBackupSource()
	source.databases["main"] = store.DesiredDatabase{Name: "main", BackupSchedule: dailyAt3AM}
	// Last success 25h ago: 1h past the daily schedule's own 24h
	// interval. A 1h grace period should already be enough to fire; a
	// 48h grace period should not.
	source.dbHistory["main"] = []store.BackupHistory{
		{ID: "bkh_1", DatabaseName: "main", Status: store.BackupStatusSucceeded, StartedAt: now.Add(-25 * time.Hour).Format(time.RFC3339)},
	}

	tightRule := Rule{ID: "r1", Kind: KindBackupMissing, BackupResourceKind: store.BackupResourceKindDatabase,
		BackupDatabaseName: "main", ForDuration: time.Hour, Enabled: true}
	got, _, err := EvaluateBackupMissing(context.Background(), source, tightRule, 48*time.Hour, now)
	if err != nil {
		t.Fatalf("EvaluateBackupMissing() error = %v", err)
	}
	if !got.Firing {
		t.Error("Firing = false, want true: rule's own 1h ForDuration should override the 48h default and fire")
	}

	looseRule := Rule{ID: "r2", Kind: KindBackupMissing, BackupResourceKind: store.BackupResourceKindDatabase,
		BackupDatabaseName: "main", ForDuration: 48 * time.Hour, Enabled: true}
	got2, _, err := EvaluateBackupMissing(context.Background(), source, looseRule, time.Hour, now)
	if err != nil {
		t.Fatalf("EvaluateBackupMissing() error = %v", err)
	}
	if got2.Firing {
		t.Error("Firing = true, want false: rule's own 48h ForDuration should override the 1h default and not fire yet")
	}
}

func TestEvaluateBackupMissing_ServiceVolume_Fires(t *testing.T) {
	now := time.Now().UTC()
	source := newFakeBackupSource()
	source.volumes[volumeScheduleKeyForTest("web", "uploads")] = store.ServiceVolumeBackupConfig{
		ServiceName: "web", VolumeName: "uploads", BackupSchedule: dailyAt3AM,
	}
	source.volumeHistory[volumeScheduleKeyForTest("web", "uploads")] = []store.BackupHistory{
		{ID: "bkh_1", ResourceKind: store.BackupResourceKindVolume, ServiceName: "web", VolumeName: "uploads",
			Status: store.BackupStatusSucceeded, StartedAt: now.Add(-72 * time.Hour).Format(time.RFC3339)},
	}
	r := Rule{ID: "r1", Kind: KindBackupMissing, BackupResourceKind: store.BackupResourceKindVolume,
		BackupServiceName: "web", BackupVolumeName: "uploads", Enabled: true}

	got, notice, err := EvaluateBackupMissing(context.Background(), source, r, time.Hour, now)
	if err != nil {
		t.Fatalf("EvaluateBackupMissing() error = %v", err)
	}
	if !got.Firing {
		t.Error("Firing = false, want true: last success is 3 days old against a daily schedule")
	}
	if notice == "" {
		t.Error("notice is empty, want a non-empty overdue summary when firing")
	}
}

func TestEvaluateBackupMissing_VolumeScheduleNeverConfigured_NotFiringNoError(t *testing.T) {
	source := newFakeBackupSource()
	r := Rule{ID: "r1", Kind: KindBackupMissing, BackupResourceKind: store.BackupResourceKindVolume,
		BackupServiceName: "web", BackupVolumeName: "uploads", Enabled: true}

	got, notice, err := EvaluateBackupMissing(context.Background(), source, r, 0, time.Now())
	if err != nil {
		t.Fatalf("EvaluateBackupMissing() error = %v, want nil when the volume's schedule was never configured", err)
	}
	if got.Firing {
		t.Error("Firing = true, want false")
	}
	if notice != "" {
		t.Errorf("notice = %q, want empty", notice)
	}
}

func TestEngine_Tick_BackupMissingFires_NotifiesOnce(t *testing.T) {
	now := time.Now().UTC()
	source := newFakeBackupSource()
	source.databases["main"] = store.DesiredDatabase{Name: "main", BackupSchedule: dailyAt3AM}
	source.dbHistory["main"] = []store.BackupHistory{
		{ID: "bkh_1", DatabaseName: "main", Status: store.BackupStatusSucceeded, StartedAt: now.Add(-72 * time.Hour).Format(time.RFC3339)},
	}
	r := Rule{ID: "r1", Kind: KindBackupMissing, ResourceID: "service:web", BackupResourceKind: store.BackupResourceKindDatabase,
		BackupDatabaseName: "main", ForDuration: time.Hour, Enabled: true}
	rules := newFakeRuleStore(r)
	spy := &spyNotifier{}
	engine := newTestEngineWithBackups(rules, source, time.Hour, spy)

	if err := engine.Tick(context.Background()); err != nil {
		t.Fatalf("Tick() error = %v", err)
	}

	calls := spy.calls()
	if len(calls) != 1 {
		t.Fatalf("Notify called %d times, want 1", len(calls))
	}
	if calls[0].BackupMissingNotice == "" {
		t.Error("BackupMissingNotice is empty on a firing event, want a non-empty summary")
	}
	if !rules.get("r1").Firing {
		t.Error("persisted state Firing = false, want true")
	}
}

func TestEngine_Tick_BackupMissingResolved_NoNotice(t *testing.T) {
	now := time.Now().UTC()
	source := newFakeBackupSource()
	source.databases["main"] = store.DesiredDatabase{Name: "main", BackupSchedule: dailyAt3AM}
	source.dbHistory["main"] = []store.BackupHistory{
		{ID: "bkh_1", DatabaseName: "main", Status: store.BackupStatusSucceeded, StartedAt: now.Add(-1 * time.Hour).Format(time.RFC3339)},
	}
	firingSince := now.Add(-time.Hour)
	r := Rule{ID: "r1", Kind: KindBackupMissing, ResourceID: "service:web", BackupResourceKind: store.BackupResourceKindDatabase,
		BackupDatabaseName: "main", Enabled: true, Firing: true, FiringSince: &firingSince}
	rules := newFakeRuleStore(r)
	spy := &spyNotifier{}
	engine := newTestEngineWithBackups(rules, source, time.Hour, spy)

	if err := engine.Tick(context.Background()); err != nil {
		t.Fatalf("Tick() error = %v", err)
	}

	calls := spy.calls()
	if len(calls) != 1 || !calls[0].Resolved {
		t.Fatalf("calls = %+v, want one Resolved=true event", calls)
	}
	if calls[0].BackupMissingNotice != "" {
		t.Errorf("BackupMissingNotice = %q, want empty on a resolved event", calls[0].BackupMissingNotice)
	}
}

func TestEngine_Tick_BackupMissing_NoSourceConfigured_Skipped(t *testing.T) {
	r := Rule{ID: "r1", Kind: KindBackupMissing, ResourceID: "service:web", BackupResourceKind: store.BackupResourceKindDatabase,
		BackupDatabaseName: "main", Enabled: true}
	rules := newFakeRuleStore(r)
	spy := &spyNotifier{}
	engine := newTestEngine(rules, nil, nil, nil, spy) // no backup source wired

	if err := engine.Tick(context.Background()); err != nil {
		t.Fatalf("Tick() error = %v", err)
	}
	if calls := spy.calls(); len(calls) != 0 {
		t.Errorf("Notify called %d times with no backup source configured, want 0", len(calls))
	}
}
