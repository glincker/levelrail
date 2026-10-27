package api

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestEditAppDomains_RefusesAppWithoutHTTPPort(t *testing.T) {
	rt, db, cookie, _ := quotaFixture(t)
	svc := store.DesiredService{Name: "dns", Image: "levelrail/dns:1",
		PublishedPorts: []store.PublishedPort{{ContainerPort: 53, HostPort: 5353, Protocol: "udp"}}}
	if err := db.SaveDesiredService(context.Background(), svc); err != nil {
		t.Fatal(err)
	}
	code, body := quotaDo(t, rt, cookie, http.MethodPatch, "/api/v1/apps/dns/domains", `{"add":["dns.example.com"]}`)
	if code != http.StatusBadRequest || !strings.Contains(body, domainNeedsHTTPPortCode) {
		t.Fatalf("add domain to a portless app = %d %s, want 400 %s", code, body, domainNeedsHTTPPortCode)
	}
	got, err := db.GetDesiredService(context.Background(), "dns")
	if err != nil || len(got.Domains) != 0 {
		t.Fatalf("domains = %v %v, want none", got.Domains, err)
	}
}
