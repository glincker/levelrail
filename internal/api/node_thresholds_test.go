package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHandleNodeThresholds(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedNode(t, db, "node_a", "alpha")
	do := func(method, path, body string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, method, path, body))
		return rec
	}

	if rec := do(http.MethodPut, "/api/v1/nodes/missing/thresholds", `{"cpu_percent":95}`); rec.Code != http.StatusNotFound {
		t.Fatalf("unknown node status = %d, want 404", rec.Code)
	}
	if rec := do(http.MethodPut, "/api/v1/nodes/node_a/thresholds", `{"cpu_percent":101}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("out of range status = %d, want 400", rec.Code)
	}
	if rec := do(http.MethodPut, "/api/v1/nodes/node_a/thresholds", `{"cpu_percent":95}`); rec.Code != http.StatusOK {
		t.Fatalf("set status = %d body %s", rec.Code, rec.Body.String())
	}
	rec := do(http.MethodGet, "/api/v1/nodes/node_a/thresholds", "")
	var got nodeThresholdsResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Effective.CPUPercent == nil || *got.Effective.CPUPercent != 95 || got.Override.MemoryPercent != nil || got.Effective.MemoryPercent == nil {
		t.Fatalf("thresholds = %+v, want cpu override 95 and memory from the default", got)
	}
	if rec := do(http.MethodGet, "/api/v1/nodes/node_a/metrics?metric=cpu_percent&range=2y", ""); rec.Code != http.StatusBadRequest && rec.Code != http.StatusNotImplemented {
		t.Fatalf("bad range status = %d", rec.Code)
	}
}
