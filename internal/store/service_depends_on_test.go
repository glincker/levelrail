package store

import (
	"context"
	"reflect"
	"testing"
)

// TestSaveDesiredService_DependsOnRoundTrips checks depends_on's
// JSON-array column the same way command/entrypoint's own migrations
// (0088/0098) are already covered: save with a value, read it back
// unchanged, save again with none, confirm it clears rather than
// sticking (SaveDesiredService's full-record-replace contract, not a
// dedicated setter's partial-update one).
func TestSaveDesiredService_DependsOnRoundTrips(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	svc := DesiredService{Name: "myapp-web", Image: "img:v1", Port: 80, DependsOn: []string{"db", "redis"}}
	if err := db.SaveDesiredService(ctx, svc); err != nil {
		t.Fatalf("SaveDesiredService() error = %v", err)
	}

	got, err := db.GetDesiredService(ctx, "myapp-web")
	if err != nil {
		t.Fatalf("GetDesiredService() error = %v", err)
	}
	if !reflect.DeepEqual(got.DependsOn, []string{"db", "redis"}) {
		t.Errorf("DependsOn = %v, want [db redis]", got.DependsOn)
	}

	svc.DependsOn = nil
	if err := db.SaveDesiredService(ctx, svc); err != nil {
		t.Fatalf("SaveDesiredService() (clear) error = %v", err)
	}
	got, err = db.GetDesiredService(ctx, "myapp-web")
	if err != nil {
		t.Fatalf("GetDesiredService() error = %v", err)
	}
	if len(got.DependsOn) != 0 {
		t.Errorf("DependsOn = %v, want none after clearing", got.DependsOn)
	}
}

// TestSaveDesiredService_NoDependsOn_DefaultsToEmpty confirms a service
// saved with no DependsOn at all reads back as an empty slice, not nil
// vs. empty ambiguity a caller would have to special-case.
func TestSaveDesiredService_NoDependsOn_DefaultsToEmpty(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	if err := db.SaveDesiredService(ctx, DesiredService{Name: "solo", Image: "img:v1", Port: 80}); err != nil {
		t.Fatalf("SaveDesiredService() error = %v", err)
	}
	got, err := db.GetDesiredService(ctx, "solo")
	if err != nil {
		t.Fatalf("GetDesiredService() error = %v", err)
	}
	if len(got.DependsOn) != 0 {
		t.Errorf("DependsOn = %v, want none", got.DependsOn)
	}
}
