package alerting

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"
)

func newTestDB(t *testing.T) *DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "alerting.db")
	db, err := Open(context.Background(), path)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestNewRuleID_UniqueAndNonEmpty(t *testing.T) {
	a, err := NewRuleID()
	if err != nil {
		t.Fatalf("NewRuleID() error = %v", err)
	}
	b, err := NewRuleID()
	if err != nil {
		t.Fatalf("NewRuleID() error = %v", err)
	}
	if a == "" || b == "" {
		t.Fatal("NewRuleID() returned an empty string")
	}
	if a == b {
		t.Error("two calls to NewRuleID() returned the same ID")
	}
}

func TestSaveAndGetRule_ThresholdRoundTrip(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()

	r := Rule{
		ID: "r1", Name: "high cpu", Kind: KindThreshold, ResourceID: "service:web",
		Metric: "cpu_percent", Comparator: GreaterThan, Threshold: 80, ForDuration: 2 * time.Minute,
		NotifyURL: "https://example.com/hook", NotifyKind: NotifySlack, Enabled: true,
	}
	if err := db.SaveRule(ctx, r); err != nil {
		t.Fatalf("SaveRule() error = %v", err)
	}

	got, err := db.GetRule(ctx, "r1")
	if err != nil {
		t.Fatalf("GetRule() error = %v", err)
	}
	if got.Name != r.Name || got.Kind != r.Kind || got.ResourceID != r.ResourceID {
		t.Errorf("got = %+v, want matching identity fields from %+v", got, r)
	}
	if got.Metric != r.Metric || got.Comparator != r.Comparator || got.Threshold != r.Threshold || got.ForDuration != r.ForDuration {
		t.Errorf("got threshold fields = %+v, want matching %+v", got, r)
	}
	if got.NotifyURL != r.NotifyURL || got.NotifyKind != r.NotifyKind || got.Enabled != r.Enabled {
		t.Errorf("got notify fields = %+v, want matching %+v", got, r)
	}
	if got.Firing || got.PendingSince != nil || got.FiringSince != nil {
		t.Errorf("got = %+v, want zero-value evaluation state on a freshly saved rule", got)
	}
}

func TestSaveAndGetRule_CrashloopRoundTrip(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()

	r := Rule{
		ID: "cl1", Name: "web crashlooping", Kind: KindCrashloop, ResourceID: "service:web",
		RestartCountThreshold: 3, RestartWindow: 5 * time.Minute,
		NotifyURL: "https://example.com/hook", NotifyKind: NotifyDiscord, Enabled: true,
	}
	if err := db.SaveRule(ctx, r); err != nil {
		t.Fatalf("SaveRule() error = %v", err)
	}

	got, err := db.GetRule(ctx, "cl1")
	if err != nil {
		t.Fatalf("GetRule() error = %v", err)
	}
	if got.RestartCountThreshold != 3 || got.RestartWindow != 5*time.Minute {
		t.Errorf("got = %+v, want RestartCountThreshold=3 RestartWindow=5m", got)
	}
}

func TestGetRule_NotFound(t *testing.T) {
	db := newTestDB(t)
	_, err := db.GetRule(context.Background(), "nonexistent")
	if !errors.Is(err, ErrRuleNotFound) {
		t.Errorf("GetRule() error = %v, want ErrRuleNotFound", err)
	}
}

func TestSaveRule_SameID_Replaces(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()

	r := Rule{ID: "r1", Name: "v1", Kind: KindThreshold, ResourceID: "service:web", Metric: "cpu_percent", Comparator: GreaterThan, Threshold: 50, Enabled: true}
	if err := db.SaveRule(ctx, r); err != nil {
		t.Fatalf("first SaveRule() error = %v", err)
	}
	r.Name = "v2"
	r.Threshold = 90
	if err := db.SaveRule(ctx, r); err != nil {
		t.Fatalf("second SaveRule() error = %v", err)
	}

	got, err := db.GetRule(ctx, "r1")
	if err != nil {
		t.Fatalf("GetRule() error = %v", err)
	}
	if got.Name != "v2" || got.Threshold != 90 {
		t.Errorf("got = %+v, want the replaced values (name=v2, threshold=90)", got)
	}

	all, err := db.ListRules(ctx)
	if err != nil {
		t.Fatalf("ListRules() error = %v", err)
	}
	if len(all) != 1 {
		t.Errorf("ListRules() = %d rules, want 1 (SaveRule with the same ID replaces, not duplicates)", len(all))
	}
}

func TestListEnabledRules_ExcludesDisabled(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()

	enabled := Rule{ID: "r1", Name: "on", Kind: KindThreshold, ResourceID: "service:web", Metric: "cpu_percent", Comparator: GreaterThan, Threshold: 50, Enabled: true}
	disabled := Rule{ID: "r2", Name: "off", Kind: KindThreshold, ResourceID: "service:web", Metric: "cpu_percent", Comparator: GreaterThan, Threshold: 50, Enabled: false}
	if err := db.SaveRule(ctx, enabled); err != nil {
		t.Fatalf("SaveRule(enabled) error = %v", err)
	}
	if err := db.SaveRule(ctx, disabled); err != nil {
		t.Fatalf("SaveRule(disabled) error = %v", err)
	}

	got, err := db.ListEnabledRules(ctx)
	if err != nil {
		t.Fatalf("ListEnabledRules() error = %v", err)
	}
	if len(got) != 1 || got[0].ID != "r1" {
		t.Errorf("ListEnabledRules() = %+v, want only the enabled rule", got)
	}

	all, err := db.ListRules(ctx)
	if err != nil {
		t.Fatalf("ListRules() error = %v", err)
	}
	if len(all) != 2 {
		t.Errorf("ListRules() = %d, want 2 (both, regardless of enabled)", len(all))
	}
}

