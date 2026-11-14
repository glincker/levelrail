package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/iac"
	"github.com/GLINCKER/levelrail/internal/store"
)

const planJobCanary = "canary-credential-7f3a91"

// blockingCloud holds every bucket plan until gate closes, to observe queued and running jobs.
type blockingCloud struct {
	fakeCloud
	gate chan struct{}
}

func (b *blockingCloud) PlanCloud(ctx context.Context, r iac.CloudResource) (iac.CloudDiff, error) {
	if r.Kind != iac.KindCloudAccount {
		<-b.gate
	}
	return b.fakeCloud.PlanCloud(ctx, r)
}

// canaryCloud returns a sensitive attribute holding a credential-shaped value.
type canaryCloud struct{ fakeCloud }

func (c *canaryCloud) PlanCloud(ctx context.Context, r iac.CloudResource) (iac.CloudDiff, error) {
	d, err := c.fakeCloud.PlanCloud(ctx, r)
	if r.Kind != iac.KindCloudAccount {
		d.Attrs = []iac.CloudAttr{{Path: "secret_key", New: planJobCanary, Sensitive: true}}
	}
	return d, err
}

func planJobFixture(t *testing.T, p iac.CloudPlanner) (*Router, *store.DB, *http.Cookie, string) {
	t.Helper()
	iac.SetCloudPlanner(p)
	t.Cleanup(func() { iac.SetCloudPlanner(nil) })
	rt, db, cookie, approver := approvalFixture(t)
	t.Cleanup(rt.waitPlanJobs)
	return rt, db, cookie, approver
}

func startPlanJob(t *testing.T, rt *Router, cookie *http.Cookie, query string, extra map[string]any) stackPlanJobResponse {
	t.Helper()
	var out stackPlanJobResponse
	code := iacCall(t, rt, authedRequest(t, cookie, http.MethodPost, "/api/v1/stacks/shop-prod/plan"+query, stackBody(t, cloudStackDocs, extra)), &out)
	if code != http.StatusAccepted || out.ID == "" || out.Status != store.PlanJobQueued || out.PollURL == "" {
		t.Fatalf("cloud plan = %d %+v, want 202 with a queued job", code, out)
	}
	return out
}

func getPlanJob(t *testing.T, rt *Router, cookie *http.Cookie, id string) (stackPlanJobResponse, string) {
	t.Helper()
	code, raw := rawCall(t, rt, authedRequest(t, cookie, http.MethodGet, "/api/v1/stacks/shop-prod/plan-jobs/"+id, ""))
	if code != http.StatusOK {
		t.Fatalf("poll %s = %d %s", id, code, raw)
	}
	var out stackPlanJobResponse
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		t.Fatal(err)
	}
	return out, raw
}

func waitPlanJobStatus(t *testing.T, rt *Router, cookie *http.Cookie, id, want string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if j, _ := getPlanJob(t, rt, cookie, id); j.Status == want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("job %s never reached %s", id, want)
}

func TestPlanJobCloudPlanIsAsyncAndCompletes(t *testing.T) {
	rt, _, cookie, _ := planJobFixture(t, &fakeCloud{action: "create"})
	job := startPlanJob(t, rt, cookie, "", nil)
	rt.waitPlanJobs()
	got, _ := getPlanJob(t, rt, cookie, job.ID)
	if got.Status != store.PlanJobDone || got.Result == nil || got.Result.Plan == nil || cloudHashOf(*got.Result.Plan) == "" {
		t.Fatalf("polled job = %+v, want done with the plan", got)
	}
	var missing iacErrorBody
	if code := iacCall(t, rt, authedRequest(t, cookie, http.MethodGet, "/api/v1/stacks/shop-prod/plan-jobs/pj_none", ""), &missing); code != http.StatusNotFound || missing.Code != iac.CodePlanJobNotFound {
		t.Fatalf("missing job = %d %+v", code, missing)
	}
}

func TestPlanJobNonCloudPlanStaysSynchronous(t *testing.T) {
	rt, _, cookie, _ := planJobFixture(t, &fakeCloud{action: "create"})
	var plan stackPlanResponse
	if code := iacCall(t, rt, authedRequest(t, cookie, http.MethodPost, "/api/v1/stacks/shop-prod/plan", stackBody(t, stackTestDocs("ghcr.io/acme/web:1.2.3"), nil)), &plan); code != http.StatusOK || plan.Plan == nil {
		t.Fatalf("non cloud plan = %d %+v, want 200 with the plan", code, plan)
	}
}

