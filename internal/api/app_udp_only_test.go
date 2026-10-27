package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

// A DNS or VPN style service publishes only UDP and has no HTTP port, so
// create and update must accept port 0 when published_ports carries one.
func TestPublishedPorts_UDPOnlyApp(t *testing.T) {
	stubUDPListeners(t)
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	do := func(method, path, body string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, method, path, body))
		return rec
	}
	stored := func(name string) store.DesiredService {
		svc, err := db.GetDesiredService(context.Background(), name)
		if err != nil {
			t.Fatalf("GetDesiredService(%s) error = %v", name, err)
		}
		return *svc
	}
	dns := store.PublishedPort{ContainerPort: 53, HostPort: 5354, Protocol: "udp"}

	if rec := do(http.MethodPost, "/api/v1/apps/dns/compose", "services:\n  dns:\n    image: alpine:3.20\n    ports: [\"5354:53/udp\"]\n"); rec.Code != http.StatusOK {
		t.Fatalf("compose deploy: status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if got := stored("dns-dns"); got.Port != 0 || len(got.PublishedPorts) != 1 || got.PublishedPorts[0] != dns {
		t.Fatalf("deployed service port = %d, published_ports = %+v, want 0 and [%+v]", got.Port, got.PublishedPorts, dns)
	}

	get := do(http.MethodGet, "/api/v1/apps/dns-dns", "")
	var detail map[string]json.RawMessage
	if err := json.Unmarshal(get.Body.Bytes(), &detail); err != nil {
		t.Fatalf("decode GET: %v, body = %s", err, get.Body.String())
	}
	detail["env"] = json.RawMessage(`{"A":"1"}`)
	echoed, _ := json.Marshal(detail)
	for _, body := range []string{string(echoed), `{"image":"alpine:3.20","env":{"A":"2"}}`} {
		if rec := do(http.MethodPut, "/api/v1/apps/dns-dns", body); rec.Code != http.StatusOK {
			t.Fatalf("PUT %s: status = %d, body = %s", body, rec.Code, rec.Body.String())
		}
		if got := stored("dns-dns"); len(got.PublishedPorts) != 1 || got.PublishedPorts[0] != dns {
			t.Fatalf("after PUT %s: published_ports = %+v, want [%+v]", body, got.PublishedPorts, dns)
		}
	}

	if rec := do(http.MethodPost, "/api/v1/apps", `{"name":"stun","image":"alpine:3.20","published_ports":[{"container_port":3478,"host_port":3478,"protocol":"udp"}]}`); rec.Code != http.StatusCreated && rec.Code != http.StatusOK {
		t.Fatalf("create UDP-only: status = %d, body = %s", rec.Code, rec.Body.String())
	}

	for name, body := range map[string]string{
		"no port and no published port":    `{"name":"bare","image":"alpine:3.20"}`,
		"domain needs a port to route to":  `{"name":"routed","image":"alpine:3.20","domains":["a.example.com"],"published_ports":[{"container_port":53,"host_port":5355,"protocol":"udp"}]}`,
		"negative port with published one": `{"name":"neg","image":"alpine:3.20","port":-1,"published_ports":[{"container_port":53,"host_port":5356,"protocol":"udp"}]}`,
	} {
		if rec := do(http.MethodPost, "/api/v1/apps", body); rec.Code != http.StatusBadRequest {
			t.Errorf("%s: status = %d, body = %s, want 400", name, rec.Code, rec.Body.String())
		}
	}
	if rec := do(http.MethodPut, "/api/v1/apps/dns-dns", `{"image":"alpine:3.20","published_ports":[]}`); rec.Code != http.StatusOK {
		t.Fatalf("clearing the ports of a portless app: status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if got := stored("dns-dns"); len(got.PublishedPorts) != 0 {
		t.Fatalf("after clear: published_ports = %+v, want none", got.PublishedPorts)
	}
	if rec := do(http.MethodPost, "/api/v1/apps", `{"name":"web","image":"alpine:3.20","port":80}`); rec.Code >= 300 {
		t.Fatalf("create web: status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if rec := do(http.MethodPut, "/api/v1/apps/web", `{"image":"alpine:3.20"}`); rec.Code != http.StatusBadRequest {
		t.Errorf("dropping the port of a web app: status = %d, body = %s, want 400", rec.Code, rec.Body.String())
	}
}
