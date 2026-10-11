package store

import (
	"context"
	"errors"
	"slices"
	"testing"
)

func TestComposeRevisions_RoundTrip(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	for i, body := range []string{"services: {a: {image: x}}", "services: {a: {image: y}}"} {
		n, err := db.AddComposeRevision(ctx, ComposeRevision{AppID: "stack", Compose: body, Profiles: []string{"debug"}, CreatedBy: "admin"})
		if err != nil {
			t.Fatalf("AddComposeRevision() error = %v", err)
		}
		if n != i+1 {
			t.Fatalf("AddComposeRevision() revision = %d, want %d", n, i+1)
		}
	}
	if _, err := db.AddComposeRevision(ctx, ComposeRevision{AppID: "other", Compose: "z"}); err != nil {
		t.Fatalf("AddComposeRevision(other) error = %v", err)
	}

	latest, err := db.GetComposeRevision(ctx, "stack", 0)
	if err != nil {
		t.Fatalf("GetComposeRevision(latest) error = %v", err)
	}
	if latest.Revision != 2 || latest.Compose != "services: {a: {image: y}}" || latest.Source != ComposeRevisionSourceAPI || !slices.Equal(latest.Profiles, []string{"debug"}) {
		t.Errorf("latest = %+v", latest)
	}
	first, err := db.GetComposeRevision(ctx, "stack", 1)
	if err != nil || first.Compose != "services: {a: {image: x}}" {
		t.Errorf("GetComposeRevision(1) = %+v, %v", first, err)
	}
	if _, err := db.GetComposeRevision(ctx, "stack", 9); !errors.Is(err, ErrComposeRevisionNotFound) {
		t.Errorf("GetComposeRevision(9) error = %v, want ErrComposeRevisionNotFound", err)
	}

	list, err := db.ListComposeRevisions(ctx, "stack", 1)
	if err != nil || len(list) != 1 || list[0].Revision != 2 {
		t.Errorf("ListComposeRevisions(limit 1) = %+v, %v", list, err)
	}
	apps, err := db.ListComposeApps(ctx)
	if err != nil || !slices.Equal(apps, []string{"other", "stack"}) {
		t.Errorf("ListComposeApps() = %v, %v", apps, err)
	}

	if err := db.DeleteComposeRevisions(ctx, "stack"); err != nil {
		t.Fatalf("DeleteComposeRevisions() error = %v", err)
	}
	if _, err := db.GetComposeRevision(ctx, "stack", 0); !errors.Is(err, ErrComposeRevisionNotFound) {
		t.Errorf("after delete, GetComposeRevision error = %v", err)
	}
}

func TestDesiredService_DependsOnHealthyRoundTrip(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	want := DesiredService{Name: "stack-web", Image: "nginx:alpine", Port: 80, DependsOn: []string{"db", "cache"}, DependsOnHealthy: []string{"db"}}
	if err := db.SaveDesiredService(ctx, want); err != nil {
		t.Fatalf("SaveDesiredService() error = %v", err)
	}
	got, err := db.GetDesiredService(ctx, "stack-web")
	if err != nil {
		t.Fatalf("GetDesiredService() error = %v", err)
	}
	if !slices.Equal(got.DependsOnHealthy, []string{"db"}) || !slices.Equal(got.DependsOn, []string{"db", "cache"}) {
		t.Errorf("got DependsOn=%v DependsOnHealthy=%v", got.DependsOn, got.DependsOnHealthy)
	}
}
