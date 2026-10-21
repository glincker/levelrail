package store

import (
	"context"
	"errors"
	"testing"
)

func TestScheduledTaskHTTP_RoundTripAndCascade(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	seedScheduledTaskService(t, db, "web")
	if err := db.SaveScheduledTask(ctx, newTestScheduledTask("st_1", "web")); err != nil {
		t.Fatalf("SaveScheduledTask: %v", err)
	}
	if _, err := db.GetScheduledTaskHTTP(ctx, "st_1"); !errors.Is(err, ErrScheduledTaskHTTPNotFound) {
		t.Fatalf("Get before set = %v, want not found", err)
	}

	steps := []ScheduledTaskHTTP{
		{TaskID: "st_1", Path: "/cron/tick"},
		{TaskID: "st_1", Method: "GET", Path: "/cron/report", Body: "x", Background: true},
	}
	wantMethod := []string{"POST", "GET"}
	for i, h := range steps {
		if err := db.SetScheduledTaskHTTP(ctx, h); err != nil {
			t.Fatalf("step %d Set: %v", i, err)
		}
		got, err := db.GetScheduledTaskHTTP(ctx, "st_1")
		if err != nil {
			t.Fatalf("step %d Get: %v", i, err)
		}
		if got.Method != wantMethod[i] || got.Path != h.Path || got.Background != h.Background || got.Body != h.Body {
			t.Errorf("step %d got %+v, want method %s and %+v", i, got, wantMethod[i], h)
		}
	}

	if err := db.DeleteScheduledTask(ctx, "st_1"); err != nil {
		t.Fatalf("DeleteScheduledTask: %v", err)
	}
	if _, err := db.GetScheduledTaskHTTP(ctx, "st_1"); !errors.Is(err, ErrScheduledTaskHTTPNotFound) {
		t.Errorf("Get after task delete = %v, want cascade to not found", err)
	}
}
