package api

import (
	"encoding/json"
	"strings"
	"sync"
	"testing"

	"github.com/GLINCKER/levelrail/internal/eventhook"
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
