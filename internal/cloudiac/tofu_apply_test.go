package cloudiac

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

var cred = CredentialHandle{ID: "oci-test"}

const newState = `{"serial":2,"secret":"` + stateCanary + `-new"}`

func (h *harness) planned(t *testing.T) string {
	t.Helper()
	h.seedState(t)
	h.runner.out["show"] = RunOutput{Stdout: []byte(showJSON)}
	pr, err := h.eng.PlanDetailed(context.Background(), testModule(), cred)
	if err != nil || len(pr.Hash) != 64 {
		t.Fatalf("plan: %v %q", err, pr.Hash)
	}
	h.runner.calls = nil
	return pr.Hash
}

func wantKind(t *testing.T, err error, kind ErrorKind, sentinel error) {
	t.Helper()
	var ee *Error
	if !errors.As(err, &ee) || ee.Kind != kind || (sentinel != nil && !errors.Is(err, sentinel)) {
		t.Fatalf("want %s %v, got %v", kind, sentinel, err)
	}
	if strings.Contains(err.Error(), stateCanary) {
		t.Fatalf("state leaked into error: %v", err)
	}
}

func TestTofuApplySavedPlanWritesState(t *testing.T) {
	h := newHarness(t, allPrices, 0)
	hash := h.planned(t)
	h.runner.applyState = []byte(newState)
	res, err := h.eng.Apply(context.Background(), ApplyRequest{Spec: testModule(), Cred: cred, PlanHash: hash})
	if err != nil || res.StateVersion != 2 || res.Partial || res.Diverged || res.PlanHash != hash {
		t.Fatalf("apply = %+v, %v", res, err)
	}
	if got := strings.Join(h.runner.calls, ","); got != "init,apply" {
		t.Fatalf("steps = %s", got)
	}
	if strings.Contains(fmt.Sprintf("%+v", res), stateCanary) {
		t.Fatal("state leaked into the result")
	}
	got, ver, err := h.state.Load(context.Background(), "oci-test", "net-lr")
	if err != nil || ver != 2 || string(got) != newState {
		t.Fatalf("state after apply: %d %v", ver, err)
	}
	h.assertClean(t)
	_, err = h.eng.Apply(context.Background(), ApplyRequest{Spec: testModule(), Cred: cred, PlanHash: hash})
	wantKind(t, err, ErrPlanRejected, nil)
}

func TestTofuApplyRefusesBeforeAnyStep(t *testing.T) {
	cases := map[string]func(h *harness, hash *string){
		"hash mismatch": func(_ *harness, hash *string) { *hash = strings.Repeat("0", 64) },
		"expired":       func(h *harness, _ *string) { h.eng.now = func() time.Time { return time.Now().Add(2 * time.Hour) } },
		"policy":        func(h *harness, _ *string) { h.eng.cfg.Policy = func() Policy { return NewPolicy(0, false) } },
	}
	kinds := map[string]ErrorKind{"hash mismatch": ErrPlanRejected, "expired": ErrPlanRejected, "policy": ErrPolicyRefused}
	sentinels := map[string]error{"hash mismatch": ErrPlanMismatch, "expired": ErrPlanExpired}
	for name, mut := range cases {
		t.Run(name, func(t *testing.T) {
			h := newHarness(t, allPrices, 0)
			hash := h.planned(t)
			mut(h, &hash)
			_, err := h.eng.Apply(context.Background(), ApplyRequest{Spec: testModule(), Cred: cred, PlanHash: hash})
			wantKind(t, err, kinds[name], sentinels[name])
			if len(h.runner.calls) != 0 {
				t.Fatalf("a step ran: %v", h.runner.calls)
			}
			h.eng.now = time.Now
			h.assertClean(t)
		})
	}
	h := newHarness(t, allPrices, 0)
	_, err := h.eng.Apply(context.Background(), ApplyRequest{Spec: testModule(), Cred: cred, PlanHash: "abc"})
	wantKind(t, err, ErrPlanRejected, ErrPlanNotFound)
}

