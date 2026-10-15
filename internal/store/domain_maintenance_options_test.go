package store

import (
	"context"
	"reflect"
	"testing"
	"time"
)

func TestDomainMaintenance_SettingsAndLinkedWindow(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)
	if err := db.SaveDesiredService(ctx, DesiredService{Name: "web", Image: "img:v1", Port: 80, Domains: []string{"a.example.com", "b.example.com"}}); err != nil {
		t.Fatal(err)
	}

	w, err := db.InsertMaintenanceWindow(ctx, MaintenanceWindow{
		Title: "Upgrade", Scope: MaintenanceScopeApp, Target: "web", StartsAt: now.Add(-time.Minute), EndsAt: now.Add(time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	want := DomainMaintenance{
		Domain: "a.example.com", Title: "Upgrading", Message: "Back soon", RetryAfterSeconds: 120,
		BypassCIDRs: []string{"10.0.0.0/8", "192.0.2.7/32"}, BypassHealthCheck: true, WindowID: w.ID,
	}
	if err := db.SaveDomainMaintenance(ctx, want); err != nil {
		t.Fatal(err)
	}
	if err := db.SetDomainMaintenance(ctx, "b.example.com"); err != nil {
		t.Fatal(err)
	}

	got, ok, err := db.GetDomainMaintenanceSettings(ctx, "a.example.com", now)
	if err != nil || !ok {
		t.Fatalf("get = %v, %v", ok, err)
	}
	if got.Title != want.Title || got.RetryAfterSeconds != 120 || !reflect.DeepEqual(got.BypassCIDRs, want.BypassCIDRs) ||
		!got.BypassHealthCheck || !got.EndsAt.Equal(w.EndsAt) {
		t.Errorf("settings = %+v", got)
	}
	plain, ok, _ := db.GetDomainMaintenanceSettings(ctx, "b.example.com", now)
	if !ok || !plain.BypassHealthCheck || plain.WindowID != "" || !plain.EndsAt.IsZero() {
		t.Errorf("default row = %+v, %v", plain, ok)
	}

	active, _ := db.ListActiveDomainMaintenance(ctx, now)
	if len(active) != 2 {
		t.Errorf("active now = %+v", active)
	}
	later := now.Add(2 * time.Hour)
	active, _ = db.ListActiveDomainMaintenance(ctx, later)
	expired, _ := db.ListExpiredDomainMaintenance(ctx, later)
	if len(active) != 1 || active[0].Domain != "b.example.com" || len(expired) != 1 || expired[0].Domain != "a.example.com" {
		t.Errorf("after the window: active=%+v expired=%+v", active, expired)
	}
	if _, ok, _ := db.GetDomainMaintenanceSettings(ctx, "a.example.com", later); ok {
		t.Error("an ended window still reports maintenance")
	}

	// Cancelling the window ends the linked maintenance at once.
	if err := db.DeleteMaintenanceWindow(ctx, w.ID); err != nil {
		t.Fatal(err)
	}
	if expired, _ = db.ListExpiredDomainMaintenance(ctx, now); len(expired) != 1 || expired[0].Domain != "a.example.com" {
		t.Errorf("cancelled window: expired = %+v", expired)
	}
	names, _ := db.ListDomainMaintenance(ctx)
	if !reflect.DeepEqual(names, []string{"b.example.com"}) {
		t.Errorf("ListDomainMaintenance = %v", names)
	}
}
