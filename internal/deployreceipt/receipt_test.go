package deployreceipt

import (
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/supplychain"
)

func TestAssembleGolden(t *testing.T) {
	start := time.Date(2026, 11, 5, 10, 0, 0, 0, time.UTC)
	fin := start.Add(time.Minute)
	in := Inputs{
		Attempt: store.DeployAttempt{ID: "dep_1", ServiceName: "web", Image: "repo/web:abc", CommitSHA: "abc", Branch: "main", Author: "dev",
			Source: "git_push", Status: "succeeded", StartedAt: start, FinishedAt: &fin, ImageDigest: "sha256:aa", DigestReason: store.DigestReasonLocalBuild},
		Verdict:   "succeeded",
		Requester: Person{Kind: "user", ID: "u1", Name: "Ana"},
		Approval: &store.DeployApproval{ID: "apr_1", Action: "deploy", Status: "approved", RequestedByType: "user", RequestedBy: "u1", RequestedByName: "Ana",
			ApprovedByType: "user", ApprovedBy: "u2", ApprovedByName: "Bo", DecidedAt: "2026-11-05T10:00:30Z", FreezeOverride: "incident fix"},
		SBOM: &supplychain.Record{Summary: supplychain.SBOMSummary{Format: "spdx", PackageCount: 12}, GeneratedAt: start, ScanStatus: "ok"},
		Gate: []store.DeployGateEvent{{At: start, Phase: "readiness", Status: "passed"}},
		Now:  start.Add(time.Hour),
	}
	got, err := json.MarshalIndent(Assemble(in), "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if os.Getenv("UPDATE_GOLDEN") != "" {
		if err := os.WriteFile("testdata/receipt.golden.json", append(got, '\n'), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile("testdata/receipt.golden.json")
	if err != nil {
		t.Fatal(err)
	}
	if string(got)+"\n" != string(want) {
		t.Fatalf("receipt mismatch:\n%s", got)
	}
}

func TestAssembleNotRecorded(t *testing.T) {
	r := Assemble(Inputs{Attempt: store.DeployAttempt{ID: "d", StartedAt: time.Unix(0, 0)}, Now: time.Unix(0, 0)})
	if r.Image.Digest != NotRecorded || r.SBOM.Recorded || r.SBOM.Note != NotRecorded || r.Requester != nil || r.Signed {
		t.Fatalf("unexpected defaults: %+v", r)
	}
}
