package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func TestPrintUpgradePlanShowsSchemaAndRollback(t *testing.T) {
	db, target, n := 3034, 3048, 14
	p := apiclient.SelfUpgradePlan{
		CurrentVersion: "v0.8.9", TargetVersion: "v0.8.10", NotesAvailable: true,
		DBSchemaVersion: &db, TargetSchemaVersion: &target, PendingMigrations: &n,
		RollbackCommands: []string{"sudo levelrail rollback"},
	}
	var out bytes.Buffer
	printUpgradePlan(&out, p)
	for _, want := range []string{"schema 3034 -> 3048", "14 migrations", "backup is taken", "sudo levelrail rollback"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("missing %q in %q", want, out.String())
		}
	}
}
