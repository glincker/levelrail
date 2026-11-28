package api

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/glincker/levelrail/internal/ingresstrust"
)

// serveThroughHop runs r through the hop middleware and returns what the inner handler saw.
func serveThroughHop(r *http.Request) (https bool, trusted bool, seen http.Header) {
	IngressHopMiddleware(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		https, trusted, seen = requestIsHTTPS(r), peerIsTrustedProxy(r), r.Header.Clone()
	})).ServeHTTP(httptest.NewRecorder(), r)
	return
}

func hopRequest(remote, marker string) *http.Request {
	r := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", nil)
	r.RemoteAddr = remote
	r.Header.Set("X-Forwarded-Proto", "https")
	r.Header.Set("X-Forwarded-Host", "server.example.com")
	if marker != "" {
		r.Header.Set(ingresstrust.HeaderName, marker)
	}
	return r
}

func TestIngressHop_VerifiedMarkerIsTrustedFromNATedPeer(t *testing.T) {
	t.Setenv(envTrustedProxies, "")
	https, trusted, seen := serveThroughHop(hopRequest("72.61.8.70:56632", ingresstrust.Secret()))
	if !https || !trusted {
		t.Fatalf("ingress-originated request: https=%v trusted=%v, want both true", https, trusted)
	}
	if seen.Get(ingresstrust.HeaderName) != "" {
		t.Error("marker header must be stripped before handlers run")
	}
	if seen.Get("X-Forwarded-Host") != "server.example.com" {
		t.Error("verified request must keep X-Forwarded-Host")
	}
}

func TestIngressHop_ForgedOrMissingMarkerFromRemotePeerRefused(t *testing.T) {
	t.Setenv(envTrustedProxies, "")
	for name, marker := range map[string]string{"wrong": "guess", "missing": "", "wrong same length": strings.Repeat("a", len(ingresstrust.Secret()))} {
		t.Run(name, func(t *testing.T) {
			https, trusted, seen := serveThroughHop(hopRequest("203.0.113.9:4000", marker))
			if https || trusted {
				t.Errorf("remote peer: https=%v trusted=%v, want both false", https, trusted)
			}
			for _, h := range []string{ingresstrust.HeaderName, "X-Forwarded-Proto", "X-Forwarded-Host"} {
				if seen.Get(h) != "" {
					t.Errorf("%s must be stripped from an untrusted request", h)
				}
			}
		})
	}
}

func TestIngressHop_DuplicateMarkerIsInvalid(t *testing.T) {
	t.Setenv(envTrustedProxies, "")
	r := hopRequest("203.0.113.9:4000", "")
	r.Header.Add(ingresstrust.HeaderName, ingresstrust.Secret())
	r.Header.Add(ingresstrust.HeaderName, "other")
	if https, _, _ := serveThroughHop(r); https {
		t.Error("two marker values must not verify")
	}
}

func TestIngressHop_TrustedProxyEnvStillWorks(t *testing.T) {
	t.Setenv(envTrustedProxies, "10.0.0.1/32")
	https, trusted, seen := serveThroughHop(hopRequest("10.0.0.1:4000", ""))
	if !https || !trusted || seen.Get("X-Forwarded-Host") == "" {
		t.Errorf("env trusted proxy: https=%v trusted=%v hdr=%v", https, trusted, seen)
	}
}

func TestIngressHop_LoopbackUnchanged(t *testing.T) {
	t.Setenv(envTrustedProxies, "")
	https, trusted, _ := serveThroughHop(hopRequest("127.0.0.1:4000", ""))
	if !https || !trusted {
		t.Errorf("loopback: https=%v trusted=%v, want both true", https, trusted)
	}
}

func TestIngressHop_ClientIPFromVerifiedHop(t *testing.T) {
	t.Setenv(envTrustedProxies, "")
	r := hopRequest("72.61.8.70:1", ingresstrust.Secret())
	r.Header.Set("X-Forwarded-For", "198.51.100.7")
	var got string
	IngressHopMiddleware(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) { got = clientIP(r) })).ServeHTTP(httptest.NewRecorder(), r)
	if got != "198.51.100.7" {
		t.Errorf("clientIP = %q, want the forwarded client", got)
	}
}

func TestRefuseInsecureLogin_LogsReasonRateLimited(t *testing.T) {
	t.Setenv(envInsecureLoginLogInterval, "1h")
	insecureLoginLog.mu.Lock()
	insecureLoginLog.last = time.Time{}
	insecureLoginLog.mu.Unlock()
	var buf bytes.Buffer
	rt := &Router{logger: slog.New(slog.NewTextHandler(&buf, nil))}
	r := hopRequest("203.0.113.9:4000", "")
	for range 3 {
		IngressHopMiddleware(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) { rt.logInsecureLoginRefusal(r) })).ServeHTTP(httptest.NewRecorder(), r)
	}
	out := buf.String()
	if n := strings.Count(out, "refused sign-in"); n != 1 {
		t.Fatalf("logged %d times, want 1 (rate limited)", n)
	}
	for _, want := range []string{"203.0.113.9:4000", "forwarded_proto=https", "reason="} {
		if !strings.Contains(out, want) {
			t.Errorf("log missing %q: %s", want, out)
		}
	}
	if strings.Contains(out, ingresstrust.Secret()) {
		t.Error("log leaked the marker secret")
	}
}