func TestPlanJobQueuesBehindRunningPlanOfSameStack(t *testing.T) {
	b := &blockingCloud{fakeCloud: fakeCloud{action: "create"}, gate: make(chan struct{})}
	rt, _, cookie, _ := planJobFixture(t, b)
	first := startPlanJob(t, rt, cookie, "", nil)
	waitPlanJobStatus(t, rt, cookie, first.ID, store.PlanJobRunning)
	second := startPlanJob(t, rt, cookie, "", nil)
	time.Sleep(50 * time.Millisecond)
	if j, _ := getPlanJob(t, rt, cookie, second.ID); j.Status != store.PlanJobQueued {
		t.Fatalf("second job = %s, want queued behind the first", j.Status)
	}
	close(b.gate)
	rt.waitPlanJobs()
	for _, id := range []string{first.ID, second.ID} {
		if j, _ := getPlanJob(t, rt, cookie, id); j.Status != store.PlanJobDone {
			t.Fatalf("job %s = %+v, want done", id, j)
		}
	}
}

func TestPlanJobGateCapsConcurrentPlans(t *testing.T) {
	g := &planJobGate{stacks: map[string]bool{}}
	g.cond = sync.NewCond(&g.mu)
	one := func() int { return 1 }
	g.acquire("a", one)
	got := make(chan struct{})
	go func() { g.acquire("b", one); close(got) }()
	select {
	case <-got:
		t.Fatal("second stack ran past a cap of one")
	case <-time.After(50 * time.Millisecond):
	}
	g.release("a")
	select {
	case <-got:
	case <-time.After(2 * time.Second):
		t.Fatal("queued plan never ran after the slot freed")
	}
	g.release("b")
}

func TestPlanJobRestartMarksInterrupted(t *testing.T) {
	rt, db, cookie, _ := planJobFixture(t, &fakeCloud{action: "create"})
	old := time.Now().Add(-3 * time.Hour)
	if err := db.InsertStackPlanJob(t.Context(), store.StackPlanJob{ID: "pj_dead", Stack: "shop-prod", Actor: "admin", CreatedAt: old}); err != nil {
		t.Fatal(err)
	}
	if ok, err := db.StartStackPlanJob(t.Context(), "pj_dead", old); err != nil || !ok {
		t.Fatalf("start: %v %v", ok, err)
	}
	if j, _ := getPlanJob(t, rt, cookie, "pj_dead"); j.Status != store.PlanJobFailed || j.Code != iac.CodeInterrupted {
		t.Fatalf("job of a stopped process = %+v, want failed interrupted", j)
	}
	next := startPlanJob(t, rt, cookie, "", nil)
	rt.waitPlanJobs()
	if j, _ := getPlanJob(t, rt, cookie, next.ID); j.Status != store.PlanJobDone {
		t.Fatalf("plan after restart = %+v, want done with nothing left locked", j)
	}
}

func TestPlanJobApprovalPendingAndSupersede(t *testing.T) {
	rt, _, cookie, approver := planJobFixture(t, &fakeCloud{action: "create"})
	requestCloudApproval(t, rt, cookie)
	var out iacErrorBody
	code := iacCall(t, rt, authedRequest(t, cookie, http.MethodPost, "/api/v1/stacks/shop-prod/plan", stackBody(t, cloudStackDocs, nil)), &out)
	if code != http.StatusConflict || out.Code != iac.CodeApprovalPending || !strings.Contains(out.Error, "revision 1") {
		t.Fatalf("plan with pending approval = %d %+v", code, out)
	}
	job := startPlanJob(t, rt, cookie, "?supersede=true", nil)
	rt.waitPlanJobs()
	if j, _ := getPlanJob(t, rt, cookie, job.ID); j.Status != store.PlanJobDone {
		t.Fatalf("superseding job = %+v", j)
	}
	if code := tokenPost(t, rt, approver, "/api/v1/stacks/shop-prod/revisions/1/approve", "", &out); code != http.StatusConflict || out.Code != iac.CodePlanSuperseded {
		t.Fatalf("approve superseded = %d %+v", code, out)
	}
}

