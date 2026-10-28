package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/GLINCKER/levelrail/internal/bindaddr"
	"github.com/GLINCKER/levelrail/internal/store"
)

type fakeUFW struct {
	mu        sync.Mutex
	calls     []string
	active    bool
	failOn    string
	installed bool
	rules     string
}

func (f *fakeUFW) run(_ context.Context, name string, args ...string) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	call := name + " " + strings.Join(args, " ")
	if len(args) > 0 && args[0] == "status" {
		if f.active {
			return []byte("Status: active\nDefault: deny (incoming), allow (outgoing), disabled (routed)\n" + f.rules), nil
		}
		return []byte("Status: inactive\n"), nil
	}
	f.calls = append(f.calls, call)
	if f.failOn != "" && strings.Contains(call, f.failOn) {
		return []byte("boom"), errors.New("exit status 1")
	}
	if strings.Contains(call, "--force enable") {
		f.active = true
	}
	if strings.Contains(call, "ufw disable") {
		f.active = false
	}
	return nil, nil
}

func (f *fakeUFW) lookPath(string) (string, error) {
	if f.installed {
		return "/usr/sbin/ufw", nil
	}
	return "", errors.New("not found")
}

func hostFirewallRouter(t *testing.T, f *fakeUFW) (*Router, *http.Cookie) {
	t.Helper()
	db := openTestDB(t)
	rt := NewRouter(discardLogger(), testBrand(), db, WithHostFirewallRunner(f.run, f.lookPath))
	return rt, loginTestSession(t, rt, db)
}

func postFirewall(t *testing.T, rt *Router, cookie *http.Cookie, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, path, body))
	return rec
}

