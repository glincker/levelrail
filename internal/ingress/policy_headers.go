package ingress

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/GLINCKER/levelrail/internal/trafficpolicy"
)

// policyHeaderOps is Caddy's headers.HeaderOps with every operation.
type policyHeaderOps struct {
	Add      map[string][]string `json:"add,omitempty"`
	Set      map[string][]string `json:"set,omitempty"`
	Delete   []string            `json:"delete,omitempty"`
	Deferred bool                `json:"deferred,omitempty"`
}

// policyHeadersHandler is Caddy's headers handler with both sides.
type policyHeadersHandler struct {
	Handler  string           `json:"handler"`
	Request  *policyHeaderOps `json:"request,omitempty"`
	Response *policyHeaderOps `json:"response,omitempty"`
}

func headerOp(op, name, value string, deferred bool) *policyHeaderOps {
	o := &policyHeaderOps{Deferred: deferred}
	name = http.CanonicalHeaderKey(name)
	switch op {
	case trafficpolicy.OpSet:
		o.Set = map[string][]string{name: {escapePlaceholders(value)}}
	case trafficpolicy.OpAdd:
		o.Add = map[string][]string{name: {escapePlaceholders(value)}}
	default:
		o.Delete = []string{name}
	}
	return o
}

// headerHandlers compiles one domain's header policy. Each rule is its own
// handler so order holds: request rules run in order; deferred response
// rules apply innermost first, so they are emitted reversed and ahead of
// the presets, letting a later rule win over an earlier one and any rule
// win over a preset.
func (p *RoutePolicy) headerHandlers(h trafficpolicy.Headers) []any {
	var out []any
	if h.ForwardedPrefix != "" {
		out = append(out, policyHeadersHandler{Handler: "headers", Request: headerOp(trafficpolicy.OpSet, "X-Forwarded-Prefix", h.ForwardedPrefix, false)})
	}
	for _, r := range h.Rules {
		if r.Side == trafficpolicy.SideRequest {
			out = append(out, policyHeadersHandler{Handler: "headers", Request: headerOp(r.Op, r.Name, r.Value, false)})
		}
	}
	if h.CORS != nil {
		out = append(out, corsHandler(*h.CORS))
	}
	for i := len(h.Rules) - 1; i >= 0; i-- {
		r := h.Rules[i]
		if r.Side == trafficpolicy.SideResponse {
			out = append(out, policyHeadersHandler{Handler: "headers", Response: headerOp(r.Op, r.Name, r.Value, true)})
		}
	}
	if h.Security != nil {
		out = append(out, p.securityHandlers(*h.Security)...)
	}
	if h.HideServer {
		out = append(out, policyHeadersHandler{Handler: "headers", Response: &policyHeaderOps{Delete: []string{"Server", "X-Powered-By"}, Deferred: true}})
	}
	return out
}

func (p *RoutePolicy) securityHandlers(s trafficpolicy.SecurityPreset) []any {
	set := map[string][]string{}
	if s.ContentTypeOptions {
		set["X-Content-Type-Options"] = []string{"nosniff"}
	}
	if s.FrameOptions != "" {
		set["X-Frame-Options"] = []string{s.FrameOptions}
	}
	if s.ReferrerPolicy != "" {
		set["Referrer-Policy"] = []string{s.ReferrerPolicy}
	}
	if s.PermissionsPolicy != "" {
		set["Permissions-Policy"] = []string{escapePlaceholders(s.PermissionsPolicy)}
	}
	var out []any
	if len(set) > 0 {
		out = append(out, policyHeadersHandler{Handler: "headers", Response: &policyHeaderOps{Set: set, Deferred: true}})
	}
	if !s.HSTS || (!p.TLSReal && !p.TLSTerminatedUpstream) {
		return out
	}
	maxAge := s.HSTSMaxAge
	if maxAge == 0 {
		maxAge = trafficpolicy.DefaultHSTSMaxAge
	}
	value := "max-age=" + strconv.Itoa(maxAge)
	if s.HSTSIncludeSubdomains {
		value += "; includeSubDomains"
	}
	hsts := policyHeadersHandler{Handler: "headers", Response: &policyHeaderOps{
		Set: map[string][]string{"Strict-Transport-Security": {value}}, Deferred: true,
	}}
	if p.TLSTerminatedUpstream {
		return append(out, hsts)
	}
	// Only over HTTPS: RFC 6797 says browsers ignore HSTS on plain HTTP anyway.
	return append(out, newPolicySubroute([]policyRoute{{
		Match:  []policyMatcher{{Expression: &ExpressionMatcher{Expr: "{http.request.scheme} == 'https'"}}},
		Handle: []any{hsts},
	}}))
}

