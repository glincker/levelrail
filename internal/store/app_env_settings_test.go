package store

import (
	"context"
	"testing"
)

func TestAppEnvSettingsRoundTripAndCascade(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	if err := db.SaveDesiredService(ctx, DesiredService{Name: "web", Image: "nginx:1", Port: 80}); err != nil {
		t.Fatal(err)
	}

	got, err := db.GetAppEnvSettings(ctx, "web")
	if err != nil || got.SecretsWriteOnly || len(got.SensitiveOverrides) != 0 {
		t.Fatalf("default settings = %+v, %v", got, err)
	}

	want := AppEnvSettings{AppName: "web", SensitiveOverrides: map[string]bool{"A": true, "B": false}, SecretsWriteOnly: true}
	if err := db.SaveAppEnvSettings(ctx, want); err != nil {
		t.Fatal(err)
	}
	got, err = db.GetAppEnvSettings(ctx, "web")
	if err != nil || !got.SecretsWriteOnly || !got.SensitiveOverrides["A"] || got.SensitiveOverrides["B"] {
		t.Fatalf("saved settings = %+v, %v", got, err)
	}
	all, err := db.ListAppEnvSettings(ctx)
	if err != nil || len(all) != 1 || !all["web"].SecretsWriteOnly {
		t.Fatalf("list = %+v, %v", all, err)
	}

	// A re-created app with the same name must not inherit a plain override.
	if err := db.DeleteDesiredService(ctx, "web"); err != nil {
		t.Fatal(err)
	}
	if err := db.SaveDesiredService(ctx, DesiredService{Name: "web", Image: "nginx:1", Port: 80}); err != nil {
		t.Fatal(err)
	}
	got, err = db.GetAppEnvSettings(ctx, "web")
	if err != nil || got.SecretsWriteOnly || len(got.SensitiveOverrides) != 0 {
		t.Fatalf("settings survived delete: %+v, %v", got, err)
	}

	if err := db.SaveAppEnvSettings(ctx, AppEnvSettings{AppName: "missing"}); err == nil {
		t.Fatal("saving settings for a missing app should fail")
	}
}