func TestListRulesForResource_FiltersByResource(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()

	web1 := Rule{ID: "r1", Name: "web high cpu", Kind: KindThreshold, ResourceID: "service:web", Metric: "cpu_percent", Comparator: GreaterThan, Threshold: 80, Enabled: true}
	web2 := Rule{ID: "r2", Name: "web crashlooping", Kind: KindCrashloop, ResourceID: "service:web", RestartCountThreshold: 3, RestartWindow: 5 * time.Minute, Enabled: false}
	worker := Rule{ID: "r3", Name: "worker high cpu", Kind: KindThreshold, ResourceID: "service:worker", Metric: "cpu_percent", Comparator: GreaterThan, Threshold: 80, Enabled: true}
	for _, r := range []Rule{web1, web2, worker} {
		if err := db.SaveRule(ctx, r); err != nil {
			t.Fatalf("SaveRule(%s) error = %v", r.ID, err)
		}
	}

	got, err := db.ListRulesForResource(ctx, "service:web")
	if err != nil {
		t.Fatalf("ListRulesForResource() error = %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("ListRulesForResource(\"service:web\") = %d rules, want 2 (regardless of enabled)", len(got))
	}
	for _, r := range got {
		if r.ResourceID != "service:web" {
			t.Errorf("got rule %+v, want ResourceID = service:web", r)
		}
	}

	none, err := db.ListRulesForResource(ctx, "service:nonexistent")
	if err != nil {
		t.Fatalf("ListRulesForResource() error = %v", err)
	}
	if len(none) != 0 {
		t.Errorf("ListRulesForResource(\"service:nonexistent\") = %d rules, want 0", len(none))
	}
}

func TestDeleteRule(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()

	r := Rule{ID: "r1", Name: "x", Kind: KindThreshold, ResourceID: "service:web", Metric: "cpu_percent", Comparator: GreaterThan, Threshold: 50, Enabled: true}
	if err := db.SaveRule(ctx, r); err != nil {
		t.Fatalf("SaveRule() error = %v", err)
	}
	if err := db.DeleteRule(ctx, "r1"); err != nil {
		t.Fatalf("DeleteRule() error = %v", err)
	}
	if _, err := db.GetRule(ctx, "r1"); !errors.Is(err, ErrRuleNotFound) {
		t.Errorf("GetRule() after delete error = %v, want ErrRuleNotFound", err)
	}
}

func TestDeleteRule_NonexistentIsNotError(t *testing.T) {
	db := newTestDB(t)
	if err := db.DeleteRule(context.Background(), "never-existed"); err != nil {
		t.Errorf("DeleteRule() on a nonexistent rule error = %v, want nil (idempotent delete)", err)
	}
}

func TestUpdateState_PersistsAndDoesNotTouchConfig(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()

	r := Rule{ID: "r1", Name: "high cpu", Kind: KindThreshold, ResourceID: "service:web", Metric: "cpu_percent", Comparator: GreaterThan, Threshold: 80, Enabled: true}
	if err := db.SaveRule(ctx, r); err != nil {
		t.Fatalf("SaveRule() error = %v", err)
	}

	now := time.Now().UTC().Truncate(time.Millisecond)
	firingSince := now.Add(-time.Minute)
	value := 95.5
	if err := db.UpdateState(ctx, "r1", nil, &firingSince, true, now, &value); err != nil {
		t.Fatalf("UpdateState() error = %v", err)
	}

	got, err := db.GetRule(ctx, "r1")
	if err != nil {
		t.Fatalf("GetRule() error = %v", err)
	}
	if !got.Firing {
		t.Error("Firing = false, want true after UpdateState")
	}
	if got.FiringSince == nil || !got.FiringSince.Equal(firingSince) {
		t.Errorf("FiringSince = %v, want %v", got.FiringSince, firingSince)
	}
	if got.LastValue == nil || *got.LastValue != 95.5 {
		t.Errorf("LastValue = %v, want 95.5", got.LastValue)
	}
	// Config fields must survive an UpdateState call untouched.
	if got.Name != "high cpu" || got.Threshold != 80 {
		t.Errorf("got = %+v, want SaveRule's original config fields unchanged by UpdateState", got)
	}
}

func TestUpdateState_ClearsFiring(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()

	r := Rule{ID: "r1", Name: "x", Kind: KindThreshold, ResourceID: "service:web", Metric: "cpu_percent", Comparator: GreaterThan, Threshold: 80, Enabled: true}
	if err := db.SaveRule(ctx, r); err != nil {
		t.Fatalf("SaveRule() error = %v", err)
	}
	firingSince := time.Now().UTC()
	if err := db.UpdateState(ctx, "r1", nil, &firingSince, true, time.Now(), nil); err != nil {
		t.Fatalf("first UpdateState() error = %v", err)
	}

	if err := db.UpdateState(ctx, "r1", nil, nil, false, time.Now(), nil); err != nil {
		t.Fatalf("second UpdateState() error = %v", err)
	}

	got, err := db.GetRule(ctx, "r1")
	if err != nil {
		t.Fatalf("GetRule() error = %v", err)
	}
	if got.Firing || got.FiringSince != nil {
		t.Errorf("got = %+v, want Firing=false and FiringSince=nil after clearing", got)
	}
}
