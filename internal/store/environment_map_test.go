package store

import (
	"context"
	"testing"
)

func TestEnvironmentMaps(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	for _, n := range []string{"tagged", "loose"} {
		if err := db.SaveDesiredService(ctx, DesiredService{Name: n, Image: n + ":1", Port: 80}); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.SetServiceEnvironment(ctx, "tagged", "env_production"); err != nil {
		t.Fatal(err)
	}
	if err := db.SaveDesiredDatabase(ctx, DesiredDatabase{Name: "db", Engine: EngineRedis, Version: "7"}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE desired_databases SET environment_id = 'env_dev' WHERE name = 'db'`); err != nil {
		t.Fatal(err)
	}

	apps, err := db.EnvironmentsOfApps(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(apps) != 1 || apps["tagged"] == nil || apps["tagged"].Kind != "production" || !apps["tagged"].Protected {
		t.Fatalf("apps = %+v", apps)
	}
	dbs, err := db.EnvironmentsOfDatabases(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(dbs) != 1 || dbs["db"] == nil || dbs["db"].Kind != "dev" {
		t.Fatalf("dbs = %+v", dbs)
	}
}
