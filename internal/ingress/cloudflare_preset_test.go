package ingress

import (
	"slices"
	"testing"
)

func TestWithCloudflarePresetTrustsOnlyListedPeers(t *testing.T) {
	h := DefaultHardening().WithCloudflarePreset([]string{"173.245.48.0/20"})
	if !slices.Equal(h.TrustedProxies, []string{"173.245.48.0/20"}) {
		t.Fatalf("trusted = %v", h.TrustedProxies)
	}
	if !slices.Equal(h.ClientIPHeaders, []string{HeaderCFConnectingIP}) {
		t.Fatalf("headers = %v", h.ClientIPHeaders)
	}
	var s Server
	h.applyServer(&s)
	if s.TrustedProxies == nil || s.TrustedProxiesStrict != 1 {
		t.Fatalf("server must trust strictly: %+v", s)
	}
}

func TestWithCloudflarePresetKeepsOperatorProxies(t *testing.T) {
	base := DefaultHardening()
	base.TrustedProxies = []string{"10.0.0.0/8"}
	h := base.WithCloudflarePreset([]string{"173.245.48.0/20"})
	if !slices.Contains(h.TrustedProxies, "10.0.0.0/8") || !slices.Contains(h.TrustedProxies, "173.245.48.0/20") {
		t.Fatalf("trusted = %v", h.TrustedProxies)
	}
	if !slices.Equal(h.ClientIPHeaders, []string{HeaderCFConnectingIP, "X-Forwarded-For"}) {
		t.Fatalf("headers = %v", h.ClientIPHeaders)
	}
	if len(base.TrustedProxies) != 1 {
		t.Fatal("the input hardening must not be mutated")
	}
}
