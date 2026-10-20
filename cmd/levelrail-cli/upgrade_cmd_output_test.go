package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func TestPrintUpgradeHumanVerifierWarnHasNoCommand(t *testing.T) {
	latest := "v9.9.9"
	p := updatePreflight{
		CurrentVersion: "v1.0.0", LatestVersion: &latest, UpdateAvailable: true,
		UpgradeCommand: "upgrade-cmd", RollbackCommand: "rollback-cmd",
		Checks: []apiclient.UpgradeCheck{{
			Code: "release_verifier", Name: "Release signature verification", Status: "warn",
			Message: "Checking the release signature needs one download from Sigstore. Retrying automatically.",
			Hint:    "This clears on its own once the server can reach Sigstore. You can also re-check.",
			Fix:     &apiclient.UpgradeFix{Action: "recheck"},
		}},
	}
	var out bytes.Buffer
	printUpgradeHuman(&out, p, "fresh")
	got := out.String()
	if !strings.Contains(got, "one download from Sigstore") {
		t.Fatalf("missing plain copy:\n%s", got)
	}
	if strings.Contains(strings.ToLower(got), "cosign") {
		t.Fatalf("output must not tell the operator to install cosign:\n%s", got)
	}
}

func TestPrintUpgradeHumanSchemaOlderPointsToRollback(t *testing.T) {
	latest := "v0.6.0"
	p := updatePreflight{
		CurrentVersion: "v0.6.0-138-gabbfcdba5", LatestVersion: &latest, Blocked: true,
		Checks: []apiclient.UpgradeCheck{{
			Code: "schema_compat", Name: "Compatible with your database", Status: "fail",
			Message: "target release is older than the database schema: database is at schema 2160, v0.6.0 supports 1010",
			Hint:    "This release is older than your database. Stay on your current version, or use Roll back to go to an older release on purpose.",
			Fix:     &apiclient.UpgradeFix{Action: "open_rollback"},
		}},
	}
	var out bytes.Buffer
	printUpgradeHuman(&out, p, "skipped")
	got := out.String()
	for _, want := range []string{"supports 1010", "Stay on your current version", "upgrade --list", "Upgrade blocked"} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q:\n%s", want, got)
		}
	}
}
