package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestStampAppNodeOffline(t *testing.T) {
	nodes := []store.Node{
		{ID: "node_up", Name: "up", Status: store.NodeStatusOnline},
		{ID: "node_down", Name: "down", Status: store.NodeStatusOffline},
	}
	r := newPlacementResolver(nodes, "", "host-a")
	for id, want := range map[string]bool{"node_up": false, "node_down": true, "": false} {
		app := appResource{NodeID: id}
		r.stampApp(&app)
		if app.NodeOffline != want {
			t.Errorf("node %q: NodeOffline = %v, want %v", id, app.NodeOffline, want)
		}
	}
}

func TestMeshAdvertiseHostNeedsIP(t *testing.T) {
	for host, want := range map[string]bool{"": false, "localhost": false, "203.0.113.5": false, "2001:db8::1": false, "cp.example.com": true, "lrqa-cp": true} {
		if got := meshAdvertiseHostNeedsIP(host); got != want {
			t.Errorf("meshAdvertiseHostNeedsIP(%q) = %v, want %v", host, got, want)
		}
	}
}

func TestJoinTokenMeshRefusesHostnameAdvertise(t *testing.T) {
	db := openTestDB(t)
	rt := NewRouter(nil, testBrand(), db, WithDoctorAgentAdvertise("cp.example.com", 9443))
	cookie := loginTestSession(t, rt, db)
	post := func(body string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/nodes/join-tokens", body))
		return rec
	}
	rec := post(`{"mesh":true}`)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "APP_AGENT_ADVERTISE_HOST") {
		t.Fatalf("mesh with hostname: %d %s", rec.Code, rec.Body.String())
	}
	if rec := post(`{"mesh":false}`); rec.Code != http.StatusCreated {
		t.Fatalf("no mesh with hostname: %d %s", rec.Code, rec.Body.String())
	}
}