// corsOriginExpr matches requests whose Origin is allowed. Origins are
// validated (scheme, host, port only) before they reach this expression.
func corsOriginExpr(origins []string) string {
	for _, o := range origins {
		if o == "*" {
			return "{http.request.header.Origin} != ''"
		}
	}
	quoted := make([]string, len(origins))
	for i, o := range origins {
		quoted[i] = "'" + strings.TrimSuffix(o, "/") + "'"
	}
	return fmt.Sprintf("{http.request.header.Origin} in [%s]", strings.Join(quoted, ", "))
}

func corsAllowOrigin(c trafficpolicy.CORSPreset) string {
	for _, o := range c.Origins {
		if o == "*" {
			return "*"
		}
	}
	return "{http.request.header.Origin}"
}

func corsHandler(c trafficpolicy.CORSPreset) policySubroute {
	originExpr := corsOriginExpr(c.Origins)
	allowOrigin := corsAllowOrigin(c)
	common := map[string][]string{"Access-Control-Allow-Origin": {allowOrigin}}
	if allowOrigin != "*" {
		common["Vary"] = []string{"Origin"}
	}
	if c.Credentials {
		common["Access-Control-Allow-Credentials"] = []string{"true"}
	}
	var routes []policyRoute
	if c.Preflight {
		pre := map[string][]string{}
		for k, v := range common {
			pre[k] = v
		}
		methods := c.Methods
		if len(methods) == 0 {
			methods = []string{"GET", "HEAD", "POST"}
		}
		pre["Access-Control-Allow-Methods"] = []string{strings.Join(methods, ", ")}
		if len(c.Headers) > 0 {
			pre["Access-Control-Allow-Headers"] = []string{strings.Join(c.Headers, ", ")}
		} else {
			pre["Access-Control-Allow-Headers"] = []string{"{http.request.header.Access-Control-Request-Headers}"}
		}
		if c.MaxAgeSeconds > 0 {
			pre["Access-Control-Max-Age"] = []string{strconv.Itoa(c.MaxAgeSeconds)}
		}
		routes = append(routes, policyRoute{
			Match: []policyMatcher{{
				Method:     []string{http.MethodOptions},
				Expression: &ExpressionMatcher{Expr: originExpr + " && {http.request.header.Access-Control-Request-Method} != ''"},
			}},
			Handle:   []any{StaticResponseHandler{Handler: "static_response", StatusCode: http.StatusNoContent, Headers: pre}},
			Terminal: true,
		})
	}
	resp := map[string][]string{}
	var add map[string][]string
	for k, v := range common {
		if k == "Vary" {
			add = map[string][]string{k: v}
			continue
		}
		resp[k] = v
	}
	if len(c.ExposeHeaders) > 0 {
		resp["Access-Control-Expose-Headers"] = []string{strings.Join(c.ExposeHeaders, ", ")}
	}
	routes = append(routes, policyRoute{
		Match:  []policyMatcher{{Expression: &ExpressionMatcher{Expr: originExpr}}},
		Handle: []any{policyHeadersHandler{Handler: "headers", Response: &policyHeaderOps{Set: resp, Add: add, Deferred: true}}},
	})
	return newPolicySubroute(routes)
}
