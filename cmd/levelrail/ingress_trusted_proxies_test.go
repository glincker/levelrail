package main

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/netip"
	"testing"

	"github.com/GLINCKER/levelrail/internal/api"
	ingressdriver "github.com/GLINCKER/levelrail/internal/ingress"
	"github.com/GLINCKER/levelrail/internal/runtimesettings"
	"github.com/GLINCKER/levelrail/internal/store"
)

type fakeEdgeStore struct{ s store.IngressEdgeSettings }

func (f fakeEdgeStore) GetIngressEdgeSettings(context.Context) (store.IngressEdgeSettings, error) {
	return f.s, nil
}

func TestTrustedProxyListParityBetweenAPIAndEdge(t *testing.T) {
	t.Setenv(ingressdriver.EnvTrustedProxies, "")
	edge := ingressEdge{hardening: &ingressdriver.Hardening{}}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	db := fakeEdgeStore{s: store.IngressEdgeSettings{TrustedProxies: "private_ranges"}}

	h := edge.layeredHardening(context.Background(), db, logger)
	h = withTrustedProxyPreset(h, runtimesettings.TrustedProxyPresetCloudflare, []string{"203.0.113.0/24"})
	api.SetResolvedTrustedProxies(h.TrustedProxies)

	var want []netip.Prefix
	for _, e := range h.TrustedProxies {
		p, err := netip.ParsePrefix(e)
		if err != nil {
			a, aerr := netip.ParseAddr(e)
			if aerr != nil {
				t.Fatalf("edge entry %q does not parse", e)
			}
			p = netip.PrefixFrom(a, a.BitLen())
		}
		want = append(want, p)
	}
	got := api.ResolvedTrustedProxies()
	if len(got) != len(want) {
		t.Fatalf("api trusts %d ranges, edge %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("range %d: api %s, edge %s", i, got[i], want[i])
		}
	}

	r := &http.Request{RemoteAddr: "203.0.113.9:443", Header: http.Header{"X-Forwarded-For": {"198.51.100.7"}}}
	if !api.PeerIsTrustedProxy(r) {
		t.Fatal("a preset peer must be trusted by the api")
	}
}

type mutableEdgeStore struct{ s store.IngressEdgeSettings }

func (m *mutableEdgeStore) GetIngressEdgeSettings(context.Context) (store.IngressEdgeSettings, error) {
	return m.s, nil
}

func TestTrustedProxySettingOnlyReachesAPIAndDoctor(t *testing.T) {
	t.Setenv(ingressdriver.EnvTrustedProxies, "")
	t.Cleanup(func() { api.SetResolvedTrustedProxies(nil) })
	edge := ingressEdge{hardening: &ingressdriver.Hardening{}}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	db := &mutableEdgeStore{}
	live := edge.liveTrustedProxies(db, logger, func() *runtimesettings.Service { return nil })
	peer := &http.Request{RemoteAddr: "203.0.113.9:443"}

	if live(context.Background()) != 0 || api.PeerIsTrustedProxy(peer) {
		t.Fatal("nothing configured: no trusted proxies expected")
	}
	db.s.TrustedProxies = "203.0.113.9"
	if n := live(context.Background()); n != 1 {
		t.Fatalf("doctor count after a saved setting = %d, want 1", n)
	}
	if !api.PeerIsTrustedProxy(peer) {
		t.Fatal("the saved peer must be trusted without a restart")
	}
	db.s.TrustedProxies = "private_ranges"
	if live(context.Background()) < 2 || api.PeerIsTrustedProxy(peer) {
		t.Fatal("a live change must replace the previous list")
	}
	if !api.PeerIsTrustedProxy(&http.Request{RemoteAddr: "10.1.2.3:1"}) {
		t.Fatal("private_ranges must trust a private peer")
	}
}
