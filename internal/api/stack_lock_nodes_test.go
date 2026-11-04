package api

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestStackLockNodeRoutes(t *testing.T) {
	rt, db, cookie, _ := stackLockFixture(t, "lock")
	now := time.Now().UTC()
	for _, n := range []store.Node{
		{ID: "n-1", Name: "edge-1", Status: store.NodeStatusOnline, CreatedAt: now, UpdatedAt: now},
		{ID: "n-2", Name: "edge-2", Status: store.NodeStatusOnline, CreatedAt: now, UpdatedAt: now},
	} {
		if err := db.SaveNode(t.Context(), n); err != nil {
			t.Fatal(err)
		}
	}
	stack, err := db.GetStackByName(t.Context(), "shop-prod")
	if err != nil {
		t.Fatal(err)
	}
	entry := store.StackLedgerEntry{StackID: stack.ID, Kind: "Node", Key: "Node/edge-1", LiveID: "edge-1", SinceRevision: 1}
	if err := db.UpsertStackResource(t.Context(), entry); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{"/api/v1/nodes/n-1/taints", "/api/v1/nodes/n-1/labels"} {
		body := `{"taints":[],"labels":{}}`
		code, out := stackLockCall(t, rt, authedRequest(t, cookie, http.MethodPut, p, body))
		if code != http.StatusConflict || out.Code != "stack_locked" || !strings.Contains(out.Error, "shop-prod") {
			t.Errorf("PUT %s = %d %+v, want 409 stack_locked", p, code, out)
		}
		req := authedRequest(t, cookie, http.MethodPut, p, body)
		req.Header.Set("X-Levelrail-Override-Stack-Lock", "shop-prod")
		if code, out := stackLockCall(t, rt, req); out.Code == "stack_locked" || code == http.StatusConflict {
			t.Errorf("override PUT %s = %d %+v", p, code, out)
		}
		other := strings.Replace(p, "n-1", "n-2", 1)
		if code, out := stackLockCall(t, rt, authedRequest(t, cookie, http.MethodPut, other, body)); out.Code == "stack_locked" || code == http.StatusConflict {
			t.Errorf("unowned node PUT %s = %d %+v", other, code, out)
		}
	}
}