func TestPlanJobApproveAfterReplacedEnginePlanIsSuperseded(t *testing.T) {
	f := &fakeCloud{action: "create"}
	rt, db, cookie, approver := planJobFixture(t, f)
	requestCloudApproval(t, rt, cookie)
	later := time.Now().Add(time.Minute)
	if err := db.InsertStackPlanJob(t.Context(), store.StackPlanJob{ID: "pj_later", Stack: "shop-prod", CreatedAt: later}); err != nil {
		t.Fatal(err)
	}
	_, _ = db.StartStackPlanJob(t.Context(), "pj_later", later)
	if ok, err := db.FinishStackPlanJob(t.Context(), store.StackPlanJob{ID: "pj_later", Status: store.PlanJobDone, CloudHashes: []string{"enginehash-other"}, FinishedAt: later}); err != nil || !ok {
		t.Fatalf("finish: %v %v", ok, err)
	}
	var out iacErrorBody
	if code := tokenPost(t, rt, approver, "/api/v1/stacks/shop-prod/revisions/1/approve", "", &out); code != http.StatusConflict || out.Code != iac.CodePlanSuperseded || !strings.Contains(out.Error, "plan again") {
		t.Fatalf("approve after replaced plan = %d %+v", code, out)
	}
	if len(f.applies) != 0 {
		t.Fatalf("engine ran a superseded plan")
	}
}

func TestPlanJobNoCredentialCanary(t *testing.T) {
	rt, db, cookie, _ := planJobFixture(t, &canaryCloud{fakeCloud{action: "create"}})
	job := startPlanJob(t, rt, cookie, "", map[string]any{"secrets": map[string]string{"oci-key": planJobCanary}})
	rt.waitPlanJobs()
	got, raw := getPlanJob(t, rt, cookie, job.ID)
	if got.Status != store.PlanJobDone || strings.Contains(raw, planJobCanary) {
		t.Fatalf("job response leaks or failed: %s", raw)
	}
	var result, msg string
	if err := db.QueryRowContext(t.Context(), "SELECT COALESCE(CAST(result AS TEXT), ''), error FROM stack_plan_jobs WHERE id = ?", job.ID).Scan(&result, &msg); err != nil {
		t.Fatal(err)
	}
	if result == "" || strings.Contains(result+msg, planJobCanary) {
		t.Fatalf("job row leaks the canary or holds no result")
	}
}

// ctxBlockingCloud blocks a bucket plan until its context ends, then keeps unwinding until released.
type ctxBlockingCloud struct {
	fakeCloud
	unwind chan struct{}
}

func (c *ctxBlockingCloud) PlanCloud(ctx context.Context, r iac.CloudResource) (iac.CloudDiff, error) {
	if r.Kind != iac.KindCloudAccount {
		<-ctx.Done()
		<-c.unwind
		return iac.CloudDiff{}, ctx.Err()
	}
	return c.fakeCloud.PlanCloud(ctx, r)
}

func TestPlanJobTimeoutFreesSlotWhileEngineUnwinds(t *testing.T) {
	c := &ctxBlockingCloud{fakeCloud: fakeCloud{action: "create"}, unwind: make(chan struct{})}
	rt, _, cookie, _ := planJobFixture(t, c)
	t.Cleanup(func() { close(c.unwind) })
	planJobTimeoutOverride = 300 * time.Millisecond
	t.Cleanup(func() { planJobTimeoutOverride = 0 })
	job := startPlanJob(t, rt, cookie, "", nil)
	waitPlanJobStatus(t, rt, cookie, job.ID, store.PlanJobFailed)
	if j, _ := getPlanJob(t, rt, cookie, job.ID); j.Code != iac.CodePlanTimeout {
		t.Fatalf("timed out job = %+v, want code %s", j, iac.CodePlanTimeout)
	}
	g := rt.planGate()
	g.mu.Lock()
	running, held := g.running, g.stacks[job.Stack]
	g.mu.Unlock()
	if running != 0 || held {
		t.Fatalf("slot still held after timeout: running=%d stack=%v", running, held)
	}
}
