package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/spec"
	"github.com/GLINCKER/levelrail/internal/store"
)

// A spec file deploy (app.yaml through webhook, git push or a build) gets
// the same refusals as a compose deploy, and its own redeploy still passes.
func TestAdmitPublishedPorts_SpecDeploy(t *testing.T) {
	stubUDPListeners(t)
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/dnsapp/compose", udpComposeYAML))
	if rec.Code != http.StatusOK {
		t.Fatalf("compose deploy: status = %d, body = %s", rec.Code, rec.Body.String())
	}
	udp := func(port int) []store.PublishedPort {
		return []store.PublishedPort{{ContainerPort: port, HostPort: port, Protocol: spec.PortProtocolUDP}}
	}
	ctx := context.Background()

	if _, err := rt.AdmitPublishedPorts(ctx, store.DesiredService{Name: "repo-web", PublishedPorts: udp(53)}); err == nil || !strings.Contains(err.Error(), `already published by "dnsapp-dns"`) {
		t.Fatalf("taken port: error = %v, want already published", err)
	}
	if _, err := rt.AdmitPublishedPorts(ctx, store.DesiredService{Name: "repo-web", PublishedPorts: udp(443)}); err == nil || !strings.Contains(err.Error(), "reserved by the control plane") {
		t.Fatalf("reserved port: error = %v, want reserved", err)
	}
	if _, err := rt.AdmitPublishedPorts(ctx, store.DesiredService{Name: "repo-web", PublishedPorts: []store.PublishedPort{{ContainerPort: 53, Protocol: spec.PortProtocolUDP}}}); err == nil {
		t.Fatal("missing host port admitted")
	}
	release, err := rt.AdmitPublishedPorts(ctx, store.DesiredService{Name: "dnsapp-dns", AppID: "dnsapp", PublishedPorts: udp(53)})
	if err != nil {
		t.Fatalf("own redeploy: error = %v", err)
	}
	release()
}
