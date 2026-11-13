package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/GLINCKER/levelrail/internal/eventhook"
	"github.com/GLINCKER/levelrail/internal/iac"
	"github.com/GLINCKER/levelrail/internal/store"
)

// captureEvents records emitted events; the other service methods are unused.
type captureEvents struct {
	EventWebhookService
	mu  sync.Mutex
	got []eventhook.Event
}

func (c *captureEvents) Emit(e eventhook.Event) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.got = append(c.got, e)
}

func (c *captureEvents) json(t *testing.T) string {
	t.Helper()
	c.mu.Lock()
	defer c.mu.Unlock()
	b, err := json.Marshal(c.got)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestStackEventPayloadHasNoValues(t *testing.T) {
	rt, _ := newTestRouter(t)
	sink := &captureEvents{}
	rt.SetEventWebhooks(sink)

	const hash = "0123456789abcdef0123456789abcdef"
	rt.emitStackEvent(t.Context(), stackEventApplied, "shop-prod", "user:alice", hash, 3,
		[]string{"app/web", "database/db"}, map[string]int{"create": 1, "update": 1})

	if len(sink.got) != 1 || sink.got[0].Type != "stacks.applied" {
		t.Fatalf("events = %+v", sink.got)
	}
	d := sink.got[0].Data
	if d["plan_hash"] != hash[:stackEventHashPrefix] || d["actor"] != "user:alice" || d["revision"] != 3 {
		t.Fatalf("payload = %+v", d)
	}
	raw := sink.json(t)
	if strings.Contains(raw, hash) {
		t.Fatalf("full plan hash in payload: %s", raw)
	}
	for k := range d {
		switch k {
		case "stack", "actor", "items", "items_truncated", "revision", "plan_hash", "counts":
		default:
			t.Fatalf("unexpected payload key %q", k)
		}
	}
	for _, typ := range []string{"stacks.applied", "stacks.approval_requested", "stacks.approved", "stacks.rolled_back", "stacks.drifted", "stacks.apply_failed"} {
		found := false
		for _, k := range eventhook.EventTypes() {
			found = found || k == typ
		}
		if !found {
			t.Errorf("event type %s not in the known types", typ)
		}
	}
}

func (c *captureEvents) count(typ string) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	n := 0
	for _, e := range c.got {
		if e.Type == typ {
			n++
		}
	}
	return n
}

func expectEvents(t *testing.T, sink *captureEvents, want map[string]int) {
	t.Helper()
	for _, typ := range []string{stackEventApplied, stackEventApprovalRequested, stackEventApproved, stackEventRolledBack, stackEventApplyFailed} {
		if got := sink.count(typ); got != want[typ] {
			t.Errorf("%s events = %d, want %d: %s", typ, got, want[typ], sink.json(t))
		}
	}
}

func TestStackEventsApplyAndRollback(t *testing.T) {
	rt, _, cookie := stackFixture(t)
	sink := &captureEvents{}
	rt.SetEventWebhooks(sink)
	stackApply(t, rt, cookie, stackTestDocs("ghcr.io/acme/web:1.2.3"), map[string]any{"secrets": map[string]string{"TOKEN": "hunter2-secret-value"}})
	expectEvents(t, sink, map[string]int{stackEventApplied: 1})
	stackApply(t, rt, cookie, stackTestDocs("ghcr.io/acme/web:1.2.4"), nil)
	if code := iacCall(t, rt, authedRequest(t, cookie, http.MethodPost, "/api/v1/stacks/shop-prod/revisions/1/rollback", ""), nil); code != http.StatusOK {
		t.Fatalf("rollback = %d", code)
	}
	expectEvents(t, sink, map[string]int{stackEventApplied: 2, stackEventRolledBack: 1})
	raw := sink.json(t)
	for _, leak := range []string{"hunter2-secret-value", "1.2.3", "1.2.4"} {
		if strings.Contains(raw, leak) {
			t.Fatalf("event carries a value %q: %s", leak, raw)
		}
	}
}

func TestStackEventsApproval(t *testing.T) {
	rt, _, cookie, approver := approvalFixture(t)
	sink := &captureEvents{}
	rt.SetEventWebhooks(sink)
	stackApply(t, rt, cookie, stackTestDocs("ghcr.io/acme/web:1.0.0"), map[string]any{"secrets": map[string]string{"TOKEN": "hunter2-secret-value"}})
	expectEvents(t, sink, map[string]int{stackEventApprovalRequested: 1})
	if code := tokenPost(t, rt, approver, "/api/v1/stacks/shop-prod/revisions/1/approve", "", nil); code != http.StatusOK {
		t.Fatalf("approve = %d", code)
	}
	expectEvents(t, sink, map[string]int{stackEventApprovalRequested: 1, stackEventApproved: 1, stackEventApplied: 1})
	if strings.Contains(sink.json(t), "hunter2-secret-value") {
		t.Fatalf("secret in events: %s", sink.json(t))
	}
}

func TestStackEventsFailureIsOneApplyFailed(t *testing.T) {
	rt, _ := newTestRouter(t)
	sink := &captureEvents{}
	rt.SetEventWebhooks(sink)
	rt.emitStackOutcome(t.Context(), stackRun{stack: store.StackRecord{Name: "shop-prod"}}, "user:a", iac.Plan{}, 2, store.StackRevFailed, nil)
	expectEvents(t, sink, map[string]int{stackEventApplyFailed: 1})
}
