package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestHandleListNodeEvents(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedNode(t, db, "node_a", "alpha")
	ctx := context.Background()
	for _, s := range []store.NodeStatus{store.NodeStatusOnline, store.NodeStatusOffline} {
		if err := db.UpdateNodeStatus(ctx, "node_a", s); err != nil {
			t.Fatalf("UpdateNodeStatus() error = %v", err)
		}
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/nodes/node_a/events", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var got []nodeStatusEventResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) != 2 || got[0].ToStatus != "offline" || got[1].ToStatus != "online" {
		t.Errorf("events = %+v, want offline then online (newest first)", got)
	}

	rec = httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/nodes/node_a/events?limit=1", ""))
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	if len(got) != 1 {
		t.Errorf("limit=1 returned %d events", len(got))
	}
}

func TestHandleListNodeEvents_Errors(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedNode(t, db, "node_a", "alpha")

	tests := []struct {
		name string
		path string
		want int
	}{
		{"unknown node", "/api/v1/nodes/nope/events", http.StatusNotFound},
		{"limit not a number", "/api/v1/nodes/node_a/events?limit=abc", http.StatusBadRequest},
		{"limit too large", "/api/v1/nodes/node_a/events?limit=9999", http.StatusBadRequest},
		{"limit zero", "/api/v1/nodes/node_a/events?limit=0", http.StatusBadRequest},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, tc.path, ""))
			if rec.Code != tc.want {
				t.Errorf("status = %d, want %d", rec.Code, tc.want)
			}
		})
	}
}
