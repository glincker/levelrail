package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestPublishedPorts_ReservedAndTCPConflicts(t *testing.T) {
	stubUDPListeners(t)
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	deploy := func(app, ports string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		body := "services:\n  s:\n    image: alpine:3.20\n    ports: [" + ports + "]\n"
		rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/"+app+"/compose", body))
		return rec
	}
	for _, ports := range []string{`"443:443/udp"`, `"9443:9443/udp"`} {
		rec := deploy("quic", ports)
		if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), `"code":"published_port_conflict"`) ||
			!strings.Contains(rec.Body.String(), "control plane") {
			t.Fatalf("reserved %s: status = %d, body = %s, want 409 naming the control plane", ports, rec.Code, rec.Body.String())
		}
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps", `{"name":"web","image":"alpine:3.20","port":80,"host_port":9000}`))
	if rec.Code >= 300 {
		t.Fatalf("create web: status = %d, body = %s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/apps/web", `{"image":"alpine:3.20","port":80,"host_port":9000,"published_ports":[{"container_port":53,"host_port":9000,"protocol":"tcp"}]}`))
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), `"code":"published_port_conflict"`) {
		t.Fatalf("tcp published port on its own host_port: status = %d, body = %s", rec.Code, rec.Body.String())
	}
	other := httptest.NewRecorder()
	rt.Handler().ServeHTTP(other, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps", `{"name":"dns","image":"alpine:3.20","port":80,"published_ports":[{"container_port":53,"host_port":9000,"protocol":"tcp"}]}`))
	if other.Code != http.StatusConflict || !strings.Contains(other.Body.String(), `"code":"published_port_conflict"`) {
		t.Fatalf("tcp published port on another app's host_port: status = %d, body = %s", other.Code, other.Body.String())
	}
	if rec := deploy("dnsudp", `"9000:53/udp"`); rec.Code != http.StatusOK {
		t.Fatalf("udp 9000 beside tcp 9000: status = %d, body = %s", rec.Code, rec.Body.String())
	}
}

func TestPublishedPorts_AdmissionHoldsLock(t *testing.T) {
	stubUDPListeners(t)
	rt, _ := newTestRouter(t)
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/", nil)
	next := []store.DesiredService{{Name: "a", PublishedPorts: []store.PublishedPort{{ContainerPort: 53, HostPort: 5353, Protocol: "udp"}}}}
	release, ok := rt.admitPublishedPorts(w, r, "a", nil, next)
	if !ok {
		t.Fatalf("admit refused: %s", w.Body.String())
	}
	if rt.publishedPortsMu.TryLock() {
		t.Fatal("admission lock free while the caller has not saved yet")
	}
	release()
	if !rt.publishedPortsMu.TryLock() {
		t.Fatal("admission lock still held after release")
	}
	rt.publishedPortsMu.Unlock()
}
