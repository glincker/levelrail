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
