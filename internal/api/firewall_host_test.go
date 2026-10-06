package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

type fakeUFW struct {
	mu        sync.Mutex
	calls     []string
	active    bool
	failOn    string
	installed bool
}

func (f *fakeUFW) run(_ context.Context, name string, args ...string) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	call := name + " " + strings.Join(args, " ")
	if len(args) > 0 && args[0] == "status" {
		if f.active {
			return []byte("Status: active\nDefault: deny (incoming), allow (outgoing), disabled (routed)\n"), nil
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
	for _, p := range rt.hostFirewallRequired() {
		if p.Port < 1 || p.Port > 65535 {
			t.Errorf("required ports include %d, want only valid ports", p.Port)
		}
	}
}
