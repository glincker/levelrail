package api

import (
	"context"
	"testing"
)

func TestProbeDialerRefusesInternalTenantTarget(t *testing.T) {
	d := probeDialer("169.254.169.254:443")
	if _, err := d.DialContext(context.Background(), "tcp", "169.254.169.254:80"); err == nil {
		t.Fatal("dial to metadata address succeeded, want refusal")
	}
}

func TestProbeDialAddrIPLiteralDomainIsNotOwnIngress(t *testing.T) {
	rt := &Router{}
	addr := rt.probeDialAddr("127.0.0.1", true, 0)
	d := probeDialer(addr)
	if _, err := d.DialContext(context.Background(), "tcp", addr); err == nil {
		t.Fatal("IP-literal tenant domain reached the loopback exemption")
	}
}
