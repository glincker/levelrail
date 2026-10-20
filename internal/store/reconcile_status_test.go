package store

import (
	"context"
	"testing"

	"github.com/GLINCKER/levelrail/internal/reconcile"
)

func TestUpsertAndGetConditions(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	got, err := db.GetConditions(ctx, "nginx-demo")
	if err != nil {
		t.Fatalf("GetConditions on empty table error = %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected no conditions before any upsert, got %d", len(got))
	}

	first := []reconcile.Condition{
		{Type: "Ready", Status: reconcile.ConditionTrue, Reason: "Created", Message: ""},
	}
	if err := db.UpsertConditions(ctx, "nginx-demo", first); err != nil {
		t.Fatalf("UpsertConditions() error = %v", err)
	}

	got, err = db.GetConditions(ctx, "nginx-demo")
	if err != nil {
		t.Fatalf("GetConditions() error = %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 condition, got %d", len(got))
	}
	if got[0].Type != "Ready" || got[0].Status != reconcile.ConditionTrue || got[0].Reason != "Created" {
		t.Errorf("got %+v, want Type=Ready Status=True Reason=Created", got[0])
	}
	if got[0].LastTransitionTime.IsZero() {
		t.Error("expected LastTransitionTime to be populated from updated_at, got zero value")
	}

	// A second reconcile with a different outcome must overwrite the row,
	// not add a second one, since the key is (controller, condition_type).
	second := []reconcile.Condition{
		{Type: "Ready", Status: reconcile.ConditionFalse, Reason: "StartFailed", Message: "boom"},
	}
	if err := db.UpsertConditions(ctx, "nginx-demo", second); err != nil {
		t.Fatalf("second UpsertConditions() error = %v", err)
	}

	got, err = db.GetConditions(ctx, "nginx-demo")
	if err != nil {
		t.Fatalf("GetConditions() after update error = %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected upsert to replace the existing row, not add one, got %d rows", len(got))
	}
	if got[0].Status != reconcile.ConditionFalse || got[0].Reason != "StartFailed" || got[0].Message != "boom" {
		t.Errorf("got %+v, want Status=False Reason=StartFailed Message=boom", got[0])
	}
}

func TestUpsertConditions_MultipleControllersDoNotCollide(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	if err := db.UpsertConditions(ctx, "nginx-demo", []reconcile.Condition{
		{Type: "Ready", Status: reconcile.ConditionTrue, Reason: "AlreadyRunning"},
	}); err != nil {
		t.Fatalf("upsert nginx-demo: %v", err)
	}
	if err := db.UpsertConditions(ctx, "other-controller", []reconcile.Condition{
		{Type: "Ready", Status: reconcile.ConditionFalse, Reason: "SomethingElse"},
	}); err != nil {
		t.Fatalf("upsert other-controller: %v", err)
	}

	a, err := db.GetConditions(ctx, "nginx-demo")
	if err != nil {
		t.Fatalf("GetConditions(nginx-demo): %v", err)
	}
	b, err := db.GetConditions(ctx, "other-controller")
	if err != nil {
		t.Fatalf("GetConditions(other-controller): %v", err)
	}

	if len(a) != 1 || a[0].Reason != "AlreadyRunning" {
		t.Errorf("nginx-demo conditions = %+v, want one condition with Reason=AlreadyRunning", a)
	}
	if len(b) != 1 || b[0].Reason != "SomethingElse" {
		t.Errorf("other-controller conditions = %+v, want one condition with Reason=SomethingElse", b)
	}
}

func TestUpsertConditions_MultipleConditionTypesForOneController(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	err := db.UpsertConditions(ctx, "example", []reconcile.Condition{
		{Type: "Ready", Status: reconcile.ConditionTrue, Reason: "Running"},
		{Type: "Progressing", Status: reconcile.ConditionFalse, Reason: "Settled"},
	})
	if err != nil {
		t.Fatalf("UpsertConditions() error = %v", err)
	}

	got, err := db.GetConditions(ctx, "example")
	if err != nil {
		t.Fatalf("GetConditions() error = %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 distinct condition types stored, got %d: %+v", len(got), got)
	}
}

