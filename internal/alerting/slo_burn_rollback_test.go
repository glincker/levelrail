package alerting

import (
	"context"
	"errors"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

// fakeSLOAutoRollbackStore extends fakeAutoRollbackStore (crashloop_test.go)
// with SaveDeployApproval, the extra surface SLOAutoRollbackStore needs
// for pause_for_human mode.
type fakeSLOAutoRollbackStore struct {
	fakeAutoRollbackStore
	savedApprovals  []store.DeployApproval
	saveApprovalErr error
}

func (f *fakeSLOAutoRollbackStore) SaveDeployApproval(_ context.Context, a store.DeployApproval) error {
	f.savedApprovals = append(f.savedApprovals, a)
	return f.saveApprovalErr
}

// fakeSLOBurnHistoryRecorder is an in-memory SLOBurnHistoryRecorder for
// dry-run mode's own tests.
type fakeSLOBurnHistoryRecorder struct {
	entries   []HistoryEntry
	recordErr error
}

func (f *fakeSLOBurnHistoryRecorder) RecordHistory(_ context.Context, e HistoryEntry) error {
	f.entries = append(f.entries, e)
	return f.recordErr
}

func sloBurnTestRule() Rule {
	return Rule{ID: "rule_1", Name: "web p99 burn", Kind: KindSLOBurn, ResourceID: "service:web", Severity: SeverityCritical}
}

func TestMaybeAutoRollbackOnSLOBurn_Auto_RollsBackToPreviousImage(t *testing.T) {
	st := &fakeSLOAutoRollbackStore{fakeAutoRollbackStore: fakeAutoRollbackStore{
		svc: store.DesiredService{Name: "web", Image: "web:v3", AutoRollbackOnSLOBurn: store.AutoRollbackSLOBurnAuto},
		attempts: []store.DeployAttempt{
			{Image: "web:v3", Status: store.DeployAttemptStatusSucceeded},
			{Image: "web:v2", Status: store.DeployAttemptStatusSucceeded},
		},
	}}
	nudger := &fakeAutoRollbackNudger{}
	tracker := NewAutoRollbackTracker()

	MaybeAutoRollbackOnSLOBurn(context.Background(), st, nil, nudger, tracker, sloBurnTestRule(), "SLO notice", nil)

	if len(st.savedServices) != 1 || st.savedServices[0].Image != "web:v2" {
		t.Fatalf("savedServices = %+v, want one save with image web:v2", st.savedServices)
	}
	if len(st.savedAttempts) != 1 || st.savedAttempts[0].Source != store.DeployAttemptSourceAutoRollback {
		t.Fatalf("savedAttempts = %+v, want one with Source=%q", st.savedAttempts, store.DeployAttemptSourceAutoRollback)
	}
	if nudger.calls != 1 {
		t.Errorf("nudger.calls = %d, want 1", nudger.calls)
	}
	if len(st.savedApprovals) != 0 {
		t.Errorf("savedApprovals = %+v, want none in auto mode", st.savedApprovals)
	}
	if !tracker.alreadyHandled("service:web", "web:v2") {
		t.Error("tracker did not record the rollback target")
	}
}

// TestMaybeAutoRollbackOnSLOBurn_Auto_TriggerFails_HalfSucceeded covers
// the half-succeeded case CLAUDE.md requires: TriggerImageDeploy fails
// mid-rollback (its own SaveDesiredService call failing, the earliest
// possible failure point) so desired state never moves. The tracker must
// not record a rollback that never happened, or a later, real attempt
// would be wrongly suppressed as "already handled."
func TestMaybeAutoRollbackOnSLOBurn_Auto_TriggerFails_HalfSucceeded(t *testing.T) {
	st := &fakeSLOAutoRollbackStore{fakeAutoRollbackStore: fakeAutoRollbackStore{
		svc: store.DesiredService{Name: "web", Image: "web:v3", AutoRollbackOnSLOBurn: store.AutoRollbackSLOBurnAuto},
		attempts: []store.DeployAttempt{
			{Image: "web:v3", Status: store.DeployAttemptStatusSucceeded},
			{Image: "web:v2", Status: store.DeployAttemptStatusSucceeded},
		},
		saveErr: errors.New("db unavailable"),
	}}
	nudger := &fakeAutoRollbackNudger{}
	tracker := NewAutoRollbackTracker()

	MaybeAutoRollbackOnSLOBurn(context.Background(), st, nil, nudger, tracker, sloBurnTestRule(), "", nil)

	if len(st.savedServices) != 0 {
		t.Errorf("savedServices = %+v, want none: SaveDesiredService failed", st.savedServices)
	}
	if nudger.calls != 0 {
		t.Errorf("nudger.calls = %d, want 0: desired state never moved", nudger.calls)
	}
	if tracker.alreadyHandled("service:web", "web:v2") {
		t.Error("tracker recorded a rollback that never happened; a genuine retry would be wrongly suppressed")
	}
}

// TestMaybeAutoRollbackOnSLOBurn_Auto_AttemptRecordFails_StillMovesAndRecords
// mirrors TestMaybeAutoRollback_AttemptRecordFails_DesiredStateStillMovesAndTrackerStillRecords:
// once the desired-state write lands, a later SaveDeployAttempt failure
// (logged only, never returned by recordImageDeployAttempt) must not stop
// the reconciler nudge or the tracker record.
func TestMaybeAutoRollbackOnSLOBurn_Auto_AttemptRecordFails_StillMovesAndRecords(t *testing.T) {
	st := &fakeSLOAutoRollbackStore{fakeAutoRollbackStore: fakeAutoRollbackStore{
		svc: store.DesiredService{Name: "web", Image: "web:v3", AutoRollbackOnSLOBurn: store.AutoRollbackSLOBurnAuto},
		attempts: []store.DeployAttempt{
			{Image: "web:v3", Status: store.DeployAttemptStatusSucceeded},
			{Image: "web:v2", Status: store.DeployAttemptStatusSucceeded},
		},
		saveAttemptErr: errors.New("db unavailable"),
	}}
	nudger := &fakeAutoRollbackNudger{}
	tracker := NewAutoRollbackTracker()

	MaybeAutoRollbackOnSLOBurn(context.Background(), st, nil, nudger, tracker, sloBurnTestRule(), "", nil)

	if len(st.savedServices) != 1 || st.savedServices[0].Image != "web:v2" {
		t.Fatalf("savedServices = %+v, want one save with image web:v2 despite the attempt-record failure", st.savedServices)
	}
	if nudger.calls != 1 {
		t.Errorf("nudger.calls = %d, want 1: desired state landed, the reconciler must still be nudged", nudger.calls)
	}
	if !tracker.alreadyHandled("service:web", "web:v2") {
		t.Error("tracker did not record the rollback: a retry on the next tick would fire again for the same incident")
	}
}

func TestMaybeAutoRollbackOnSLOBurn_DryRun_RecordsHistoryNeverDeploys(t *testing.T) {
	st := &fakeSLOAutoRollbackStore{fakeAutoRollbackStore: fakeAutoRollbackStore{
		svc: store.DesiredService{Name: "web", Image: "web:v3", AutoRollbackOnSLOBurn: store.AutoRollbackSLOBurnDryRun},
		attempts: []store.DeployAttempt{
			{Image: "web:v3", Status: store.DeployAttemptStatusSucceeded},
			{Image: "web:v2", Status: store.DeployAttemptStatusSucceeded},
		},
	}}
	nudger := &fakeAutoRollbackNudger{}
	history := &fakeSLOBurnHistoryRecorder{}
	tracker := NewAutoRollbackTracker()

	MaybeAutoRollbackOnSLOBurn(context.Background(), st, history, nudger, tracker, sloBurnTestRule(), "burning 12x", nil)

	if len(st.savedServices) != 0 {
		t.Errorf("savedServices = %+v, want none: dry_run must never deploy", st.savedServices)
	}
	if nudger.calls != 0 {
		t.Errorf("nudger.calls = %d, want 0: dry_run must never nudge the reconciler", nudger.calls)
	}
	if len(st.savedApprovals) != 0 {
		t.Errorf("savedApprovals = %+v, want none in dry_run mode", st.savedApprovals)
	}
	if len(history.entries) != 1 {
		t.Fatalf("history.entries = %+v, want exactly one recorded event", history.entries)
	}
	e := history.entries[0]
	if e.Event != EventSLOBurnWouldRollback || e.Outcome != OutcomeSkipped || e.ResourceID != "service:web" {
		t.Errorf("history entry = %+v, want event=%q outcome=%q resource_id=service:web", e, EventSLOBurnWouldRollback, OutcomeSkipped)
	}
	// Recorded against the unchanged current image so a second tick
	// while still firing does not log the same incident again.
	if !tracker.alreadyHandled("service:web", "web:v3") {
		t.Error("tracker did not record the dry-run against the current image")
	}
}

func TestMaybeAutoRollbackOnSLOBurn_DryRun_StillFiring_LogsOnceNotEveryTick(t *testing.T) {
	st := &fakeSLOAutoRollbackStore{fakeAutoRollbackStore: fakeAutoRollbackStore{
		svc: store.DesiredService{Name: "web", Image: "web:v3", AutoRollbackOnSLOBurn: store.AutoRollbackSLOBurnDryRun},
		attempts: []store.DeployAttempt{
			{Image: "web:v3", Status: store.DeployAttemptStatusSucceeded},
			{Image: "web:v2", Status: store.DeployAttemptStatusSucceeded},
		},
	}}
	history := &fakeSLOBurnHistoryRecorder{}
	tracker := NewAutoRollbackTracker()
	rule := sloBurnTestRule()

	MaybeAutoRollbackOnSLOBurn(context.Background(), st, history, nil, tracker, rule, "", nil)
	MaybeAutoRollbackOnSLOBurn(context.Background(), st, history, nil, tracker, rule, "", nil)

	if len(history.entries) != 1 {
		t.Errorf("history.entries = %d, want 1: desired state never changed, the second tick is the same incident", len(history.entries))
	}
}

func TestMaybeAutoRollbackOnSLOBurn_PauseForHuman_OpensApprovalNeverDeploys(t *testing.T) {
	st := &fakeSLOAutoRollbackStore{fakeAutoRollbackStore: fakeAutoRollbackStore{
		svc: store.DesiredService{Name: "web", Image: "web:v3", EnvironmentID: "env_prod", AutoRollbackOnSLOBurn: store.AutoRollbackSLOBurnPauseForHuman},
		attempts: []store.DeployAttempt{
			{Image: "web:v3", Status: store.DeployAttemptStatusSucceeded},
			{Image: "web:v2", Status: store.DeployAttemptStatusSucceeded},
		},
	}}
	nudger := &fakeAutoRollbackNudger{}
	tracker := NewAutoRollbackTracker()

	MaybeAutoRollbackOnSLOBurn(context.Background(), st, nil, nudger, tracker, sloBurnTestRule(), "", nil)

	if len(st.savedServices) != 0 {
		t.Errorf("savedServices = %+v, want none: pause_for_human must never deploy directly", st.savedServices)
	}
	if nudger.calls != 0 {
		t.Errorf("nudger.calls = %d, want 0", nudger.calls)
	}
	if len(st.savedApprovals) != 1 {
		t.Fatalf("savedApprovals = %+v, want exactly one pending approval", st.savedApprovals)
	}
	a := st.savedApprovals[0]
	if a.ServiceName != "web" || a.Image != "web:v2" || a.Action != store.DeployApprovalActionDeploy {
		t.Errorf("approval = %+v, want service=web image=web:v2 action=deploy", a)
	}
	if a.Status != store.DeployApprovalStatusPending {
		t.Errorf("approval.Status = %q, want %q", a.Status, store.DeployApprovalStatusPending)
	}
	if a.EnvironmentID != "env_prod" {
		t.Errorf("approval.EnvironmentID = %q, want env_prod (carried over from the app's own desired state)", a.EnvironmentID)
	}
	if !tracker.alreadyHandled("service:web", "web:v3") {
		t.Error("tracker did not record the pause against the current image")
	}
}

func TestMaybeAutoRollbackOnSLOBurn_PauseForHuman_SaveFails_TrackerNotRecorded(t *testing.T) {
	st := &fakeSLOAutoRollbackStore{
		fakeAutoRollbackStore: fakeAutoRollbackStore{
			svc: store.DesiredService{Name: "web", Image: "web:v3", AutoRollbackOnSLOBurn: store.AutoRollbackSLOBurnPauseForHuman},
			attempts: []store.DeployAttempt{
				{Image: "web:v3", Status: store.DeployAttemptStatusSucceeded},
				{Image: "web:v2", Status: store.DeployAttemptStatusSucceeded},
			},
		},
		saveApprovalErr: errors.New("db unavailable"),
	}
	tracker := NewAutoRollbackTracker()

	MaybeAutoRollbackOnSLOBurn(context.Background(), st, nil, nil, tracker, sloBurnTestRule(), "", nil)

	if tracker.alreadyHandled("service:web", "web:v3") {
		t.Error("tracker recorded a pause request that failed to save; a retry would be wrongly suppressed")
	}
}

func TestMaybeAutoRollbackOnSLOBurn_Off_NeverFires(t *testing.T) {
	st := &fakeSLOAutoRollbackStore{fakeAutoRollbackStore: fakeAutoRollbackStore{
		svc: store.DesiredService{Name: "web", Image: "web:v3", AutoRollbackOnSLOBurn: store.AutoRollbackSLOBurnOff},
		attempts: []store.DeployAttempt{
			{Image: "web:v2", Status: store.DeployAttemptStatusSucceeded},
		},
	}}

	MaybeAutoRollbackOnSLOBurn(context.Background(), st, nil, nil, nil, sloBurnTestRule(), "", nil)

	if len(st.savedServices) != 0 || len(st.savedApprovals) != 0 {
		t.Errorf("st = %+v, want no action: auto-rollback on SLO burn is off for this app", st)
	}
}

func TestMaybeAutoRollbackOnSLOBurn_EmptyMode_TreatedAsOff(t *testing.T) {
	st := &fakeSLOAutoRollbackStore{fakeAutoRollbackStore: fakeAutoRollbackStore{
		svc: store.DesiredService{Name: "web", Image: "web:v3", AutoRollbackOnSLOBurn: ""},
		attempts: []store.DeployAttempt{
			{Image: "web:v2", Status: store.DeployAttemptStatusSucceeded},
		},
	}}

	MaybeAutoRollbackOnSLOBurn(context.Background(), st, nil, nil, nil, sloBurnTestRule(), "", nil)

	if len(st.savedServices) != 0 || len(st.savedApprovals) != 0 {
		t.Errorf("st = %+v, want no action for an unset mode (defaults to off)", st)
	}
}

func TestMaybeAutoRollbackOnSLOBurn_NoOlderImage_NoPanicNoAction(t *testing.T) {
	st := &fakeSLOAutoRollbackStore{fakeAutoRollbackStore: fakeAutoRollbackStore{
		svc: store.DesiredService{Name: "web", Image: "web:v1", AutoRollbackOnSLOBurn: store.AutoRollbackSLOBurnAuto},
		attempts: []store.DeployAttempt{
			{Image: "web:v1", Status: store.DeployAttemptStatusSucceeded},
		},
	}}

	MaybeAutoRollbackOnSLOBurn(context.Background(), st, nil, nil, nil, sloBurnTestRule(), "", nil)

	if len(st.savedServices) != 0 {
		t.Errorf("savedServices = %+v, want none: already on the oldest known image", st.savedServices)
	}
}

func TestMaybeAutoRollbackOnSLOBurn_NotServiceScopedResourceID_NoOp(t *testing.T) {
	st := &fakeSLOAutoRollbackStore{fakeAutoRollbackStore: fakeAutoRollbackStore{
		svc: store.DesiredService{Name: "web", Image: "web:v1", AutoRollbackOnSLOBurn: store.AutoRollbackSLOBurnAuto},
	}}
	rule := sloBurnTestRule()
	rule.ResourceID = "node:some-node-id"

	MaybeAutoRollbackOnSLOBurn(context.Background(), st, nil, nil, nil, rule, "", nil)

	if len(st.savedServices) != 0 {
		t.Errorf("savedServices = %+v, want none for a non-service resourceID", st.savedServices)
	}
}

func TestMaybeAutoRollbackOnSLOBurn_Rearm_NewDeployAfterAction_FiresAgain(t *testing.T) {
	st := &fakeSLOAutoRollbackStore{fakeAutoRollbackStore: fakeAutoRollbackStore{
		svc: store.DesiredService{Name: "web", Image: "web:v3", AutoRollbackOnSLOBurn: store.AutoRollbackSLOBurnDryRun},
		attempts: []store.DeployAttempt{
			{Image: "web:v3", Status: store.DeployAttemptStatusSucceeded},
			{Image: "web:v2", Status: store.DeployAttemptStatusSucceeded},
		},
	}}
	history := &fakeSLOBurnHistoryRecorder{}
	tracker := NewAutoRollbackTracker()
	rule := sloBurnTestRule()

	MaybeAutoRollbackOnSLOBurn(context.Background(), st, history, nil, tracker, rule, "", nil)
	if len(history.entries) != 1 {
		t.Fatalf("history.entries after first tick = %d, want 1", len(history.entries))
	}

	// A fresh deploy lands (a new, different bad image), the same "the
	// operator or another path moved desired state since" case
	// AutoRollbackTracker's own doc comment describes for crashloop.
	st.svc.Image = "web:v4"
	st.attempts = append([]store.DeployAttempt{{Image: "web:v4", Status: store.DeployAttemptStatusSucceeded}}, st.attempts...)

	MaybeAutoRollbackOnSLOBurn(context.Background(), st, history, nil, tracker, rule, "", nil)
	if len(history.entries) != 2 {
		t.Errorf("history.entries after second, genuinely new incident = %d, want 2", len(history.entries))
	}
}