func TestTofuApplyFenceMovedReportsDiverged(t *testing.T) {
	h := newHarness(t, allPrices, 0)
	hash := h.planned(t)
	h.runner.applyState = []byte(newState)
	h.runner.onApply = func() {
		if _, err := h.db.ExecContext(context.Background(), "UPDATE cloud_state_locks SET fence = fence + 1"); err != nil {
			t.Error(err)
		}
	}
	res, err := h.eng.Apply(context.Background(), ApplyRequest{Spec: testModule(), Cred: cred, PlanHash: hash})
	wantKind(t, err, ErrStateDiverged, ErrStateWriteRejected)
	if !res.Diverged {
		t.Fatalf("result = %+v", res)
	}
	if _, ver, _ := h.state.Load(context.Background(), "oci-test", "net-lr"); ver != 1 {
		t.Fatalf("stale lease wrote state: version %d", ver)
	}
	var q *StateQuarantine
	if !errors.As(err, &q) || q.ID == 0 || !strings.Contains(err.Error(), fmt.Sprint(q.ID)) || !strings.Contains(res.Log, fmt.Sprint(q.ID)) {
		t.Fatalf("no quarantine id in %v / %q", err, res.Log)
	}
	h.assertQuarantined(t, q.ID, "fence_moved")
}

// bumpFence makes the lease stale mid apply, as a stealer would.
func (h *harness) bumpFence(t *testing.T) {
	h.runner.onApply = func() {
		if _, err := h.db.ExecContext(context.Background(), "UPDATE cloud_state_locks SET fence = fence + 1"); err != nil {
			t.Error(err)
		}
	}
}

// freeLock ends the simulated stealer's lease so the next run reaches the quarantine check.
func (h *harness) freeLock(t *testing.T) {
	if _, err := h.db.ExecContext(context.Background(), "UPDATE cloud_state_locks SET holder = '', expires_at = ''"); err != nil {
		t.Fatal(err)
	}
}

// assertQuarantined checks the row is sealed, opens to the new state, and blocks every run until resolved.
func (h *harness) assertQuarantined(t *testing.T, id int64, reason string) {
	t.Helper()
	ctx := context.Background()
	rows, err := h.state.Quarantined(ctx, "oci-test", "net-lr")
	if err != nil || len(rows) != 1 || rows[0].ID != id || rows[0].Reason != reason || rows[0].Fence == 0 {
		t.Fatalf("quarantine rows = %+v, %v", rows, err)
	}
	var raw []byte
	if err := h.db.QueryRowContext(ctx, "SELECT sealed FROM cloud_state_quarantine WHERE id = ?", id).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(raw, []byte(stateCanary)) {
		t.Fatal("quarantined state is plaintext in the DB")
	}
	if pt, err := h.state.OpenQuarantine(rows[0]); err != nil || string(pt) != newState {
		t.Fatalf("quarantine does not open to the applied state: %v", err)
	}
	h.runner.calls = nil
	h.runner.onApply = nil
	h.freeLock(t)
	_, err = h.eng.PlanDetailed(ctx, testModule(), cred)
	wantKind(t, err, ErrQuarantineHeld, ErrStateQuarantined)
	if !strings.Contains(err.Error(), fmt.Sprint(id)) || len(h.runner.calls) != 0 {
		t.Fatalf("refusal must name the id and run nothing: %v %v", err, h.runner.calls)
	}
	_, err = h.eng.Apply(ctx, ApplyRequest{Spec: testModule(), Cred: cred, PlanHash: "abc"})
	wantKind(t, err, ErrQuarantineHeld, ErrStateQuarantined)
	if err := h.db.ResolveCloudStateQuarantine(ctx, "oci-test", "other", id, time.Now()); err == nil {
		t.Fatal("an id from another stack resolved this one")
	}
	if err := h.db.ResolveCloudStateQuarantine(ctx, "oci-test", "net-lr", id, time.Now()); err != nil {
		t.Fatal(err)
	}
	if _, err := h.eng.PlanDetailed(ctx, testModule(), cred); err != nil {
		t.Fatalf("plan after resolve: %v", err)
	}
	h.assertClean(t)
}

func TestTofuApplyFailedAndFenceMovedQuarantines(t *testing.T) {
	h := newHarness(t, allPrices, 0)
	hash := h.planned(t)
	h.runner.applyState = []byte(newState)
	h.runner.fail = "apply"
	h.bumpFence(t)
	res, err := h.eng.Apply(context.Background(), ApplyRequest{Spec: testModule(), Cred: cred, PlanHash: hash})
	wantKind(t, err, ErrStateDiverged, ErrStateWriteRejected)
	var q *StateQuarantine
	if !res.Diverged || !errors.As(err, &q) {
		t.Fatalf("result = %+v, %v", res, err)
	}
	h.runner.fail = ""
	h.assertQuarantined(t, q.ID, "fence_moved,apply_failed")
}

