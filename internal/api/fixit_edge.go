package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/fixit"
	"github.com/GLINCKER/levelrail/internal/ingress"
)

const settingObservedProxy = "ingress_observed_proxy"

// observeUpstreamProxy remembers the proxy seen on an authenticated dashboard
// request. Unauthenticated callers never reach it: a spoofed header must not become a trust suggestion.
func (rt *Router) observeUpstreamProxy(ctx context.Context, det edgeDetection) {
	if det.Proxy == proxyKindNone {
		return
	}
	if cur := rt.fixit.observed.Load(); cur != nil && cur.Proxy == det.Proxy && cur.Suggested == det.Suggested {
		return
	}
	rt.fixit.observed.Store(&det)
	if rt.fixit.store == nil {
		return
	}
	raw, err := json.Marshal(det)
	if err != nil {
		return
	}
	if err := rt.fixit.store.SetFixitSetting(ctx, settingObservedProxy, string(raw), "system", time.Now()); err != nil {
		rt.logger.Warn("api: persist observed proxy failed", "error", err.Error())
	}
}

func (rt *Router) observedProxy(ctx context.Context) *edgeDetection {
	if cur := rt.fixit.observed.Load(); cur != nil {
		return cur
	}
	if rt.fixit.store == nil {
		return nil
	}
	raw, ok, err := rt.fixit.store.GetFixitSetting(ctx, settingObservedProxy)
	if err != nil || !ok {
		return nil
	}
	var det edgeDetection
	if json.Unmarshal([]byte(raw), &det) != nil || det.Proxy == proxyKindNone || det.Proxy == "" {
		return nil
	}
	rt.fixit.observed.Store(&det)
	return &det
}

func (rt *Router) trustedProxiesActive(ctx context.Context) bool {
	return strings.TrimSpace(rt.domainPolicy.env(ingress.EnvTrustedProxies)) != "" || rt.edgeTrustedProxiesSet(ctx)
}

func (rt *Router) trustedProxiesFix() fixit.Fix {
	return fixit.Fix{
		ID: "settings.ingress.trusted_proxies", CheckID: "trusted_proxies", Subject: SubjectSettings, Kind: fixit.KindAuto,
		Title: "Trust the proxy in front of this server",
		Why:   "Requests arrive through a proxy, so apps and per-visitor limits see the proxy address instead of the real visitor.",
		Risk:  fixit.RiskSafe, Reversible: true, Needs: AbilityRoot,
		Detect: func(ctx context.Context) (fixit.Applicability, error) {
			a := fixit.Applicability{Changes: []fixit.Change{}}
			if rt.edgeStore() == nil || rt.trustedProxiesActive(ctx) {
				return a, nil
			}
			det := rt.observedProxy(ctx)
			if det == nil {
				return a, nil
			}
			a.Failing = true
			a.Detail = fmt.Sprintf("requests come through a %s proxy but no trusted proxies are set", det.Proxy)
			a.Signature = "trusted_proxies:" + det.Proxy
			if det.Suggested == "" {
				a.Kind, a.Reason = fixit.KindGuided, "no address is safe to trust automatically; enter the proxy addresses in Settings, Ingress"
				return a, nil
			}
			a.Changes = []fixit.Change{{Label: "Trust " + det.Suggested + " as the upstream proxy", Detail: "saved as a setting; the ingress picks it up without a restart"}}
			return a, nil
		},
		Run: func(ctx context.Context, _ fixit.Input) error {
			st := rt.edgeStore()
			det := rt.observedProxy(ctx)
			if st == nil || det == nil || det.Suggested == "" {
				return fixit.ErrCannotFixHere
			}
			if rt.trustedProxiesActive(ctx) {
				return errors.New("trusted proxies are already set")
			}
			cur, err := st.GetIngressEdgeSettings(ctx)
			if err != nil {
				return fmt.Errorf("fixit: load edge settings: %w", err)
			}
			cur.TrustedProxies = det.Suggested
			if err := st.UpdateIngressEdgeSettings(ctx, cur); err != nil {
				return fmt.Errorf("fixit: save trusted proxies: %w", err)
			}
			rt.nudgeReconciler()
			return nil
		},
	}
}
