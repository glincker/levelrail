package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestPublishedPorts_AppPutCreate(t *testing.T) {
	stubUDPListeners(t)
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	do := func(method, path, body string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, method, path, body))
		return rec
	}
	stored := func(name string) []store.PublishedPort {
		svc, err := db.GetDesiredService(context.Background(), name)
		if err != nil {
			t.Fatalf("GetDesiredService(%s) error = %v", name, err)
		}
		return svc.PublishedPorts
	}
	wg := store.PublishedPort{ContainerPort: 51820, HostPort: 51820, Protocol: "udp"}

	if rec := do(http.MethodPost, "/api/v1/apps/vpn/compose", "services:\n  vpn:\n    image: alpine:3.20\n    ports: [\"51821\", \"51820:51820/udp\"]\n"); rec.Code != http.StatusOK {
		t.Fatalf("compose deploy: status = %d, body = %s", rec.Code, rec.Body.String())
	}

	// (a) an ordinary env edit keeps the port, whether the dashboard echoes the field back or a client omits it.
	get := do(http.MethodGet, "/api/v1/apps/vpn-vpn", "")
	var detail map[string]json.RawMessage
	if err := json.Unmarshal(get.Body.Bytes(), &detail); err != nil {
		t.Fatalf("decode GET: %v, body = %s", err, get.Body.String())
	}
	detail["env"] = json.RawMessage(`{"A":"1"}`)
	echoedBody, _ := json.Marshal(detail)
	for _, body := range []string{
		string(echoedBody),
		`{"image":"alpine:3.20","port":51821,"env":{"A":"2"}}`,
	} {
		if rec := do(http.MethodPut, "/api/v1/apps/vpn-vpn", body); rec.Code != http.StatusOK {
			t.Fatalf("PUT %s: status = %d, body = %s", body, rec.Code, rec.Body.String())
		}
		if got := stored("vpn-vpn"); len(got) != 1 || got[0] != wg {
			t.Fatalf("after PUT %s: published_ports = %+v, want [%+v]", body, got, wg)
		}
	}

	// (b) new ports persist, and the compose admission codes apply.
	rec := do(http.MethodPut, "/api/v1/apps/vpn-vpn", `{"image":"alpine:3.20","port":51821,"published_ports":[{"container_port":51820,"host_port":51822,"protocol":"udp"}]}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT new port: status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if got := stored("vpn-vpn"); len(got) != 1 || got[0].HostPort != 51822 {
		t.Fatalf("published_ports = %+v, want host port 51822", got)
	}
	var echoed appResource
	if err := json.Unmarshal(rec.Body.Bytes(), &echoed); err != nil || len(echoed.PublishedPorts) != 1 {
		t.Fatalf("response published_ports = %+v (err %v), want one entry", echoed.PublishedPorts, err)
	}
	if rec := do(http.MethodPost, "/api/v1/apps", `{"name":"plain","image":"alpine:3.20","port":80}`); rec.Code != http.StatusCreated && rec.Code != http.StatusOK {
		t.Fatalf("create plain: status = %d, body = %s", rec.Code, rec.Body.String())
	}
	for body, code := range map[string]string{
		`{"image":"alpine:3.20","port":80,"published_ports":[{"container_port":53,"host_port":51822,"protocol":"udp"}]}`:                                                    "published_port_conflict",
		`{"image":"alpine:3.20","port":80,"published_ports":[{"container_port":53,"protocol":"udp"}]}`:                                                                      "published_port_invalid",
		`{"image":"alpine:3.20","port":80,"published_ports":[{"container_port":53,"host_port":53,"protocol":"udp"},{"container_port":54,"host_port":53,"protocol":"udp"}]}`: "published_port_invalid",
	} {
		rec := do(http.MethodPut, "/api/v1/apps/plain", body)
		if rec.Code < 400 || !strings.Contains(rec.Body.String(), `"code":"`+code+`"`) {
			t.Fatalf("PUT %s: status = %d, body = %s, want code %s", body, rec.Code, rec.Body.String(), code)
		}
	}
	if got := stored("plain"); len(got) != 0 {
		t.Fatalf("refused PUT stored published_ports = %+v", got)
	}

	// (c) an explicit empty array clears.
	if rec := do(http.MethodPut, "/api/v1/apps/vpn-vpn", `{"image":"alpine:3.20","port":51821,"published_ports":[]}`); rec.Code != http.StatusOK {
		t.Fatalf("PUT clear: status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if got := stored("vpn-vpn"); len(got) != 0 {
		t.Fatalf("after clear: published_ports = %+v, want none", got)
	}

	// (d) create carries the field and runs the same admission.
	if rec := do(http.MethodPost, "/api/v1/apps", `{"name":"dns","image":"alpine:3.20","port":80,"published_ports":[{"container_port":53,"host_port":5353,"protocol":"udp"}]}`); rec.Code != http.StatusCreated && rec.Code != http.StatusOK {
		t.Fatalf("create with ports: status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if got := stored("dns"); len(got) != 1 || got[0].HostPort != 5353 {
		t.Fatalf("created published_ports = %+v, want host port 5353", got)
	}
	rec = do(http.MethodPost, "/api/v1/apps", `{"name":"dns2","image":"alpine:3.20","port":80,"published_ports":[{"container_port":53,"host_port":5353,"protocol":"udp"}]}`)
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), `"code":"published_port_conflict"`) {
		t.Fatalf("create on a taken port: status = %d, body = %s, want 409 published_port_conflict", rec.Code, rec.Body.String())
	}
}