func TestEnableHostFirewall_AllowsSSHBeforeEnabling(t *testing.T) {
	f := &fakeUFW{installed: true}
	rt, cookie := hostFirewallRouter(t, f)

	rec := postFirewall(t, rt, cookie, "/api/v1/firewall/host/enable", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	enableAt, sshAt, udpAt := -1, -1, -1
	for i, c := range f.calls {
		switch {
		case strings.Contains(c, "--force enable"):
			enableAt = i
		case strings.HasSuffix(c, "allow 22/tcp"):
			sshAt = i
		case strings.HasSuffix(c, "allow 443/udp"):
			udpAt = i
		}
	}
	if sshAt != 0 || enableAt != len(f.calls)-1 || udpAt < 0 || udpAt > enableAt {
		t.Errorf("calls = %v, want SSH first, 443/udp allowed, enable last", f.calls)
	}
	var res hostFirewallResource
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil || !res.Active {
		t.Errorf("response = %s, want active status", rec.Body.String())
	}
}

func TestEnableHostFirewall_FailedAllowNeverEnables(t *testing.T) {
	f := &fakeUFW{installed: true, failOn: "allow 80/tcp"}
	rt, cookie := hostFirewallRouter(t, f)

	rec := postFirewall(t, rt, cookie, "/api/v1/firewall/host/enable", "")
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
	for _, c := range f.calls {
		if strings.Contains(c, "enable") {
			t.Errorf("calls = %v, enable ran after a failed allow", f.calls)
		}
	}
}

func TestEnableHostFirewall_DryRunChangesNothing(t *testing.T) {
	f := &fakeUFW{installed: true}
	rt, cookie := hostFirewallRouter(t, f)

	rec := postFirewall(t, rt, cookie, "/api/v1/firewall/host/enable", `{"dry_run":true}`)
	if rec.Code != http.StatusOK || len(f.calls) != 0 {
		t.Fatalf("status = %d, calls = %v, want 200 and no commands", rec.Code, f.calls)
	}
}

func TestHostFirewall_NotInstalledIsRefused(t *testing.T) {
	f := &fakeUFW{}
	rt, cookie := hostFirewallRouter(t, f)

	for _, path := range []string{"/api/v1/firewall/host/enable", "/api/v1/firewall/host/disable"} {
		if rec := postFirewall(t, rt, cookie, path, ""); rec.Code != http.StatusConflict {
			t.Errorf("%s status = %d, want 409", path, rec.Code)
		}
	}
}

func TestDisableHostFirewall(t *testing.T) {
	f := &fakeUFW{installed: true, active: true}
	rt, cookie := hostFirewallRouter(t, f)

	rec := postFirewall(t, rt, cookie, "/api/v1/firewall/host/disable", "")
	if rec.Code != http.StatusOK || len(f.calls) != 1 || f.calls[0] != "ufw disable" {
		t.Fatalf("status = %d, calls = %v, want one ufw disable", rec.Code, f.calls)
	}
}

func TestHostFirewallRequired_SkipsInvalidPorts(t *testing.T) {
	rt := NewRouter(discardLogger(), testBrand(), openTestDB(t), WithFirewallRequiredPorts([]int{0, 8080, 70000}))
	for _, p := range rt.hostFirewallRequired(t.Context()) {
		if p.Port < 1 || p.Port > 65535 {
			t.Errorf("required ports include %d, want only valid ports", p.Port)
		}
	}
}

func TestHostFirewallRequired_IncludesPublishedPorts(t *testing.T) {
	db := openTestDB(t)
	rt := NewRouter(discardLogger(), testBrand(), db)
	pub, priv := 25565, 9999
	for _, svc := range []store.DesiredService{
		{Name: "game", Image: "img", Port: 25565, HostPort: &pub, BindAddress: bindaddr.Public},
		{Name: "dbgate", Image: "img", Port: 5432, HostPort: &priv, BindAddress: bindaddr.Private},
	} {
		if err := db.SaveDesiredService(t.Context(), svc); err != nil {
			t.Fatalf("save %s: %v", svc.Name, err)
		}
	}
	var got []int
	for _, p := range rt.hostFirewallRequired(t.Context()) {
		got = append(got, p.Port)
	}
	if !slices.Contains(got, 25565) || slices.Contains(got, 9999) {
		t.Errorf("required ports = %v, want 25565 (public) and not 9999 (loopback)", got)
	}
}

func TestHostFirewallStatus_ListsUDPPublishedPorts(t *testing.T) {
	f := &fakeUFW{installed: true, active: true, rules: "To Action From\n53/udp ALLOW IN Anywhere\n8080 ALLOW IN Anywhere\n"}
	db := openTestDB(t)
	rt := NewRouter(discardLogger(), testBrand(), db, WithHostFirewallRunner(f.run, f.lookPath))
	cookie := loginTestSession(t, rt, db)
	if err := db.SaveApp(t.Context(), store.App{ID: "dns", Name: "dns", CreatedAt: "2026-01-01T00:00:00Z", UpdatedAt: "2026-01-01T00:00:00Z"}); err != nil {
		t.Fatal(err)
	}
	pinned := 8080
	for _, svc := range []store.DesiredService{
		{Name: "dns-server", AppID: "dns", Image: "img", Port: 8080, HostPort: &pinned, BindAddress: bindaddr.Public, PublishedPorts: []store.PublishedPort{
			{ContainerPort: 53, HostPort: 53, Protocol: "udp"}, {ContainerPort: 51820, HostPort: 51820, Protocol: "udp"},
		}},
		{Name: "local", Image: "img", Port: 80, PublishedPorts: []store.PublishedPort{{ContainerPort: 5353, HostPort: 5353, Protocol: "udp"}}},
	} {
		if err := db.SaveDesiredService(t.Context(), svc); err != nil {
			t.Fatalf("save %s: %v", svc.Name, err)
		}
	}
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/firewall/host", ""))
	var res hostFirewallResource
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil || rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	want := []hostFirewallPublishedPort{
		{Port: 8080, Protocol: "tcp", App: "dns", Service: "dns-server", BindAddress: bindaddr.Public, Allowed: true},
		{Port: 53, Protocol: "udp", App: "dns", Service: "dns-server", BindAddress: bindaddr.Public, Allowed: true},
		{Port: 51820, Protocol: "udp", App: "dns", Service: "dns-server", BindAddress: bindaddr.Public, Allowed: false},
		{Port: 5353, Protocol: "udp", App: "local", Service: "local", BindAddress: bindaddr.Private, Allowed: false},
	}
	for _, w := range want {
		if !slices.Contains(res.PublishedPorts, w) {
			t.Errorf("published_ports = %+v, missing %+v", res.PublishedPorts, w)
		}
	}
	if !slices.Contains(res.Required, hostFirewallPort{Port: 51820, Protocol: "udp"}) || slices.Contains(res.Required, hostFirewallPort{Port: 5353, Protocol: "udp"}) {
		t.Errorf("required = %+v, want 51820/udp (public) and not 5353/udp (loopback)", res.Required)
	}
}

func TestUFWAllowed(t *testing.T) {
	text := "22/tcp ALLOW IN Anywhere\n53 ALLOW IN Anywhere\n60000:61000/udp ALLOW IN Anywhere\n80,443/tcp ALLOW IN Anywhere\n9000/udp DENY IN Anywhere\n"
	cases := []struct {
		port  int
		proto string
		want  bool
	}{
		{22, "tcp", true}, {22, "udp", false}, {53, "udp", true}, {53, "tcp", true},
		{60500, "udp", true}, {60500, "tcp", false}, {443, "tcp", true}, {9000, "udp", false},
	}
	for _, c := range cases {
		if got := ufwAllowed(text, c.port, c.proto); got != c.want {
			t.Errorf("ufwAllowed(%d/%s) = %v, want %v", c.port, c.proto, got, c.want)
		}
	}
}
