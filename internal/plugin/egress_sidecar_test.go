package plugin

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/plugin/egress"
)

func proxyHarness(t *testing.T) (*harness, *[]egress.Policy) {
	t.Helper()
	h := newHarness(t)
	h.m.cfg.EgressMode = EgressProxy
	h.m.cfg.EgressImage = "registry.example.com/levelrail:v1"
	var pushed []egress.Policy
	h.m.pushFn = func(_ context.Context, _ string, p egress.Policy) error {
		pushed = append(pushed, p)
		return nil
	}
	return h, &pushed
}

func hostRecord() Record {
	return Record{Name: "example", Kind: KindNotificationChannel, RequestedAbilities: []string{AbilityRead}, Image: "registry.example.com/team/example@" + testDigest, OutboundHosts: []string{"hooks.example.com"}}
}

func specNamed(h *harness, name string) (found bool, env map[string]string) {
	for _, s := range h.dk.specs {
		if s.Name == name {
			return true, s.Env
		}
	}
	return false, nil
}

func TestProxyModeStartsSidecarAndInjectsProxyEnv(t *testing.T) {
	h, pushed := proxyHarness(t)
	rec := hostRecord()
	if err := h.m.start(context.Background(), rec, h.m.inst(rec.Name)); err != nil {
		t.Fatal(err)
	}
	if ok, _ := specNamed(h, "ns-egress-proxy"); !ok {
		t.Fatal("sidecar was not created")
	}
	_, env := specNamed(h, "ns-plugin-example")
	if !strings.HasPrefix(env["HTTPS_PROXY"], "http://example:") || !strings.HasSuffix(env["HTTPS_PROXY"], "@ns-egress-proxy:3128") {
		t.Fatalf("HTTPS_PROXY = %q", env["HTTPS_PROXY"])
	}
	if len(*pushed) == 0 {
		t.Fatal("no policy pushed")
	}
	last := (*pushed)[len(*pushed)-1]
	if len(last.Plugins) != 1 || last.Plugins[0].Name != "example" || last.Plugins[0].Hosts[0] != "hooks.example.com" {
		t.Fatalf("policy = %+v", last)
	}
	if strings.Contains(env["HTTPS_PROXY"], last.Plugins[0].SecretHash) {
		t.Fatal("policy leaks into env or the reverse")
	}
	if len(h.dk.isolated) == 0 || !h.dk.isolated[0].Internal {
		t.Fatalf("plugin network is not internal: %+v", h.dk.isolated)
	}
	if len(h.dk.connects) == 0 || !strings.HasPrefix(h.dk.connects[0], "ns-plugins:") {
		t.Fatalf("sidecar not attached to the plugin network: %v", h.dk.connects)
	}
}

func TestProxyModeFailsClosed(t *testing.T) {
	t.Run("push fails", func(t *testing.T) {
		h, _ := proxyHarness(t)
		h.m.pushFn = func(context.Context, string, egress.Policy) error { return errors.New("down") }
		h.m.cfg.StartTimeout = -1
		rec := hostRecord()
		err := h.m.start(context.Background(), rec, h.m.inst(rec.Name))
		if !errors.Is(err, ErrEgressUnavailable) {
			t.Fatalf("err = %v", err)
		}
		if ok, _ := specNamed(h, "ns-plugin-example"); ok {
			t.Fatal("plugin container created without a working proxy")
		}
	})
	t.Run("no image", func(t *testing.T) {
		h, _ := proxyHarness(t)
		h.m.cfg.EgressImage = ""
		rec := hostRecord()
		err := h.m.start(context.Background(), rec, h.m.inst(rec.Name))
		if !errors.Is(err, ErrEgressUnavailable) {
			t.Fatalf("err = %v", err)
		}
		if ok, _ := specNamed(h, "ns-plugin-example"); ok {
			t.Fatal("plugin container created without a proxy image")
		}
	})
}

func TestOpenModeKeepsBridgeAndTurnsICCOff(t *testing.T) {
	h := newHarness(t)
	rec := hostRecord()
	if err := h.m.start(context.Background(), rec, h.m.inst(rec.Name)); err != nil {
		t.Fatal(err)
	}
	if ok, _ := specNamed(h, "ns-egress-proxy"); ok {
		t.Fatal("sidecar created in open mode")
	}
	_, env := specNamed(h, "ns-plugin-example")
	if _, has := env["HTTPS_PROXY"]; has {
		t.Fatal("proxy env set in open mode")
	}
	if len(h.dk.isolated) == 0 || h.dk.isolated[0].Internal || !h.dk.isolated[0].DisableICC || h.dk.isolated[0].Recreate {
		t.Fatalf("open mode network options = %+v", h.dk.isolated)
	}
}

func TestEgressModeDefaultsToOpen(t *testing.T) {
	get := func(map[string]string) func(string) (string, bool) {
		return func(string) (string, bool) { return "", false }
	}
	if c := ConfigFromEnv("ns", "/d", get(nil)); c.EgressMode != "" {
		t.Fatalf("mode = %q", c.EgressMode)
	}
	on := func(k string) (string, bool) {
		if k == EnvEgressMode {
			return " Proxy ", true
		}
		return "", false
	}
	if c := ConfigFromEnv("ns", "/d", on); c.EgressMode != EgressProxy {
		t.Fatalf("mode = %q", c.EgressMode)
	}
}
