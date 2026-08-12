package store

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"
)

func TestSaveAndGetDesiredService(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	want := DesiredService{
		Name:    "web",
		Image:   "levelrail/thesvg:abc1234",
		Port:    3000,
		Domains: []string{"app.example.com", "www.app.example.com"},
		Env:     map[string]string{"NODE_ENV": "production"},
		Resources: &ServiceResources{
			MemoryBytes: 512 * 1024 * 1024,
			NanoCPUs:    500_000_000,
		},
		Health: &ServiceHealth{
			Readiness: &ServiceProbe{Path: "/healthz", Interval: 5 * time.Second, Timeout: 2 * time.Second},
			Liveness:  &ServiceProbe{Path: "/healthz", Interval: 30 * time.Second, Failures: 3},
		},
	}

	if err := db.SaveDesiredService(ctx, want); err != nil {
		t.Fatalf("SaveDesiredService() error = %v", err)
	}

	got, err := db.GetDesiredService(ctx, "web")
	if err != nil {
		t.Fatalf("GetDesiredService() error = %v", err)
	}

	if got.Name != want.Name || got.Image != want.Image || got.Port != want.Port {
		t.Errorf("scalar fields = %+v, want %+v", got, want)
	}
	if !reflect.DeepEqual(got.Domains, want.Domains) {
		t.Errorf("Domains = %+v, want %+v", got.Domains, want.Domains)
	}
	if !reflect.DeepEqual(got.Env, want.Env) {
		t.Errorf("Env = %+v, want %+v", got.Env, want.Env)
	}
	if got.Resources == nil || *got.Resources != *want.Resources {
		t.Errorf("Resources = %+v, want %+v", got.Resources, want.Resources)
	}
	if got.Health == nil || got.Health.Readiness == nil || *got.Health.Readiness != *want.Health.Readiness {
		t.Errorf("Health.Readiness = %+v, want %+v", got.Health, want.Health)
	}
	if got.Health.Liveness == nil || *got.Health.Liveness != *want.Health.Liveness {
		t.Errorf("Health.Liveness = %+v, want %+v", got.Health.Liveness, want.Health.Liveness)
	}
}

func TestSaveDesiredService_MinimalFieldsRoundTrip(t *testing.T) {
	// No Env, no Resources, no Health: the common case for a service
	// with nothing but an image and a port. Must round-trip cleanly,
	// not error on the way in or produce nil-pointer surprises on the
	// way out.
	db := openTestDB(t)
	ctx := context.Background()

	if err := db.SaveDesiredService(ctx, DesiredService{Name: "minimal", Image: "img:tag", Port: 8080}); err != nil {
		t.Fatalf("SaveDesiredService() error = %v", err)
	}

	got, err := db.GetDesiredService(ctx, "minimal")
	if err != nil {
		t.Fatalf("GetDesiredService() error = %v", err)
	}
	if got.Resources != nil {
		t.Errorf("Resources = %+v, want nil", got.Resources)
	}
	if got.Health != nil {
		t.Errorf("Health = %+v, want nil", got.Health)
	}
	if got.Env == nil {
		t.Error("Env = nil, want a non-nil (possibly empty) map")
	}
	if len(got.Env) != 0 {
		t.Errorf("Env = %+v, want empty", got.Env)
	}
	if got.Domains == nil {
		t.Error("Domains = nil, want a non-nil (possibly empty) slice")
	}
	if len(got.Domains) != 0 {
		t.Errorf("Domains = %+v, want empty", got.Domains)
	}
}

func TestSaveDesiredService_UpsertReplacesNotAccumulates(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	if err := db.SaveDesiredService(ctx, DesiredService{Name: "web", Image: "img:v1", Port: 3000}); err != nil {
		t.Fatalf("first save: %v", err)
	}
	if err := db.SaveDesiredService(ctx, DesiredService{Name: "web", Image: "img:v2", Port: 4000}); err != nil {
		t.Fatalf("second save: %v", err)
	}

	got, err := db.GetDesiredService(ctx, "web")
	if err != nil {
		t.Fatalf("GetDesiredService() error = %v", err)
	}
	if got.Image != "img:v2" || got.Port != 4000 {
		t.Errorf("got %+v, want the second save's values, not a merge of both", got)
	}

	all, err := db.ListDesiredServices(ctx)
	if err != nil {
		t.Fatalf("ListDesiredServices() error = %v", err)
	}
	if len(all) != 1 {
		t.Errorf("expected exactly 1 row after two saves to the same name, got %d", len(all))
	}
}

func TestGetDesiredService_NotFound(t *testing.T) {
	db := openTestDB(t)
	_, err := db.GetDesiredService(context.Background(), "never-saved")
	if !errors.Is(err, ErrServiceNotFound) {
		t.Errorf("error = %v, want ErrServiceNotFound", err)
	}
}

func TestListDesiredServices_OrderedByName(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	for _, name := range []string{"worker", "api", "web"} {
		if err := db.SaveDesiredService(ctx, DesiredService{Name: name, Image: "img:tag", Port: 8080}); err != nil {
			t.Fatalf("save %q: %v", name, err)
		}
	}

	got, err := db.ListDesiredServices(ctx)
	if err != nil {
		t.Fatalf("ListDesiredServices() error = %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("expected 3 services, got %d", len(got))
	}
	want := []string{"api", "web", "worker"}
	for i, name := range want {
		if got[i].Name != name {
			t.Errorf("position %d: got %q, want %q (expected alphabetical order)", i, got[i].Name, name)
		}
	}
}

func TestDeleteDesiredService(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	if err := db.SaveDesiredService(ctx, DesiredService{Name: "web", Image: "img:tag", Port: 8080}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	if err := db.DeleteDesiredService(ctx, "web"); err != nil {
		t.Fatalf("DeleteDesiredService() error = %v", err)
	}

	if _, err := db.GetDesiredService(ctx, "web"); !errors.Is(err, ErrServiceNotFound) {
		t.Errorf("GetDesiredService after delete: error = %v, want ErrServiceNotFound", err)
	}
}

func TestDeleteDesiredService_NotFound(t *testing.T) {
	db := openTestDB(t)

	err := db.DeleteDesiredService(context.Background(), "never-saved")
	if !errors.Is(err, ErrServiceNotFound) {
		t.Errorf("error = %v, want ErrServiceNotFound", err)
	}
}

func TestDeleteDesiredService_DoesNotAffectOtherServices(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	for _, name := range []string{"api", "web"} {
		if err := db.SaveDesiredService(ctx, DesiredService{Name: name, Image: "img:tag", Port: 8080}); err != nil {
			t.Fatalf("seed %q: %v", name, err)
		}
	}

	if err := db.DeleteDesiredService(ctx, "web"); err != nil {
		t.Fatalf("DeleteDesiredService() error = %v", err)
	}

	if _, err := db.GetDesiredService(ctx, "api"); err != nil {
		t.Errorf("unrelated service api should be untouched, got error = %v", err)
	}
}
