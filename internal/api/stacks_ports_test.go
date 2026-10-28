package api

import (
	"io"
	"log/slog"
	"net/http"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/stackdrift"
	"github.com/GLINCKER/levelrail/internal/store"
)

func udpStackDocs(hostPort string) string {
	return `version: 1
kind: Project
metadata: {name: shop}
---
version: 1
kind: App
metadata: {name: dns}
spec:
  project: shop
  service:
    build: {type: image, image: "ghcr.io/acme/dns:1"}
    port: 8080
    restart: "no"
    published_ports:
      - {container_port: 53, host_port: ` + hostPort + `, protocol: udp}
`
}

func udpPorts(t *testing.T, db *store.DB) []store.PublishedPort {
	t.Helper()
	svc, err := db.GetDesiredService(t.Context(), "dns")
	if err != nil {
		t.Fatal(err)
	}
	if svc.Health == nil || svc.Health.Restart != "no" {
		t.Fatalf("restart not saved: %+v", svc.Health)
	}
	return svc.PublishedPorts
}

func TestStackPublishedPortsRefusedByAdmissionStoreNothing(t *testing.T) {
	rt, db, cookie := stackFixture(t)
	held := store.DesiredService{Name: "resolver", Image: "ghcr.io/acme/resolver:1", Port: 8053,
		PublishedPorts: []store.PublishedPort{{ContainerPort: 53, HostPort: 53, Protocol: "udp"}}}
	if err := db.SaveDesiredService(t.Context(), held); err != nil {
		t.Fatal(err)
	}
	_, out := stackApply(t, rt, cookie, udpStackDocs("53"), nil)
	if out.Result == nil || out.Result.OK() {
		t.Fatalf("conflicting port applied: %+v", out)
	}
	var msg string
	for _, r := range out.Result.Results {
		msg += r.Error
	}
	if !strings.Contains(msg, `host port 53/udp for service "dns" is already published by "resolver"`) {
		t.Fatalf("refusal message = %q", msg)
	}
	if _, err := db.GetDesiredService(t.Context(), "dns"); err == nil {
		t.Fatal("refused app was stored")
	}
}

func TestStackPublishedPortsDriftAndRollback(t *testing.T) {
	rt, db, cookie := stackFixture(t)
	for i, port := range []string{"5353", "5354"} {
		if code, out := stackApply(t, rt, cookie, udpStackDocs(port), nil); code != http.StatusOK || out.Result == nil || !out.Result.OK() {
			t.Fatalf("apply %d = %d %+v", i+1, code, out)
		}
	}
	if p := udpPorts(t, db); len(p) != 1 || p[0].HostPort != 5354 || p[0].Protocol != "udp" {
		t.Fatalf("ports after apply = %+v", p)
	}

	c := &stackdrift.Checker{Store: db, Live: rt.StackDriftLive, ProjectName: rt.StackProjectName, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
	c.CheckAll(t.Context())
	svc, _ := db.GetDesiredService(t.Context(), "dns")
	svc.PublishedPorts = []store.PublishedPort{{ContainerPort: 53, HostPort: 5399, Protocol: "udp"}}
	if err := db.SaveDesiredService(t.Context(), *svc); err != nil {
		t.Fatal(err)
	}
	c.CheckAll(t.Context())
	var s stackResource
	iacCall(t, rt, authedRequest(t, cookie, http.MethodGet, "/api/v1/stacks/shop-prod", ""), &s)
	if s.Drift.Status != "drifted" {
		t.Fatalf("out-of-band port change not drift: %+v", s.Drift)
	}

	var rolled stackApplyResponse
	if code := iacCall(t, rt, authedRequest(t, cookie, http.MethodPost, "/api/v1/stacks/shop-prod/revisions/1/rollback", ""), &rolled); code != http.StatusOK {
		t.Fatalf("rollback = %d %+v", code, rolled)
	}
	if p := udpPorts(t, db); len(p) != 1 || p[0].HostPort != 5353 {
		t.Fatalf("rollback ports = %+v", p)
	}
}
