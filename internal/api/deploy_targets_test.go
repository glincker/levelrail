package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/glincker/levelrail/internal/usagecost"
)

func TestHandleListDeployTargets_Groups(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedNode(t, db, "node_a", "alpha")

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/deploy-targets", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var got []deployTargetGroup
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) != 2 || got[0].Kind != deployTargetKindLocal || got[1].Kind != deployTargetKindUnmanaged {
		t.Fatalf("groups = %+v", got)
	}
	if got[1].Nodes[0].Name != "alpha" {
		t.Errorf("unmanaged node = %+v", got[1].Nodes[0])
	}
}

func TestHandleListDeployTargets_RequiresAuth(t *testing.T) {
	rt, _ := newTestRouter(t)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/deploy-targets", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestHandleListDeployTargets_CostAndCapacity(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedNode(t, db, "node_a", "alpha")
	seedNode(t, db, "node_b", "beta")
	if err := db.ReplaceUsageCostNodes(context.Background(), []usagecost.NodePrice{{NodeID: "node_a", MonthlyPrice: 12.5, VCPUs: 2, MemoryGiB: 4}}, time.Now()); err != nil {
		t.Fatalf("seed price: %v", err)
	}
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/deploy-targets", ""))
	var got []deployTargetGroup
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	byName := map[string]deployTargetNode{}
	for _, g := range got {
		for _, n := range g.Nodes {
			byName[n.Name] = n
		}
	}
	if p := byName["alpha"].MonthlyCostEstimate; p == nil || *p != 12.5 {
		t.Errorf("alpha cost = %v, want 12.5", p)
	}
	if byName["beta"].MonthlyCostEstimate != nil || byName["beta"].FreeCapacity != nil {
		t.Errorf("beta must omit unknown fields: %+v", byName["beta"])
	}
}
