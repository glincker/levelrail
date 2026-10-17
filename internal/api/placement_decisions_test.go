package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/agent"
)

const testGiB = uint64(1) << 30

func seedCapacityCluster(t *testing.T) (*Router, *http.Cookie) {
	t.Helper()
	rt, db, cookie := newMultiNodeRouter(t)
	seedMNNode(t, db, "node_a", nil)
	seedMNNode(t, db, "node_b", nil)
	reports := agent.NewHostReportStore()
	reports.SetNodeHostReport("node_a", agent.HostReport{CPUCores: 4, MemoryTotalBytes: testGiB, DiskTotalBytes: 100 * testGiB, DiskFreeBytes: 80 * testGiB})
	reports.SetNodeHostReport("node_b", agent.HostReport{CPUCores: 4, MemoryTotalBytes: 8 * testGiB, DiskTotalBytes: 100 * testGiB, DiskFreeBytes: 80 * testGiB})
	rt.SetNodeHostReports(reports)
	return rt, cookie
}

func TestPlacementDecisionRecordedOnAutoPlace(t *testing.T) {
	rt, cookie := seedCapacityCluster(t)

	rec := mnDo(t, rt, cookie, http.MethodPost, "/api/v1/apps", `{"name":"web","image":"img:1","port":80,"resources":{"memory_bytes":2147483648}}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create = %d: %s", rec.Code, rec.Body.String())
	}

	rec = mnDo(t, rt, cookie, http.MethodGet, "/api/v1/apps/web/placement/decision", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("decision = %d: %s", rec.Code, rec.Body.String())
	}
	var got placementDecisionResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.NodeID != "node_b" || got.Recorded == nil || got.Recorded.Trigger != placementTriggerCreate {
		t.Fatalf("decision = %+v", got)
	}
	d := got.Recorded.Decision
	if d.NodeID != "node_b" || d.Why == "" || len(d.Considered) != 2 {
		t.Fatalf("recorded = %+v", d)
	}
	for _, e := range d.Considered {
		if e.NodeID == "node_a" && (!e.Capacity || !strings.Contains(e.Reason, "memory")) {
			t.Fatalf("node_a must be refused for memory: %+v", e)
		}
	}
	if got.Preview.NodeID != "node_b" {
		t.Fatalf("preview = %+v", got.Preview)
	}
}

func TestCreateFailsFastWhenNothingFits(t *testing.T) {
	rt, cookie := seedCapacityCluster(t)

	rec := mnDo(t, rt, cookie, http.MethodPost, "/api/v1/apps", `{"name":"big","image":"img:1","port":80,"resources":{"memory_bytes":68719476736}}`)
	if rec.Code != http.StatusConflict {
		t.Fatalf("create = %d: %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{"node_a", "node_b", "short by", "node_id"} {
		if !strings.Contains(body, want) {
			t.Errorf("message missing %q: %s", want, body)
		}
	}
	if rec := mnDo(t, rt, cookie, http.MethodGet, "/api/v1/apps/big", ""); rec.Code != http.StatusNotFound {
		t.Fatalf("app must not exist after a refused create, got %d", rec.Code)
	}
}

func TestExplicitNodeIsHonoredAndFlagged(t *testing.T) {
	rt, cookie := seedCapacityCluster(t)

	rec := mnDo(t, rt, cookie, http.MethodPost, "/api/v1/apps", `{"name":"pinned","image":"img:1","port":80,"node_id":"node_a","resources":{"memory_bytes":2147483648}}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create = %d: %s", rec.Code, rec.Body.String())
	}
	rec = mnDo(t, rt, cookie, http.MethodGet, "/api/v1/apps/pinned/placement/decision", "")
	var got placementDecisionResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Recorded == nil || got.Recorded.Decision.NodeID != "node_a" || len(got.Recorded.Decision.Warnings) != 1 {
		t.Fatalf("pin must be recorded with a warning: %+v", got.Recorded)
	}
}

func TestNodeAllocations(t *testing.T) {
	rt, cookie := seedCapacityCluster(t)
	mnDo(t, rt, cookie, http.MethodPost, "/api/v1/apps", `{"name":"web","image":"img:1","port":80,"resources":{"memory_bytes":536870912}}`)

	rec := mnDo(t, rt, cookie, http.MethodGet, "/api/v1/nodes/allocations", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("allocations = %d: %s", rec.Code, rec.Body.String())
	}
	var got nodeAllocationsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Nodes) != 2 || got.Policy.MemoryOvercommitPercent != 100 {
		t.Fatalf("allocations = %+v", got)
	}
	want := map[string]int64{"node_a": 1 << 30, "node_b": 8 << 30}
	for _, n := range got.Nodes {
		for _, d := range n.Dimensions {
			if d.Name == "memory" && (d.Allocatable == nil || *d.Allocatable != want[n.NodeID]) {
				t.Fatalf("%s memory allocatable = %v", n.NodeID, d.Allocatable)
			}
		}
	}
}

func TestUnreportedNodeCapacityStaysUnknown(t *testing.T) {
	rt, _, _ := newMultiNodeRouter(t)
	c := rt.nodeCapacity("never_reported")
	if c.CPUMilli.Known || c.MemoryBytes.Known || c.DiskTotalBytes.Known || c.DiskFreeBytes.Known {
		t.Fatalf("unreported capacity must be unknown, got %+v", c)
	}
}

func TestPlacementDecisionUnknownApp(t *testing.T) {
	rt, _, cookie := newMultiNodeRouter(t)
	if rec := mnDo(t, rt, cookie, http.MethodGet, "/api/v1/apps/ghost/placement/decision", ""); rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d", rec.Code)
	}
}
