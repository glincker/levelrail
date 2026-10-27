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

const udpComposeYAML = `
services:
  dns:
    image: alpine:3.20
    ports: ["8080:80", "53:53/udp"]
`

func TestHandleDeployCompose_UDPPublishedPorts(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	deploy := func(app, body string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/"+app+"/compose", body))
		return rec
	}

	rec := deploy("dnsapp", udpComposeYAML)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var got composeDeployResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	want := store.PublishedPort{ContainerPort: 53, HostPort: 53, Protocol: "udp"}
	if len(got.Services) != 1 || len(got.Services[0].PublishedPorts) != 1 || got.Services[0].PublishedPorts[0] != want {
		t.Fatalf("services = %+v, want published_ports [%+v]", got.Services, want)
	}
	if !strings.Contains(rec.Body.String(), `"published_ports":[{"container_port":53,"host_port":53,"protocol":"udp"}]`) {
		t.Fatalf("body = %s, want the published_ports JSON shape", rec.Body.String())
	}

	rec = deploy("other", udpComposeYAML)
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), `"code":"published_port_conflict"`) {
		t.Fatalf("second app on udp 53: status = %d, body = %s, want 409 published_port_conflict", rec.Code, rec.Body.String())
	}

	rec = deploy("tcp53", "services:\n  web:\n    image: alpine:3.20\n    ports: [\"53:53\"]\n")
	if rec.Code != http.StatusOK {
		t.Fatalf("tcp 53 beside udp 53: status = %d, body = %s", rec.Code, rec.Body.String())
	}

	rec = deploy("bad", "services:\n  web:\n    image: alpine:3.20\n    ports: [\"51820/udp\"]\n")
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), `"code":"published_port_invalid"`) {
		t.Fatalf("udp without host port: status = %d, body = %s", rec.Code, rec.Body.String())
	}

	if err := db.UpdateServiceNode(context.Background(), "dnsapp-dns", "node-b"); err != nil {
		t.Fatalf("UpdateServiceNode() error = %v", err)
	}
	rec = deploy("dnsapp", udpComposeYAML)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), `"code":"published_port_remote_node"`) {
		t.Fatalf("remote node: status = %d, body = %s", rec.Code, rec.Body.String())
	}
}
