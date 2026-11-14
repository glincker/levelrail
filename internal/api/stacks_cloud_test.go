package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/iac"
	"github.com/GLINCKER/levelrail/internal/runtimesettings"
)

const cloudStackDocs = "version: 1\nkind: CloudAccount\nmetadata: {name: oci-test}\nspec:\n  provider: oracle\n  region: us-phoenix-1\n  auth: {method: secret_ref, secretRef: oci-key}\n  limits: {monthlyCost: 0, instances: 1}\n" +
	"---\nversion: 1\nkind: CloudBucket\nmetadata: {name: backups}\nspec: {account: oci-test}\n"

// fakeCloud stands in for the engine: every plan saves a new hash, as a real saved plan does.
type fakeCloud struct {
	mu      sync.Mutex
	action  string
	plans   int
	applies []iac.CloudApplyRequest
	result  iac.CloudApplyResult
	err     error
}

func (f *fakeCloud) PlanCloud(_ context.Context, r iac.CloudResource) (iac.CloudDiff, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if r.Kind == iac.KindCloudAccount {
		return iac.CloudDiff{Action: "noop"}, nil
	}
	f.plans++
	return iac.CloudDiff{Action: f.action, Type: "oci_objectstorage_bucket", PlanHash: fmt.Sprintf("enginehash-%d", f.plans)}, nil
}

func (f *fakeCloud) ApplyCloud(_ context.Context, req iac.CloudApplyRequest) (iac.CloudApplyResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.applies = append(f.applies, req)
	return f.result, f.err
}

func cloudStackFixture(t *testing.T, action string) (*Router, *http.Cookie, string, *fakeCloud) {
	t.Helper()
	f := &fakeCloud{action: action}
	iac.SetCloudPlanner(f)
	t.Cleanup(func() {
		iac.SetCloudPlanner(nil)
	})
	rt, _, cookie, approver := approvalFixture(t)
	return rt, cookie, approver, f
}

func requestCloudApproval(t *testing.T, rt *Router, cookie *http.Cookie) iac.Plan {
	t.Helper()
	var out stackApplyResponse
	code := iacCall(t, rt, authedRequest(t, cookie, http.MethodPost, "/api/v1/stacks/shop-prod/apply", stackBody(t, cloudStackDocs, nil)), &out)
	if code != http.StatusAccepted || out.Plan == nil {
		t.Fatalf("request approval: %d %+v", code, out)
	}
	return *out.Plan
}

func cloudHashOf(p iac.Plan) string {
	for _, c := range p.Changes {
		if c.Kind == iac.KindCloudBucket {
			return c.CloudPlanHash
		}
	}
	return ""
}

func TestCloudStackApplyUnapprovedRefused(t *testing.T) {
	rt, cookie, _, f := cloudStackFixture(t, "create")
	if code := iacCall(t, rt, authedRequest(t, cookie, http.MethodPatch, "/api/v1/stacks/shop-prod", `{"require_approval":false}`), nil); code != http.StatusOK {
		t.Fatalf("disable approval: %d", code)
	}
	var out iacErrorBody
	if code := iacCall(t, rt, authedRequest(t, cookie, http.MethodPost, "/api/v1/stacks/shop-prod/apply", stackBody(t, cloudStackDocs, nil)), &out); code != http.StatusForbidden || out.Code != iac.CodeApprovalRequired {
		t.Fatalf("unapproved cloud apply = %d %+v", code, out)
	}
	if len(f.applies) != 0 {
		t.Fatalf("engine called %d times", len(f.applies))
	}
}

func TestCloudStackApprovedAppliesPinnedHashOnce(t *testing.T) {
	rt, cookie, approver, f := cloudStackFixture(t, "create")
	plan := requestCloudApproval(t, rt, cookie)
	want := cloudHashOf(plan)
	if want == "" {
		t.Fatalf("plan carries no engine hash: %+v", plan.Changes)
	}
	if code := iacCall(t, rt, authedRequest(t, cookie, http.MethodPost, "/api/v1/stacks/shop-prod/revisions/1/approve", `{"confirm_stack":"shop-prod"}`), nil); code != http.StatusForbidden {
		t.Fatalf("requester approved own cloud change: %d", code)
	}
	var out stackApplyResponse
	if code := tokenPost(t, rt, approver, "/api/v1/stacks/shop-prod/revisions/1/approve", "", &out); code != http.StatusOK || out.Revision == nil || out.Revision.Status != "applied" {
		t.Fatalf("approve: %d %+v", code, out)
	}
	if len(f.applies) != 1 || f.applies[0].PlanHash != want || f.applies[0].Destroy || f.applies[0].Stack != "shop-prod" {
		t.Fatalf("engine applies = %+v, want one with %s", f.applies, want)
	}
}

