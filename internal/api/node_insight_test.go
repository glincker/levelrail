package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/GLINCKER/levelrail/internal/nodeinsight"
	"github.com/GLINCKER/levelrail/internal/store"
)

func TestNodeInsightRoutes(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedNode(t, db, "node_a", "alpha")
	if err := db.UpdateNodeStatus(context.Background(), "node_a", store.NodeStatusOnline); err != nil {
		t.Fatal(err)
	}

	get := func(path string, into any) int {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, path, ""))
		if rec.Code == http.StatusOK {
			if err := json.Unmarshal(rec.Body.Bytes(), into); err != nil {
				t.Fatalf("decode %s: %v", path, err)
			}
		}
		return rec.Code
	}

	var tl nodeinsight.Timeline
	if code := get("/api/v1/nodes/node_a/timeline?window=2h", &tl); code != http.StatusOK || len(tl.Segments) == 0 {
		t.Fatalf("timeline code=%d segments=%d", code, len(tl.Segments))
	}
	var doc nodeDoctorResponse
	if code := get("/api/v1/nodes/node_a/doctor", &doc); code != http.StatusOK || len(doc.Checks) == 0 {
		t.Fatalf("doctor code=%d checks=%d", code, len(doc.Checks))
	}
	var skew nodeinsight.SkewReport
	if code := get("/api/v1/nodes/version-skew", &skew); code != http.StatusOK || len(skew.Nodes) == 0 {
		t.Fatalf("skew code=%d nodes=%d", code, len(skew.Nodes))
	}
	var none nodeinsight.Timeline
	if code := get("/api/v1/nodes/nope/timeline", &none); code != http.StatusNotFound {
		t.Fatalf("unknown node code=%d", code)
	}
	if code := get("/api/v1/nodes/node_a/timeline?window=banana", &none); code != http.StatusBadRequest {
		t.Fatalf("bad window code=%d", code)
	}
}
