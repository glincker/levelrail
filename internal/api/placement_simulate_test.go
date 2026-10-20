package api

import (
	"encoding/json"
	"net/http"
	"testing"
)

func TestPlacementSimulation(t *testing.T) {
	rt, db, cookie := newMultiNodeRouter(t)
	seedMNNode(t, db, "n1", map[string]string{"zone": "a"})
	seedMNNode(t, db, "n2", map[string]string{"zone": "b"})
	rec := mnDo(t, rt, cookie, http.MethodGet, "/api/v1/nodes/placement-simulation?count=2", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	var got placementSimulationResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Placed) != 2 || got.Placed[0].NodeID == got.Placed[1].NodeID {
		t.Fatalf("two apps should land on two zones: %+v", got.Placed)
	}
	for _, bad := range []string{"count=0", "count=9999", "cpu_milli=-1", "memory_mib=x", "selector=bogus"} {
		if rec := mnDo(t, rt, cookie, http.MethodGet, "/api/v1/nodes/placement-simulation?"+bad, ""); rec.Code != http.StatusBadRequest {
			t.Fatalf("%s: status = %d, want 400", bad, rec.Code)
		}
	}
}

func TestFailureSimulation(t *testing.T) {
	rt, db, cookie := newMultiNodeRouter(t)
	seedMNNode(t, db, "n1", map[string]string{"zone": "a"})
	seedMNNode(t, db, "n2", map[string]string{"zone": "b"})
	seedMNApp(t, db, "n1")
	rec := mnDo(t, rt, cookie, http.MethodGet, "/api/v1/nodes/failure-simulation?zone=a", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	var got failureSimulationResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Count != 1 || got.Placed[0].NodeID != "n2" || got.Unplaced != 0 {
		t.Fatalf("web should move to n2: %+v", got)
	}
	if rec := mnDo(t, rt, cookie, http.MethodGet, "/api/v1/nodes/failure-simulation?zone=nope", ""); rec.Code != http.StatusNotFound {
		t.Fatalf("unknown zone: status = %d, want 404", rec.Code)
	}
	if rec := mnDo(t, rt, cookie, http.MethodGet, "/api/v1/nodes/failure-simulation", ""); rec.Code != http.StatusBadRequest {
		t.Fatalf("no target: status = %d, want 400", rec.Code)
	}
}
