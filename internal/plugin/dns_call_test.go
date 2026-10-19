package plugin

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/plugin/pluginpb"
)

func dnsManifest(abilities string) []byte {
	return []byte("api_version: 1\nname: example\nversion: 1.0.0\nkind: dns_provider\npublisher: Example Org\n" +
		"image: registry.example.com/team/example@" + testDigest + "\nabilities: " + abilities + "\noutbound_hosts: [api.cloudflare.com]\n")
}

func dnsHarness(t *testing.T, approve []string) *harness {
	t.Helper()
	h := newHarness(t)
	ctx := context.Background()
	if _, err := h.m.Install(ctx, h.request(dnsManifest("[read]")), "admin"); err != nil {
		t.Fatal(err)
	}
	if _, err := h.m.Approve(ctx, "example", approve, "admin"); err != nil {
		t.Fatal(err)
	}
	if _, err := h.m.Enable(ctx, "example", "admin"); err != nil {
		t.Fatal(err)
	}
	h.rpc.describe = &pluginpb.DescribeResponse{ContractVersion: 1, Name: "example", Version: "1.0.0", Kind: KindDNSProvider, RequestedAbilities: []string{AbilityRead}}
	h.m.Reconcile(ctx)
	if h.m.inst("example").connected() == nil {
		t.Fatal("plugin did not start")
	}
	return h
}

func TestDNSManifestKindNeedsRead(t *testing.T) {
	if _, err := ParseManifest(dnsManifest("[]")); err == nil || !strings.Contains(err.Error(), "read ability") {
		t.Fatalf("err = %v", err)
	}
	if _, err := ParseManifest(dnsManifest("[read, root]")); err == nil {
		t.Fatal("root must never be grantable to a dns_provider")
	}
	if _, err := ParseManifest(dnsManifest("[read]")); err != nil {
		t.Fatal(err)
	}
}

func TestDNSCallsCarryPerCallSecretAndAreAudited(t *testing.T) {
	h := dnsHarness(t, []string{AbilityRead})
	h.rpc.dns.zones = []string{"example.com."}
	ctx := context.Background()
	zones, err := h.m.DNSListZones(ctx, "example", "tok-SECRET")
	if err != nil || len(zones) != 1 {
		t.Fatalf("zones = %v, %v", zones, err)
	}
	rec := DNSRecord{Name: "app", Type: "A", Value: "203.0.113.5", TTL: time.Second}
	if err := h.m.DNSUpsert(ctx, "example", "tok-SECRET", "example.com.", rec); err != nil {
		t.Fatal(err)
	}
	h.rpc.dns.present = true
	if ok, err := h.m.DNSVerify(ctx, "example", "tok-SECRET", "example.com.", rec); err != nil || !ok {
		t.Fatalf("verify = %v, %v", ok, err)
	}
	if err := h.m.DNSDelete(ctx, "example", "tok-SECRET", "example.com.", rec); err != nil {
		t.Fatal(err)
	}
	for _, s := range h.rpc.dns.secrets {
		if s != "tok-SECRET" {
			t.Fatalf("secret not handed to every call: %v", h.rpc.dns.secrets)
		}
	}
	stored, _ := h.store.GetPlugin(ctx, "example")
	if strings.Contains(stored.ConfigJSON+stored.Manifest+stored.LastError, "tok-SECRET") {
		t.Fatal("the per-call secret was persisted")
	}
	used := 0
	for _, e := range h.audit.events {
		for _, v := range e.Detail {
			if strings.Contains(v, "tok-SECRET") {
				t.Fatal("the per-call secret reached the audit log")
			}
		}
		if e.Action == AuditAbilityUsed {
			used++
		}
	}
	if used != 4 {
		t.Fatalf("audited ability uses = %d, want 4", used)
	}
}

func TestDNSCallRefusals(t *testing.T) {
	ctx := context.Background()
	rec := DNSRecord{Name: "app", Type: "A", Value: "203.0.113.5"}

	h := newHarness(t)
	if _, err := h.m.Install(ctx, h.request(dnsManifest("[read]")), "admin"); err != nil {
		t.Fatal(err)
	}
	if err := h.m.DNSUpsert(ctx, "example", "t", "example.com.", rec); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("installed but not enabled plugin: %v", err)
	}

	n := runningHarness(t, []string{AbilityRead})
	if _, err := n.m.DNSListZones(ctx, "example", "t"); !errors.Is(err, ErrNotDNSProvider) {
		t.Fatalf("notification channel must not serve DNS: %v", err)
	}

	d := dnsHarness(t, []string{AbilityRead})
	if _, err := d.m.DNSListZones(ctx, "example", strings.Repeat("x", d.m.cfg.MaxSecretBytes+1)); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("oversized secret: %v", err)
	}
	if _, err := d.m.Disable(ctx, "example", "admin"); err != nil {
		t.Fatal(err)
	}
	if _, err := d.m.DNSListZones(ctx, "example", "t"); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("disabled plugin: %v", err)
	}
}

func TestDNSPluginFailureIsRedactedAndTyped(t *testing.T) {
	h := dnsHarness(t, []string{AbilityRead})
	h.rpc.dns.outcome = &pluginpb.DnsOutcome{Ok: false, Retryable: true, Detail: "bad token tok-SECRET at upstream"}
	err := h.m.DNSUpsert(context.Background(), "example", "tok-SECRET", "example.com.", DNSRecord{Name: "a", Type: "A", Value: "203.0.113.5"})
	var de *DNSError
	if !errors.As(err, &de) || !de.Retryable || strings.Contains(err.Error(), "tok-SECRET") {
		t.Fatalf("err = %v", err)
	}

	h.rpc.dns.outcome = nil
	h.rpc.dns.err = errors.New("transport broke tok-SECRET")
	_, err = h.m.DNSListRecords(context.Background(), "example", "tok-SECRET", "example.com.")
	if err == nil || strings.Contains(err.Error(), "tok-SECRET") {
		t.Fatalf("transport error leaked the secret: %v", err)
	}
}

func TestDNSBreakerOpensAfterRepeatedTransportFailures(t *testing.T) {
	h := dnsHarness(t, []string{AbilityRead})
	h.rpc.dns.err = errBoom
	for range 2 {
		_, _ = h.m.DNSListZones(context.Background(), "example", "t")
	}
	if _, err := h.m.DNSListZones(context.Background(), "example", "t"); !errors.Is(err, ErrCircuitOpen) {
		t.Fatalf("err = %v", err)
	}
}
