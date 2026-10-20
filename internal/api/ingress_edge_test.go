package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestDetectUpstreamProxy(t *testing.T) {
	tests := []struct {
		name      string
		remote    string
		headers   map[string]string
		wantProxy string
		wantSug   string
		wantRot   bool
	}{
		{"direct", "203.0.113.9:5000", nil, proxyKindNone, "", false},
		{"cloudflare tunnel on loopback", "127.0.0.1:5000", map[string]string{"CF-Ray": "abc"}, proxyKindCloudflare, "private_ranges", false},
		{"cloudflare public edge", "104.16.0.1:443", map[string]string{"CF-Connecting-IP": "198.51.100.1"}, proxyKindCloudflare, "", true},
		{"traefik on docker network", "172.18.0.2:4000", map[string]string{"X-Forwarded-Server": "proxy-host"}, proxyKindTraefik, "private_ranges", false},
		{"coolify traefik", "10.0.0.5:4000", map[string]string{"X-Forwarded-Server": "coolify-proxy"}, proxyKindCoolify, "private_ranges", false},
		{"generic public proxy", "198.51.100.7:4000", map[string]string{"X-Forwarded-For": "1.2.3.4"}, proxyKindGeneric, "198.51.100.7", false},
		{"v4 mapped peer", "[::ffff:192.168.1.2]:80", map[string]string{"X-Forwarded-Proto": "https"}, proxyKindGeneric, "private_ranges", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/", nil)
			r.RemoteAddr = tc.remote
			for k, v := range tc.headers {
				r.Header.Set(k, v)
			}
			got := detectUpstreamProxy(r)
			if got.Proxy != tc.wantProxy || got.Suggested != tc.wantSug || got.RangesRotate != tc.wantRot {
				t.Errorf("detect = %+v, want proxy %s suggested %q rotate %v", got, tc.wantProxy, tc.wantSug, tc.wantRot)
			}
		})
	}
}

func edgeDo(t *testing.T, rt *Router, cookie *http.Cookie, method, path, body string, hdr map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	req := authedRequest(t, cookie, method, path, body)
	req.RemoteAddr = "172.18.0.2:4000"
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	return rec
}

func TestIngressEdgeFixFlow(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	proxied := map[string]string{"X-Forwarded-Server": "proxy-host"}

	var res edgeSettingsResource
	rec := edgeDo(t, rt, cookie, http.MethodGet, "/api/v1/settings/ingress/edge", "", proxied)
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &res) != nil {
		t.Fatalf("get = %d %s", rec.Code, rec.Body)
	}
	if !res.NeedsTrustedProxies || res.Detected.Proxy != proxyKindTraefik || res.Effective.MaxBodyBytes.Source != edgeSourceDefault {
		t.Fatalf("before fix = %+v", res)
	}

	rec = edgeDo(t, rt, cookie, http.MethodPost, "/api/v1/settings/ingress/edge/trusted-proxies/apply-detected", "", proxied)
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &res) != nil {
		t.Fatalf("apply = %d %s", rec.Code, rec.Body)
	}
	if res.TrustedProxies != "private_ranges" || res.NeedsTrustedProxies || !res.TrustedProxiesActive || res.TrustedProxiesSource != edgeSourceSetting {
		t.Fatalf("after fix = %+v", res)
	}

	rec = edgeDo(t, rt, cookie, http.MethodGet, "/api/v1/settings/ingress", "", proxied)
	if rec.Code != http.StatusOK {
		t.Fatalf("ingress settings = %d", rec.Code)
	}
}

func TestIngressEdgeApplyRefusals(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	rec := edgeDo(t, rt, cookie, http.MethodPost, "/api/v1/settings/ingress/edge/trusted-proxies/apply-detected", "", nil)
	if rec.Code != http.StatusConflict {
		t.Errorf("no proxy = %d, want 409", rec.Code)
	}
	if s, _ := db.GetIngressEdgeSettings(context.Background()); s.TrustedProxies != "" {
		t.Errorf("refused apply wrote %q", s.TrustedProxies)
	}

	rt.domainPolicy.getenv = func(k string) string {
		if k == "APP_INGRESS_TRUSTED_PROXIES" {
			return "10.0.0.0/8"
		}
		return ""
	}
	rec = edgeDo(t, rt, cookie, http.MethodPost, "/api/v1/settings/ingress/edge/trusted-proxies/apply-detected", "", map[string]string{"X-Forwarded-Server": "x"})
	if rec.Code != http.StatusConflict {
		t.Errorf("env locked = %d, want 409", rec.Code)
	}
	if s, _ := db.GetIngressEdgeSettings(context.Background()); s.TrustedProxies != "" {
		t.Errorf("env locked apply wrote %q", s.TrustedProxies)
	}
}

