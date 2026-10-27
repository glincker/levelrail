package api

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestPublishedPorts_HostHolesAreRefused(t *testing.T) {
	stubUDPListeners(t)
	prev := hostListeningPorts
	hostListeningPorts = func() []int { return []int{25000} }
	t.Cleanup(func() { hostListeningPorts = prev })
	rt, db, cookie, _ := quotaFixture(t)
	rt.hostFirewallSSHPorts = []int{22}
	rt.firewallRequiredPorts = []int{7946}
	ctx := context.Background()
	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "streamer", Image: "levelrail/s:1", Port: 80}); err != nil {
		t.Fatal(err)
	}
	if err := db.SaveAppStream(ctx, store.AppStream{ID: "st1", ServiceName: "streamer", ContainerPort: 5432, HostPort: 6000, Protocol: "tcp", CreatedAt: "2026-10-27T00:00:00Z"}); err != nil {
		t.Fatal(err)
	}
	create := func(name, extra string) (int, string) {
		return quotaDo(t, rt, cookie, http.MethodPost, "/api/v1/apps", fmt.Sprintf(`{"name":%q,"image":"levelrail/web:1","port":80%s}`, name, extra))
	}
	published := func(port int, proto string) string {
		return fmt.Sprintf(`,"published_ports":[{"container_port":53,"host_port":%d,"protocol":%q}]`, port, proto)
	}
	cases := []struct {
		name, extra, code string
	}{
		{"ssh", published(22, "tcp"), codePublishedPortConflict},
		{"fwguard", published(7946, "udp"), codePublishedPortConflict},
		{"listener", published(25000, "tcp"), codePublishedPortInUse},
		{"stream", published(6000, "tcp"), codePublishedPortConflict},
	}
	for _, c := range cases {
		if code, body := create(c.name, c.extra); code != http.StatusConflict || !strings.Contains(body, c.code) {
			t.Errorf("%s: create = %d %s, want 409 %s", c.name, code, body, c.code)
		}
	}
	if code, body := create("udp25000", published(25000, "udp")); code != http.StatusCreated {
		t.Fatalf("udp beside a tcp listener = %d %s, want 201", code, body)
	}
	if code, body := create("pub7000", published(7000, "tcp")); code != http.StatusCreated {
		t.Fatalf("create pub7000 = %d %s", code, body)
	}
	if code, body := create("pinned", `,"host_port":7000`); code != http.StatusConflict || !strings.Contains(body, codePublishedPortConflict) {
		t.Fatalf("host_port on another app's published tcp port = %d %s, want 409", code, body)
	}
}
