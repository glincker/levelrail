package main

import (
	"bytes"
	"os"
	"testing"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func TestPrintUpgradePlanListsPendingMigrationsGolden(t *testing.T) {
	db, target, n := 3034, 3036, 2
	p := apiclient.SelfUpgradePlan{
		CurrentVersion: "v0.9.0", TargetVersion: "v0.9.1", NotesAvailable: true,
		DBSchemaVersion: &db, TargetSchemaVersion: &target, PendingMigrations: &n,
		PendingMigrationList: []apiclient.PendingMigration{
			{Version: 3035, Name: "alpha_table"}, {Version: 3036, Name: "beta_index"},
		},
		PendingMigrationSource: "running_binary",
		Steps:                  []string{"download", "health"},
	}
	var out bytes.Buffer
	printUpgradePlan(&out, p)
	want, err := os.ReadFile("testdata/upgrade_plan_migrations.golden")
	if err != nil {
		t.Fatal(err)
	}
	if out.String() != string(want) {
		t.Errorf("plan output changed:\n%s\nwant:\n%s", out.String(), want)
	}
}
