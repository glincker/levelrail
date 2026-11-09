package store

import (
	"context"
	"errors"
	"testing"
)

func TestAppBuildSettingsRoundTrip(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	if err := db.SaveAppBuildSettings(ctx, AppBuildSettings{AppName: "web"}); !errors.Is(err, ErrServiceNotFound) {
		t.Fatalf("save without app = %v, want ErrServiceNotFound", err)
	}
	if err := db.SaveDesiredService(ctx, DesiredService{Name: "web", Image: "nginx:1", Port: 80}); err != nil {
		t.Fatal(err)
	}
	if _, found, err := db.GetAppBuildSettings(ctx, "web"); err != nil || found {
		t.Fatalf("empty get: found=%v err=%v", found, err)
	}
	want := AppBuildSettings{AppName: "web", Target: "runtime", NoCache: true, Provider: "node"}
	if err := db.SaveAppBuildSettings(ctx, want); err != nil {
		t.Fatal(err)
	}
	want.NoCache = false
	if err := db.SaveAppBuildSettings(ctx, want); err != nil {
		t.Fatal(err)
	}
	got, found, err := db.GetAppBuildSettings(ctx, "web")
	if err != nil || !found || got.Target != "runtime" || got.NoCache || got.Provider != "node" {
		t.Fatalf("got %+v found=%v err=%v", got, found, err)
	}
}
