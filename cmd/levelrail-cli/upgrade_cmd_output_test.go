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
