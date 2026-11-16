package provision

import (
	"context"
	"net/http"
	"testing"
	"time"
)

func capByID(t *testing.T, pc ProviderCapabilities, id string) Capability {
	t.Helper()
	for _, c := range pc.Capabilities {
		if c.ID == id {
			return c
		}
	}
	t.Fatalf("capability %q missing", id)
	return Capability{}
}

func TestDigitalOceanProbe_AllReadsGranted(t *testing.T) {
	d := newTestDigitalOcean(t, func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("{}")) })
	pc := d.ProbeCapabilities(context.Background(), 4)
	if c := capByID(t, pc, "billing.balance"); c.State != CapabilityGranted || c.Evidence != EvidenceProbeOK {
		t.Errorf("balance = %+v", c)
	}
	if c := capByID(t, pc, "instances.create"); c.State != CapabilityUnknown || c.Evidence != EvidenceNotProbed {
		t.Errorf("write must stay unknown: %+v", c)
	}
	if pc.LiveCheckError != "" {
		t.Errorf("live error = %q", pc.LiveCheckError)
	}
}

func TestDigitalOceanProbe_ScopedTokenMissesAccountAndBilling(t *testing.T) {
	d := newTestDigitalOcean(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/account" || r.URL.Path == "/customers/my/balance" {
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"id":"forbidden","message":"secret provider body"}`))
			return
		}
		_, _ = w.Write([]byte("{}"))
	})
	pc := d.ProbeCapabilities(context.Background(), 2)
	for _, id := range []string{"account.read", "billing.balance"} {
		c := capByID(t, pc, id)
		if c.State != CapabilityMissing || c.Evidence != EvidenceProbe403 || c.Needs == "" {
			t.Errorf("%s = %+v", id, c)
		}
	}
	if c := capByID(t, pc, "instances.read"); c.State != CapabilityGranted {
		t.Errorf("droplets = %+v", c)
	}
}

func TestDigitalOceanProbe_EverythingFails(t *testing.T) {
	d := newTestDigitalOcean(t, func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusUnauthorized) })
	pc := d.ProbeCapabilities(context.Background(), 4)
	if c := capByID(t, pc, "regions.read"); c.State != CapabilityMissing {
		t.Errorf("regions = %+v", c)
	}
}

func TestDigitalOceanProbe_ServerErrorIsUnknownNotMissing(t *testing.T) {
	d := newTestDigitalOcean(t, func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusInternalServerError) })
	d.client.sleep = func(context.Context, time.Duration) error { return nil }
	pc := d.ProbeCapabilities(context.Background(), 4)
	c := capByID(t, pc, "regions.read")
	if c.State != CapabilityUnknown || pc.LiveCheckError == "" {
		t.Errorf("regions = %+v err=%q", c, pc.LiveCheckError)
	}
}

func TestScopeFromError(t *testing.T) {
	se, ok := ScopeFromError("digitalocean", "billing.balance", &ProviderError{Status: 403, Body: "x"})
	if !ok || se.Needs != "billing:read" {
		t.Fatalf("got %+v %v", se, ok)
	}
	if _, ok := ScopeFromError("digitalocean", "billing.balance", &ProviderError{Status: 500}); ok {
		t.Error("500 must not map to a scope")
	}
}
