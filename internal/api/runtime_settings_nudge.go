package api

import "github.com/GLINCKER/levelrail/internal/runtimesettings"

// renderedSettings are read when the ingress config is rendered, so a change needs a reconcile to reach Caddy.
var renderedSettings = map[string]bool{runtimesettings.KeyIngressMaxBodyBytes: true, runtimesettings.KeyIngressTrustedProxyPreset: true, runtimesettings.KeyIngressFallbackCerts: true}

func (rt *Router) nudgeIfRenderedSetting(key string) {
	if renderedSettings[key] {
		rt.nudgeReconciler()
	}
}
