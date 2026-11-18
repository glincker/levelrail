package ingress

import (
	"net/http"
	"sync/atomic"

	"github.com/caddyserver/caddy/v2/modules/caddyhttp"

	"github.com/GLINCKER/levelrail/internal/trafficclass"
)

// trafficRules holds per-route-key path rules, copy-on-write like the other
// process-wide registries. A key with no entry uses the built-in defaults.
var trafficRules atomic.Pointer[map[string]*trafficclass.Rules]

// SetTrafficRules replaces the per-route-key path rules.
func SetTrafficRules(byKey map[string]*trafficclass.Rules) {
	cp := make(map[string]*trafficclass.Rules, len(byKey))
	for k, v := range byKey {
		cp[k] = v
	}
	trafficRules.Store(&cp)
}

func trafficRulesFor(key string) *trafficclass.Rules {
	if p := trafficRules.Load(); p != nil {
		return (*p)[key]
	}
	return nil
}

// requestClass classifies a finished request. Only headers already read for the
// request are used; nothing from the query string or the body.
func requestClass(key string, r *http.Request, contentType string, status int) trafficclass.Class {
	shielded := false
	if n, ok := caddyhttp.GetVar(r.Context(), shieldOutcomeVar).(int); ok && n > 0 {
		shielded = true
	}
	return trafficclass.Classify(r, trafficclass.Response{Status: status, ContentType: contentType, Shielded: shielded}, trafficRulesFor(key)).Class
}