func TestTofuQuarantineStoreFailureFailsClosed(t *testing.T) {
	h := newHarness(t, allPrices, 0)
	hash := h.planned(t)
	h.runner.applyState = []byte(newState)
	h.runner.onApply = func() {
		for _, q := range []string{"UPDATE cloud_state_locks SET fence = fence + 1", "DROP TABLE cloud_state_quarantine"} {
			if _, err := h.db.ExecContext(context.Background(), q); err != nil {
				t.Error(err)
			}
		}
	}
	res, err := h.eng.Apply(context.Background(), ApplyRequest{Spec: testModule(), Cred: cred, PlanHash: hash})
	wantKind(t, err, ErrStateDiverged, ErrStateWriteRejected)
	var q *StateQuarantine
	if !res.Diverged || errors.As(err, &q) || !strings.Contains(err.Error(), "reconcile by hand") {
		t.Fatalf("result = %+v, %v", res, err)
	}
	h.runner.calls = nil
	h.freeLock(t)
	_, err = h.eng.PlanDetailed(context.Background(), testModule(), cred)
	wantKind(t, err, ErrInternal, ErrStateQuarantined)
	if len(h.runner.calls) != 0 {
		t.Fatalf("a step ran: %v", h.runner.calls)
	}
}

func TestTofuApplyFailureStoresPartialState(t *testing.T) {
	h := newHarness(t, allPrices, 0)
	hash := h.planned(t)
	h.runner.applyState = []byte(newState)
	h.runner.fail = "apply"
	h.runner.out["apply"] = RunOutput{Stderr: []byte("Error: quota exceeded")}
	res, err := h.eng.Apply(context.Background(), ApplyRequest{Spec: testModule(), Cred: cred, PlanHash: hash})
	wantKind(t, err, ErrProvider, nil)
	if !res.Partial || res.StateVersion != 2 {
		t.Fatalf("result = %+v", res)
	}
	got, _, err := h.state.Load(context.Background(), "oci-test", "net-lr")
	if err != nil || string(got) != newState {
		t.Fatalf("partial state lost: %v", err)
	}
	h.assertClean(t)
}

const destroyJSON = `{"resource_changes":[
 {"address":"oci_core_vcn.lr","mode":"managed","type":"oci_core_vcn","change":{"actions":["delete"],"before":{},"after":null}},
 {"address":"oci_core_subnet.lr","mode":"managed","type":"oci_core_subnet","change":{"actions":["delete"],"before":{},"after":null}}
]}`

func TestTofuDestroyNeedsConfirmationAndCap(t *testing.T) {
	h := newHarness(t, allPrices, 0)
	ctx := context.Background()
	h.seedState(t)
	h.runner.out["show"] = RunOutput{Stdout: []byte(destroyJSON)}
	_, err := h.eng.Destroy(ctx, ApplyRequest{Spec: testModule(), Cred: cred, PlanHash: "x"})
	wantKind(t, err, ErrPolicyRefused, ErrDestroyNotConfirmed)
	if len(h.runner.calls) != 0 {
		t.Fatal("a step ran without confirmation")
	}
	_, err = h.eng.PlanDestroy(ctx, testModule(), cred)
	wantKind(t, err, ErrPolicyRefused, nil)
	h.eng.cfg.Policy = func() Policy { return NewPolicy(5, false) }
	pr, err := h.eng.PlanDestroy(ctx, testModule(), cred)
	if err != nil {
		t.Fatal(err)
	}
	_, err = h.eng.Apply(ctx, ApplyRequest{Spec: testModule(), Cred: cred, PlanHash: pr.Hash})
	wantKind(t, err, ErrPlanRejected, ErrPlanMismatch)
	h.eng.cfg.Policy = func() Policy { return NewPolicy(1, false) }
	_, err = h.eng.Destroy(ctx, ApplyRequest{Spec: testModule(), Cred: cred, PlanHash: pr.Hash, ConfirmDestroy: true})
	wantKind(t, err, ErrPolicyRefused, nil)
	h.eng.cfg.Policy = func() Policy { return NewPolicy(5, false) }
	h.runner.applyState = []byte(`{"serial":3}`)
	res, err := h.eng.Destroy(ctx, ApplyRequest{Spec: testModule(), Cred: cred, PlanHash: pr.Hash, ConfirmDestroy: true})
	if err != nil || res.Step != StepDestroy || res.StateVersion != 2 {
		t.Fatalf("destroy = %+v, %v", res, err)
	}
	h.assertClean(t)
}
