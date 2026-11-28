package api

import (
	"testing"

	ingressdriver "github.com/glincker/levelrail/internal/ingress"
)

func TestSetResolvedTrustedProxiesReachesIngressReaders(t *testing.T) {
	t.Cleanup(func() { SetResolvedTrustedProxies(nil) })
	SetResolvedTrustedProxies([]string{"10.0.0.0/8"})
	if len(ingressdriver.TrustedProxyPrefixes()) != 1 {
		t.Fatal("ingress readers did not receive the resolved list")
	}
	SetResolvedTrustedProxies(nil)
	if len(ingressdriver.TrustedProxyPrefixes()) != 0 {
		t.Fatal("clearing did not reach ingress readers")
	}
}