func TestIngressPeerCheck(t *testing.T) {
	none := func(string) bool { return false }
	tests := []struct {
		name, bind, peer string
		trusted          func(string) bool
		want             string
	}{
		{"loopback bind", "127.0.0.1:8080", "", none, doctorStatusOK},
		{"wildcard bind", ":8080", "", none, doctorStatusOK},
		{"no observation", "10.0.0.1:18080", "", none, doctorStatusOK},
		{"rewritten source", "10.0.0.1:18080", "72.61.8.70", none, doctorStatusWarn},
		{"bind address untrusted", "10.0.0.1:18080", "10.0.0.1", none, doctorStatusWarn},
		{"bind address trusted", "10.0.0.1:18080", "10.0.0.1", func(string) bool { return true }, doctorStatusOK},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ingressPeerCheck(tt.bind, tt.peer, tt.trusted).Status; got != tt.want {
				t.Errorf("status = %s, want %s", got, tt.want)
			}
		})
	}
}

func policyAddrThroughHop(r *http.Request) (addr, clientIPv string) {
	IngressHopMiddleware(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		addr, clientIPv = policyClientAddr(r).String(), clientIP(r)
	})).ServeHTTP(httptest.NewRecorder(), r)
	return
}

func TestIngressHop_ExternalTrustedProxyDoesNotLetLeftmostForwardedForWin(t *testing.T) {
	// 203.0.113.9 is an external proxy (Cloudflare style), 198.51.100.7 the real client,
	// 192.0.2.99 an address the caller invented to hit an allowlist.
	t.Setenv(envTrustedProxies, "203.0.113.9")
	r := hopRequest("172.18.0.1:4000", ingresstrust.Secret())
	r.Header.Set("X-Forwarded-For", "192.0.2.99, 198.51.100.7, 203.0.113.9")
	addr, ip := policyAddrThroughHop(r)
	if addr != "198.51.100.7" || ip != "198.51.100.7" {
		t.Errorf("policyClientAddr=%s clientIP=%s, want the real client, never the spoofed leftmost", addr, ip)
	}
}

func TestIngressHop_SpoofedEntryBehindMarkedHopIsNotReturned(t *testing.T) {
	t.Setenv(envTrustedProxies, "")
	r := hopRequest("172.18.0.1:4000", ingresstrust.Secret())
	r.Header.Set("X-Forwarded-For", "192.0.2.99, 198.51.100.7")
	addr, ip := policyAddrThroughHop(r)
	if addr != "198.51.100.7" || ip != "198.51.100.7" {
		t.Errorf("policyClientAddr=%s clientIP=%s, want the entry the ingress appended", addr, ip)
	}
}

func TestIngressHop_SingleHopReturnsRealClient(t *testing.T) {
	t.Setenv(envTrustedProxies, "")
	r := hopRequest("172.18.0.1:4000", ingresstrust.Secret())
	r.Header.Set("X-Forwarded-For", "198.51.100.7")
	addr, ip := policyAddrThroughHop(r)
	if addr != "198.51.100.7" || ip != "198.51.100.7" {
		t.Errorf("policyClientAddr=%s clientIP=%s, want 198.51.100.7", addr, ip)
	}
}

func TestIngressHop_ForgedMarkerIgnoredForClientAddr(t *testing.T) {
	t.Setenv(envTrustedProxies, "")
	r := hopRequest("172.18.0.1:4000", "forged")
	r.Header.Set("X-Forwarded-For", "192.0.2.99")
	addr, ip := policyAddrThroughHop(r)
	if addr != "172.18.0.1" || ip != "172.18.0.1" {
		t.Errorf("policyClientAddr=%s clientIP=%s, want the TCP peer", addr, ip)
	}
}

func TestIngressHop_TrustedProxyListAndLoopbackClientAddrUnchanged(t *testing.T) {
	t.Setenv(envTrustedProxies, "203.0.113.9")
	r := hopRequest("203.0.113.9:4000", "")
	r.Header.Set("X-Forwarded-For", "192.0.2.99, 198.51.100.7")
	if addr, ip := policyAddrThroughHop(r); addr != "198.51.100.7" || ip != "198.51.100.7" {
		t.Errorf("listed proxy: policyClientAddr=%s clientIP=%s", addr, ip)
	}
	r = hopRequest("127.0.0.1:4000", "")
	r.Header.Set("X-Forwarded-For", "198.51.100.7")
	if addr, ip := policyAddrThroughHop(r); addr != "198.51.100.7" || ip != "198.51.100.7" {
		t.Errorf("loopback: policyClientAddr=%s clientIP=%s", addr, ip)
	}
}
