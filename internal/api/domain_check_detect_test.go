package api

import (
	"context"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestDomainCheckAcceptsEitherAddressFamily(t *testing.T) {
	rt, _ := newTestRouter(t)
	rt.publicHost = "2a02:4780:2d:55d7::1"
	rt.detectPublicIPs = func(context.Context) []string { return []string{"72.61.8.70", "2a02:4780:2d:55d7::1"} }
	rt.lookupHost = func(context.Context, string) ([]string, error) { return []string{"72.61.8.70"}, nil }

	got := rt.runDomainCheck(context.Background(), "server.example.com", rt.publicHost, false)
	if got.Status != domainCheckStatusConnected {
		t.Errorf("status = %q, want connected: an A record to this host's IPv4 is this server", got.Status)
	}
	if len(got.ExpectedIPv4) == 0 || len(got.ExpectedIPv6) == 0 {
		t.Errorf("expected addresses = %v / %v, want both families so the UI offers A and AAAA", got.ExpectedIPv4, got.ExpectedIPv6)
	}
}

func TestClassifyACMEErrorOtherProxy(t *testing.T) {
	msg := "authorization failed: HTTP 403 urn:ietf:params:acme:error:unauthorized - Invalid response from http://server.glinr.com/.well-known/acme-challenge/x: 404"
	if got := classifyACMEError(msg); got != httpsHintOtherProxy {
		t.Errorf("classifyACMEError() = %q, want %q", got, httpsHintOtherProxy)
	}
	if got := acmeActionForReason(httpsHintOtherProxy); got != acmeActionOtherProxy {
		t.Errorf("action = %q", got)
	}
}

func TestClassifyACMEErrorTLSALPNOtherProxy(t *testing.T) {
	msg := "authorization failed: HTTP 400 urn:ietf:params:acme:error:tls - 72.61.8.70: remote error: tls: unrecognized name"
	if got := classifyACMEError(msg); got != httpsHintOtherProxy {
		t.Errorf("classifyACMEError() = %q, want %q", got, httpsHintOtherProxy)
	}
}

func TestIngressSettingsACMEBlockedOnNonStandardPorts(t *testing.T) {
	rt, _ := newTestRouter(t)
	rt.doctorHTTPPort, rt.doctorHTTPSPort = 8088, 8443
	res := rt.toIngressSettingsResource(store.IngressSettings{ACMEEnabled: true})
	if res.ACMEBlocked != acmeBlockedNonStandardPorts || res.IngressHTTPSPort != 8443 {
		t.Errorf("resource = %+v, want blocked on non standard ports", res)
	}
	if res := rt.toIngressSettingsResource(store.IngressSettings{ACMEEnabled: false}); res.ACMEBlocked != "" {
		t.Errorf("acme off must not report a block, got %q", res.ACMEBlocked)
	}
	rt.doctorHTTPPort, rt.doctorHTTPSPort = 0, 0
	if res := rt.toIngressSettingsResource(store.IngressSettings{ACMEEnabled: true}); res.ACMEBlocked != "" {
		t.Errorf("standard ports must not report a block, got %q", res.ACMEBlocked)
	}
}

func TestIngressSettingsNoACMEBlockedBehindTLSUpstream(t *testing.T) {
	rt, _ := newTestRouter(t)
	rt.doctorHTTPPort, rt.doctorHTTPSPort = 8088, 8443
	res := rt.toIngressSettingsResource(store.IngressSettings{ACMEEnabled: true, TLSTerminatedUpstream: true})
	if res.ACMEBlocked != "" {
		t.Errorf("a proxy that terminates TLS must not report an ACME block, got %q", res.ACMEBlocked)
	}
}
