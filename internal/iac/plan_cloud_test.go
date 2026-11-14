package iac

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/experimental"
)

type fakeCloudPlanner struct {
	diffs map[Kind]CloudDiff
	err   error
}

func (f fakeCloudPlanner) PlanCloud(r CloudResource) (CloudDiff, error) {
	return f.diffs[r.Kind], f.err
}

type fakePolicy struct{ out []CloudRefusal }

func (f fakePolicy) CheckCloud([]CloudChange) []CloudRefusal { return f.out }

const canarySecret = "CANARY-s3cr3t-value-77"

func cloudPlanSetup(t *testing.T) []*Resource {
	t.Helper()
	experimental.Set(experimental.Cloud)
	t.Cleanup(func() {
		experimental.Reset()
		SetCloudPlanner(nil)
		SetCloudPolicy(nil)
	})
	res, issues := buildCloudDocs(t, cloudAccountDoc+"---\n"+cloudBucketDoc)
	if len(issues) > 0 {
		t.Fatal(issues)
	}
	return res
}

func TestCloudPlanUnconfiguredFailsLoudly(t *testing.T) {
	res := cloudPlanSetup(t)
	plan := PlanFor(newState(), res, Options{})
	if len(plan.Changes) != 2 || plan.Summary.Error != 2 || plan.Summary.Noop != 0 {
		t.Fatalf("want 2 failed items, got %+v", plan.Summary)
	}
	for _, c := range plan.Changes {
		if c.Action != ActionError || c.Code != CodeUnavailable || c.Reason != "cloud planner not configured" {
			t.Fatalf("item %+v", c)
		}
	}
}

func TestCloudPlanFakePlannerMasksSecrets(t *testing.T) {
	res := cloudPlanSetup(t)
	SetCloudPlanner(fakeCloudPlanner{diffs: map[Kind]CloudDiff{
		KindCloudAccount: {Action: "create", Type: "x_account", Attrs: []CloudAttr{
			{Path: "region", New: "us-phoenix-1"},
			{Path: "db.admin_password", New: canarySecret},
			{Path: "blob", New: canarySecret, Sensitive: true},
		}},
		KindCloudBucket: {Action: "destroy", Type: "x_bucket", Attrs: []CloudAttr{{Path: "tier", Old: "std"}}},
	}})
	plan := PlanFor(newState(), res, Options{})
	if plan.Summary.Create != 1 || plan.Summary.Delete != 1 || plan.Summary.Error != 0 {
		t.Fatalf("summary %+v", plan.Summary)
	}
	raw, _ := json.Marshal(plan)
	if strings.Contains(string(raw), canarySecret) {
		t.Fatal("canary leaked into plan output")
	}
	if !strings.Contains(string(raw), hiddenValue) || !strings.Contains(string(raw), "us-phoenix-1") {
		t.Fatalf("masking wrong: %s", raw)
	}
	if !strings.HasPrefix(plan.Hash, HashV2Prefix) {
		t.Fatal(plan.Hash)
	}
	if plan.Changes[0].Code != "" || plan.Changes[0].Action == ActionNoop {
		t.Fatalf("%+v", plan.Changes[0])
	}
	for _, c := range plan.Changes {
		if isCloudKind(c.Kind) && cloudDigest(c) == "" {
			t.Fatal("no digest")
		}
	}
}

func TestCloudPlanPlannerErrorAndUnknownAction(t *testing.T) {
	res := cloudPlanSetup(t)
	SetCloudPlanner(fakeCloudPlanner{err: errors.New("boom")})
	if p := PlanFor(newState(), res, Options{}); p.Summary.Error != 2 {
		t.Fatalf("%+v", p.Summary)
	}
	SetCloudPlanner(fakeCloudPlanner{diffs: map[Kind]CloudDiff{KindCloudAccount: {Action: "bogus"}, KindCloudBucket: {Action: "noop"}}})
	p := PlanFor(newState(), res, Options{})
	if p.Summary.Error != 1 || p.Summary.Noop != 1 {
		t.Fatalf("%+v", p.Summary)
	}
}

func TestCloudPlanPolicyRefusals(t *testing.T) {
	res := cloudPlanSetup(t)
	SetCloudPlanner(fakeCloudPlanner{diffs: map[Kind]CloudDiff{
		KindCloudAccount: {Action: "create", Type: "a"},
		KindCloudBucket:  {Action: "delete", Type: "oci_objectstorage_bucket"},
	}})
	SetCloudPolicy(fakePolicy{out: []CloudRefusal{{Code: "cloud_protected_delete", Address: resourceKey(KindCloudBucket, "", "backups"), Message: "protected"}}})
	p := PlanFor(newState(), res, Options{})
	var failed *Change
	for i := range p.Changes {
		if p.Changes[i].Action == ActionError {
			failed = &p.Changes[i]
		}
	}
	if p.Summary.Error != 1 || failed == nil || failed.Code != "cloud_protected_delete" || failed.Kind != KindCloudBucket {
		t.Fatalf("%+v", p)
	}
}

func TestCloudPlanHashIgnoredByNonCloud(t *testing.T) {
	c := []Change{{Kind: KindProject, Name: "a", Action: ActionCreate}}
	if a, b := planHashV2("s", false, c, nil), planHashV2("s", false, c, nil); a != b {
		t.Fatal("unstable")
	}
}

type notReadyPlanner struct{ fakeCloudPlanner }

func (notReadyPlanner) CloudReady() bool { return false }

func TestCloudPlanNotReadyPlannerFailsAccountToo(t *testing.T) {
	res := cloudPlanSetup(t)
	SetCloudPlanner(notReadyPlanner{fakeCloudPlanner{diffs: map[Kind]CloudDiff{KindCloudAccount: {Action: "noop"}, KindCloudBucket: {Action: "create"}}}})
	plan := PlanFor(newState(), res, Options{})
	if plan.Summary.Error != 2 || plan.Summary.Noop != 0 {
		t.Fatalf("want 2 failed items, got %+v", plan.Summary)
	}
	for _, c := range plan.Changes {
		if c.Code != CodeUnavailable || c.Reason != "cloud planner not configured" {
			t.Fatalf("item %+v", c)
		}
	}
}

func TestCloudPlanReadyPlannerKeepsAccountNoop(t *testing.T) {
	res := cloudPlanSetup(t)
	SetCloudPlanner(fakeCloudPlanner{diffs: map[Kind]CloudDiff{KindCloudAccount: {Action: "noop"}, KindCloudBucket: {Action: "create"}}})
	plan := PlanFor(newState(), res, Options{})
	if plan.Summary.Error != 0 || plan.Summary.Noop != 1 {
		t.Fatalf("got %+v", plan.Summary)
	}
}

type unavailablePlanner struct{ fakeCloudPlanner }

func (unavailablePlanner) PlanCloud(CloudResource) (CloudDiff, error) {
	return CloudDiff{}, fmt.Errorf("%w: no stored cloud connection named %q", ErrCloudUnavailable, "oci-test")
}

func TestCloudPlanMissingConnectionIsUnavailable(t *testing.T) {
	res := cloudPlanSetup(t)
	SetCloudPlanner(unavailablePlanner{})
	plan := PlanFor(newState(), res, Options{})
	if plan.Summary.Error != 2 {
		t.Fatalf("want 2 failed items, got %+v", plan.Summary)
	}
	for _, c := range plan.Changes {
		if c.Code != CodeUnavailable || !strings.Contains(c.Reason, "oci-test") {
			t.Fatalf("item %+v", c)
		}
	}
}