func TestGetConditionsForControllers_Empty(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	got, err := db.GetConditionsForControllers(ctx, nil)
	if err != nil {
		t.Fatalf("GetConditionsForControllers(nil) error = %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected empty map for nil input, got %d entries", len(got))
	}

	got, err = db.GetConditionsForControllers(ctx, []string{"application/nothing-here"})
	if err != nil {
		t.Fatalf("GetConditionsForControllers() error = %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected no entry for a controller with no stored conditions, got %+v", got)
	}
}

// TestGetConditionsForControllers_MatchesGetConditions is the batched
// method's core contract: for a mix of controllers with conditions, some
// with none, and one not even asked for, the batched result for each
// requested controller must equal what GetConditions would return for it
// individually, and a controller with no rows must be entirely absent
// from the map (not present with a nil/empty slice), the same
// convention GetConditions itself documents.
func TestGetConditionsForControllers_MatchesGetConditions(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	if err := db.UpsertConditions(ctx, "application/web", []reconcile.Condition{
		{Type: "Ready", Status: reconcile.ConditionTrue, Reason: "Running"},
	}); err != nil {
		t.Fatalf("upsert application/web: %v", err)
	}
	if err := db.UpsertConditions(ctx, "application/api", []reconcile.Condition{
		{Type: "Ready", Status: reconcile.ConditionFalse, Reason: "CrashLoop"},
		{Type: "Progressing", Status: reconcile.ConditionUnknown, Reason: "Pending"},
	}); err != nil {
		t.Fatalf("upsert application/api: %v", err)
	}
	// Deliberately not requested below, proving the batched query is
	// scoped to controllerNames, not "everything in the table."
	if err := db.UpsertConditions(ctx, "application/not-requested", []reconcile.Condition{
		{Type: "Ready", Status: reconcile.ConditionTrue, Reason: "Running"},
	}); err != nil {
		t.Fatalf("upsert application/not-requested: %v", err)
	}

	got, err := db.GetConditionsForControllers(ctx, []string{
		"application/web", "application/api", "application/worker-no-status",
	})
	if err != nil {
		t.Fatalf("GetConditionsForControllers() error = %v", err)
	}

	if _, ok := got["application/not-requested"]; ok {
		t.Error("batched result must not include a controller that wasn't requested")
	}
	if _, ok := got["application/worker-no-status"]; ok {
		t.Error("a requested controller with no stored conditions must be absent from the map, not present with an empty slice")
	}

	web := got["application/web"]
	if len(web) != 1 || web[0].Status != reconcile.ConditionTrue || web[0].Reason != "Running" {
		t.Errorf("application/web = %+v, want one True/Running condition", web)
	}

	api := got["application/api"]
	if len(api) != 2 {
		t.Fatalf("application/api = %+v, want 2 conditions", api)
	}
	byType := map[string]reconcile.Condition{}
	for _, c := range api {
		byType[c.Type] = c
	}
	if byType["Ready"].Status != reconcile.ConditionFalse || byType["Ready"].Reason != "CrashLoop" {
		t.Errorf("application/api Ready condition = %+v, want False/CrashLoop", byType["Ready"])
	}
	if byType["Progressing"].Status != reconcile.ConditionUnknown {
		t.Errorf("application/api Progressing condition = %+v, want Unknown", byType["Progressing"])
	}

	// Cross-check against the unbatched method for the same controller,
	// so a future change to either query can't silently drift the two
	// apart.
	individualWeb, err := db.GetConditions(ctx, "application/web")
	if err != nil {
		t.Fatalf("GetConditions(application/web): %v", err)
	}
	if len(individualWeb) != len(web) || individualWeb[0].Reason != web[0].Reason {
		t.Errorf("batched result for application/web = %+v, want it to match GetConditions = %+v", web, individualWeb)
	}
}