func TestCloudStackApproveHashMismatchRefused(t *testing.T) {
	rt, cookie, approver, f := cloudStackFixture(t, "create")
	requestCloudApproval(t, rt, cookie)
	var out iacErrorBody
	if code := tokenPost(t, rt, approver, "/api/v1/stacks/shop-prod/revisions/1/approve", `{"expected_plan_hash":"v2:other"}`, &out); code != http.StatusConflict || out.Code != iac.CodePlanStale {
		t.Fatalf("mismatch = %d %+v", code, out)
	}
	if len(f.applies) != 0 {
		t.Fatalf("engine called on mismatch")
	}
}

func TestCloudStackDestroyNeedsConfirmation(t *testing.T) {
	rt, cookie, approver, f := cloudStackFixture(t, "delete")
	requestCloudApproval(t, rt, cookie)
	var out stackApplyResponse
	if code := tokenPost(t, rt, approver, "/api/v1/stacks/shop-prod/revisions/1/approve", "", &out); code != http.StatusOK || out.Result == nil || out.Result.OK() {
		t.Fatalf("unconfirmed destroy: %d %+v", code, out)
	}
	if len(f.applies) != 0 || !strings.Contains(fmt.Sprint(out.Result.Results), iac.CodeDestroyNotConfirmed) {
		t.Fatalf("destroy ran without confirmation: %+v %+v", f.applies, out.Result.Results)
	}

	requestCloudApproval(t, rt, cookie)
	if code := tokenPost(t, rt, approver, "/api/v1/stacks/shop-prod/revisions/2/approve", `{"confirm_destroy":"shop-prod"}`, &out); code != http.StatusOK {
		t.Fatalf("confirmed destroy: %d", code)
	}
	if len(f.applies) != 1 || !f.applies[0].Destroy {
		t.Fatalf("engine applies = %+v", f.applies)
	}
}

func TestCloudStackDivergedReportsQuarantine(t *testing.T) {
	rt, cookie, approver, f := cloudStackFixture(t, "create")
	f.result, f.err = iac.CloudApplyResult{Diverged: true, QuarantineID: 42}, errors.New("state write rejected")
	requestCloudApproval(t, rt, cookie)
	var out stackApplyResponse
	if code := tokenPost(t, rt, approver, "/api/v1/stacks/shop-prod/revisions/1/approve", "", &out); code != http.StatusOK || out.Result == nil {
		t.Fatalf("approve: %d %+v", code, out)
	}
	msg := fmt.Sprint(out.Result.Results)
	if !strings.Contains(msg, iac.CodeStateDiverged) || !strings.Contains(msg, "42") {
		t.Fatalf("result = %s", msg)
	}
	p := cloudDivergedPayload("shop-prod", "a", 1, iac.CloudRun{PlanHash: "enginehash-1", Items: []string{"CloudBucket/backups"}, Diverged: true, QuarantineID: 42}, 0)
	raw, _ := json.Marshal(p)
	if p["reason"] != iac.CodeStateDiverged || p["quarantine_id"] != int64(42) || strings.Contains(string(raw), "state write") {
		t.Fatalf("payload = %s", raw)
	}
}

func TestCloudStackApprovalOlderThanPlanMaxAge(t *testing.T) {
	rt, cookie, approver, f := cloudStackFixture(t, "create")
	requestCloudApproval(t, rt, cookie)
	stackCloudNow = func() time.Time { return time.Now().Add(2 * time.Hour) }
	t.Cleanup(func() { stackCloudNow = time.Now })
	var out iacErrorBody
	if code := tokenPost(t, rt, approver, "/api/v1/stacks/shop-prod/revisions/1/approve", "", &out); code != http.StatusGone || out.Code != iac.CodeCloudPlanExpired {
		t.Fatalf("old approval = %d %+v", code, out)
	}
	if len(f.applies) != 0 {
		t.Fatalf("engine called on an expired plan")
	}
}

func TestCloudStackKillSwitchAndFlagOff(t *testing.T) {
	rt, cookie, approver, f := cloudStackFixture(t, "create")
	requestCloudApproval(t, rt, cookie)
	if _, err := rt.runtimeSettings.Set(t.Context(), runtimesettings.KeyIaCApplyEnabled, json.RawMessage("false"), "test"); err != nil {
		t.Fatal(err)
	}
	if code := tokenPost(t, rt, approver, "/api/v1/stacks/shop-prod/revisions/1/approve", "", nil); code != http.StatusServiceUnavailable {
		t.Fatalf("kill switch = %d", code)
	}
	if _, err := rt.runtimeSettings.Set(t.Context(), runtimesettings.KeyIaCApplyEnabled, json.RawMessage("true"), "test"); err != nil {
		t.Fatal(err)
	}
	iac.SetCloudPlanner(nil)
	if code := tokenPost(t, rt, approver, "/api/v1/stacks/shop-prod/revisions/1/approve", "", nil); code == http.StatusOK {
		t.Fatalf("flag off approve applied")
	}
	if len(f.applies) != 0 {
		t.Fatalf("engine called with the switch or flag off")
	}
}
