package store

import (
	"context"
	"testing"
)

func TestSaveDesiredService_UserWorkingDir_RoundTrip(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	want := DesiredService{Name: "web", Image: "nginx:1", Port: 80, User: "1000:1000", WorkingDir: "/srv"}
	if err := db.SaveDesiredService(ctx, want); err != nil {
		t.Fatalf("SaveDesiredService() error = %v", err)
	}
	got, err := db.GetDesiredService(ctx, "web")
	if err != nil {
		t.Fatalf("GetDesiredService() error = %v", err)
	}
	if got.User != want.User || got.WorkingDir != want.WorkingDir {
		t.Fatalf("user/working_dir = %q/%q, want %q/%q", got.User, got.WorkingDir, want.User, want.WorkingDir)
	}
}

func TestAppliedFields_UserWorkingDirChangeDrifts(t *testing.T) {
	base := DesiredService{Name: "web", Image: "nginx:1", User: "node", WorkingDir: "/app"}
	before := AppliedFields(base)
	for name, mutate := range map[string]func(*DesiredService){
		AppliedFieldUser:       func(s *DesiredService) { s.User = "1000" },
		AppliedFieldWorkingDir: func(s *DesiredService) { s.WorkingDir = "/srv" },
	} {
		next := base
		mutate(&next)
		after := AppliedFields(next)
		if after[name] == before[name] {
			t.Errorf("%s: applied hash unchanged after a change", name)
		}
		if after[name] == next.User {
			t.Errorf("%s: applied field stores the raw value", name)
		}
	}
}
