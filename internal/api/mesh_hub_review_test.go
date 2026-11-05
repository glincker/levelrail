package api

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
	"time"
)

func TestJoinTokenMeshReportsResolutionFailure(t *testing.T) {
	now := time.Unix(1000, 0)
	db := openTestDB(t)
	rt := NewRouter(nil, testBrand(), db, WithDoctorAgentAdvertise("cp.example.com", 9443))
	rt.SetMeshHubResolver(hubTestResolver(&fakeHubLookup{err: &net.DNSError{IsNotFound: true}}, &now))
	cookie := loginTestSession(t, rt, db)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/nodes/join-tokens", `{"mesh":true}`))
	body := rec.Body.String()
	if rec.Code != http.StatusBadRequest || !strings.Contains(body, "NXDOMAIN") || strings.Contains(body, "does not resolve names") {
		t.Fatalf("%d %s", rec.Code, body)
	}
}

func TestDoctorMeshHubPrivateAnswerMentionsLAN(t *testing.T) {
	now := time.Unix(1000, 0)
	rt, _ := newDoctorTestRouter(t)
	rt.doctorAgentAdvertiseHost = "server.example.com"
	rt.mesh = &fakeMeshStatus{}
	rt.meshHubResolver = hubTestResolver(&fakeHubLookup{addrs: []netip.Addr{netip.MustParseAddr("10.0.0.5")}}, &now)
	c := rt.doctorCheckMeshHubEndpoint(context.Background())
	if strings.Contains(c.Message, "cannot reach") || !strings.Contains(c.Message, "LAN") || !strings.Contains(c.Message, "IP itself") {
		t.Fatalf("message = %q", c.Message)
	}
}

func TestDoctorMeshHubStaleShowsSince(t *testing.T) {
	now := time.Unix(1000, 0)
	l := &fakeHubLookup{addrs: []netip.Addr{netip.MustParseAddr("203.0.113.7")}}
	rt, _ := newDoctorTestRouter(t)
	rt.doctorAgentAdvertiseHost = "server.example.com"
	rt.mesh = &fakeMeshStatus{}
	rt.meshHubResolver = hubTestResolver(l, &now)
	_ = rt.doctorCheckMeshHubEndpoint(context.Background())
	now = now.Add(time.Hour)
	l.addrs, l.err = nil, &net.DNSError{IsNotFound: true}
	c := rt.doctorCheckMeshHubEndpoint(context.Background())
	if !strings.Contains(c.Message, "since 1970-01-01T00:16:40Z") {
		t.Fatalf("message = %q", c.Message)
	}
}
