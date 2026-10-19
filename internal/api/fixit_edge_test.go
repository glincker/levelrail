package api

import (
	"context"
	"net/http"
	"testing"

	"github.com/GLINCKER/levelrail/internal/fixit"
)

func TestTrustedProxiesFix(t *testing.T) {
	rt, db := newFixitRouter(t)
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()
	eng := rt.fixitEngine()
	const id = "settings.ingress.trusted_proxies"

	if v, err := eng.Preview(ctx, id); err != nil || v.State != fixit.StateHealthy {
		t.Fatalf("before any proxy: %+v %v", v, err)
	}

	edgeDo(t, rt, cookie, http.MethodGet, "/api/v1/settings/ingress/edge", "", map[string]string{"X-Forwarded-Server": "proxy-host"})
	v, err := eng.Preview(ctx, id)
	if err != nil || v.State != fixit.StateOffer || v.Kind != fixit.KindAuto {
		t.Fatalf("after a proxied read: %+v %v", v, err)
	}

	rt.fixit.observed.Store(nil)
	if got := rt.observedProxy(ctx); got == nil || got.Proxy != proxyKindTraefik {
		t.Fatalf("persisted observation lost: %+v", got)
	}

	res, err := eng.Apply(ctx, id, "tester", nil)
	if err != nil || res.Outcome != fixit.OutcomeFixed {
		t.Fatalf("apply: %+v %v", res, err)
	}
	if !rt.edgeTrustedProxiesSet(ctx) {
		t.Fatal("trusted proxies were not saved")
	}
}

func TestTrustedProxiesFixIgnoresDirectAndEnvLocked(t *testing.T) {
	rt, db := newFixitRouter(t)
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()
	edgeDo(t, rt, cookie, http.MethodGet, "/api/v1/settings/ingress/edge", "", nil)
	if rt.observedProxy(ctx) != nil {
		t.Fatal("a request without proxy headers must not record a proxy")
	}
	rt.domainPolicy.getenv = func(k string) string {
		if k == "APP_INGRESS_TRUSTED_PROXIES" {
			return "10.0.0.0/8"
		}
		return ""
	}
	edgeDo(t, rt, cookie, http.MethodGet, "/api/v1/settings/ingress/edge", "", map[string]string{"X-Forwarded-Server": "p"})
	if v, err := rt.fixitEngine().Preview(ctx, "settings.ingress.trusted_proxies"); err != nil || v.State != fixit.StateHealthy {
		t.Fatalf("env-locked: %+v %v", v, err)
	}
}