func TestIngressEdgePutValidation(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	tests := []struct {
		name string
		body string
		want int
	}{
		{"junk", `nope`, http.StatusBadRequest},
		{"bad proxy", `{"trusted_proxies":"not-an-ip"}`, http.StatusBadRequest},
		{"negative body", `{"max_body_bytes":-1}`, http.StatusBadRequest},
		{"huge timeout", `{"idle_timeout_secs":999999999}`, http.StatusBadRequest},
		{"valid", `{"trusted_proxies":"10.0.0.0/8, 192.0.2.1","max_body_bytes":1048576,"idle_timeout_secs":30}`, http.StatusOK},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if rec := edgeDo(t, rt, cookie, http.MethodPut, "/api/v1/settings/ingress/edge", tc.body, nil); rec.Code != tc.want {
				t.Fatalf("code = %d %s, want %d", rec.Code, rec.Body, tc.want)
			}
		})
	}
	s, err := db.GetIngressEdgeSettings(context.Background())
	if err != nil || s.MaxBodyBytes == nil || *s.MaxBodyBytes != 1048576 || s.MaxHeaderBytes != nil || s.TrustedProxies != "10.0.0.0/8, 192.0.2.1" {
		t.Fatalf("stored = %+v err %v: invalid requests must not write", s, err)
	}
	var res edgeSettingsResource
	rec := edgeDo(t, rt, cookie, http.MethodGet, "/api/v1/settings/ingress/edge", "", nil)
	if json.Unmarshal(rec.Body.Bytes(), &res) != nil || res.Effective.MaxBodyBytes.Value != 1048576 || res.Effective.MaxBodyBytes.Source != edgeSourceSetting || res.Effective.IdleTimeout.Value != 30 {
		t.Fatalf("effective = %+v", res.Effective)
	}
}

func TestAppIngressLimits(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	if err := db.SaveDesiredService(context.Background(), store.DesiredService{Name: "web", Image: "nginx:1", Port: 80}); err != nil {
		t.Fatal(err)
	}
	path := "/api/v1/apps/web/ingress-limits"
	if rec := edgeDo(t, rt, cookie, http.MethodGet, "/api/v1/apps/ghost/ingress-limits", "", nil); rec.Code != http.StatusNotFound {
		t.Errorf("unknown app = %d", rec.Code)
	}
	for _, body := range []string{`nope`, `{"max_body_bytes":0}`, `{"max_body_bytes":-5}`} {
		if rec := edgeDo(t, rt, cookie, http.MethodPut, path, body, nil); rec.Code != http.StatusBadRequest {
			t.Errorf("%s = %d, want 400", body, rec.Code)
		}
	}
	if _, err := db.GetAppIngressLimit(context.Background(), "web"); err == nil {
		t.Fatal("rejected request stored an override")
	}
	var res appIngressLimitsResource
	rec := edgeDo(t, rt, cookie, http.MethodPut, path, `{"max_body_bytes":2097152}`, nil)
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &res) != nil || res.MaxBodyBytes == nil || *res.MaxBodyBytes != 2097152 || res.InstanceMaxBodyBytes == 0 {
		t.Fatalf("set = %d %s", rec.Code, rec.Body)
	}
	rec = edgeDo(t, rt, cookie, http.MethodPut, path, `{"max_body_bytes":null}`, nil)
	res = appIngressLimitsResource{}
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &res) != nil || res.MaxBodyBytes != nil {
		t.Fatalf("clear = %d %s", rec.Code, rec.Body)
	}
	if rec := edgeDo(t, rt, cookie, http.MethodPut, path, `{"max_body_bytes":null}`, nil); rec.Code != http.StatusOK {
		t.Errorf("clearing twice = %d, want 200", rec.Code)
	}
}
