package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/netip"
	"strings"

	"github.com/GLINCKER/levelrail/internal/ingress"
	"github.com/GLINCKER/levelrail/internal/trafficpolicy"
)

// policyLimitsResource is the caps the dashboard shows next to each editor.
type policyLimitsResource struct {
	MaxHeaderRules    int   `json:"max_header_rules"`
	MaxForwarders     int   `json:"max_forwarders"`
	MaxCacheRules     int   `json:"max_cache_rules"`
	MaxRegexLen       int   `json:"max_regex_len"`
	MaxHeaderValueLen int   `json:"max_header_value_len"`
	CacheMaxObject    int64 `json:"cache_max_object_bytes"`
	CacheTotalBytes   int64 `json:"cache_total_bytes"`
	ForwardTimeoutSec int   `json:"forward_timeout_seconds"`
}

// domainPoliciesResource is GET .../policies: every section plus the
// status the page needs, in one request.
type domainPoliciesResource struct {
	Domain    string                       `json:"domain"`
	App       string                       `json:"app"`
	Policy    trafficpolicy.Policy         `json:"policy"`
	UpdatedAt map[string]string            `json:"updated_at"`
	TLSReal   bool                         `json:"tls_real"`
	Geo       ingress.GeoStatus            `json:"geo"`
	Cache     ingress.CacheStats           `json:"cache"`
	Limits    policyLimitsResource         `json:"limits"`
	Preview   []trafficpolicy.Step         `json:"preview"`
	Warnings  []string                     `json:"warnings,omitempty"`
	Security  trafficpolicy.SecurityPreset `json:"security_preset"`
}

func (rt *Router) handleGetDomainPolicies(w http.ResponseWriter, r *http.Request) {
	domain, ok := rt.requireOwnedDomain(w, r)
	if !ok {
		return
	}
	p, updated, err := rt.loadDomainPolicy(r.Context(), domain)
	var warnings []string
	if err != nil {
		rt.logger.Warn("api: decode domain policy", slog.String("domain", domain), slog.String("error", err.Error()))
		warnings = append(warnings, "a stored section could not be read and is ignored: "+err.Error())
	}
	pctx, err := rt.policyContext(r.Context(), domain)
	if err != nil {
		rt.internalError(w, "api: domain policy context failed", err, slog.String("domain", domain))
		return
	}
	geo := ingress.DefaultGeoResolver().Status()
	if p.Geo != nil && !geo.Active {
		warnings = append(warnings, "geo rules are saved but inactive: no country source is configured (see APP_GEOIP_DB)")
	}
	l := rt.domainPolicy.limits()
	writeJSON(w, http.StatusOK, domainPoliciesResource{
		Domain:    domain,
		App:       r.PathValue("name"),
		Policy:    p,
		UpdatedAt: updated,
		TLSReal:   pctx.TLSReal,
		Geo:       geo,
		Cache:     ingress.DefaultResponseCache().Stats(domain),
		Limits: policyLimitsResource{
			MaxHeaderRules: l.MaxHeaderRules, MaxForwarders: l.MaxForwarders, MaxCacheRules: l.MaxCacheRules,
			MaxRegexLen: l.MaxRegexLen, MaxHeaderValueLen: l.MaxHeaderValueLen, CacheMaxObject: l.CacheMaxObject,
			CacheTotalBytes: ingress.DefaultResponseCache().MaxBytes(), ForwardTimeoutSec: int(l.ForwardTimeout.Seconds()),
		},
		Preview:  trafficpolicy.Explain(p, "GET", "/"),
		Warnings: warnings,
		Security: trafficpolicy.DefaultSecurityPreset(pctx.TLSReal),
	})
}

// policyPreviewRequest overlays draft sections on the stored policy and walks
// one request (and the redirect sample URLs) through it.
type policyPreviewRequest struct {
	Policy    *trafficpolicy.Policy `json:"policy,omitempty"`
	Method    string                `json:"method,omitempty"`
	Path      string                `json:"path,omitempty"`
	URLs      []string              `json:"urls,omitempty"`
	Canonical string                `json:"canonical_preset,omitempty"`
}

type policyPreviewHops struct {
	URL   string              `json:"url"`
	Hops  []trafficpolicy.Hop `json:"hops"`
	Error string              `json:"error,omitempty"`
}

type policyPreviewResponse struct {
	Steps  []trafficpolicy.Step       `json:"steps"`
	Hops   []policyPreviewHops        `json:"hops,omitempty"`
	Errors []trafficpolicy.FieldError `json:"errors,omitempty"`
}

const maxPreviewURLs = 6

func (rt *Router) handlePreviewDomainPolicies(w http.ResponseWriter, r *http.Request) {
	domain, ok := rt.requireOwnedDomain(w, r)
	if !ok {
		return
	}
	var req policyPreviewRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxPolicyBody)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if len(req.URLs) > maxPreviewURLs {
		writeError(w, http.StatusBadRequest, "at most 6 sample URLs")
		return
	}
	stored, _, _ := rt.loadDomainPolicy(r.Context(), domain)
	merged := stored
	if d := req.Policy; d != nil {
		if d.Headers != nil {
			merged.Headers = d.Headers
		}
		if d.Forwarders != nil {
			merged.Forwarders = d.Forwarders
		}
		if d.Geo != nil {
			merged.Geo = d.Geo
		}
		if d.Cache != nil {
			merged.Cache = d.Cache
		}
		if d.Redirects != nil {
			merged.Redirects = d.Redirects
		}
	}
	resp := policyPreviewResponse{Steps: trafficpolicy.Explain(merged, strings.ToUpper(req.Method), req.Path)}
	if pctx, err := rt.policyContext(r.Context(), domain); err == nil {
		if err := merged.Validate(rt.domainPolicy.limits(), pctx); err != nil {
			resp.Errors = append(resp.Errors, splitValidation(err)...)
		}
	}
	if len(req.URLs) > 0 {
		env, err := rt.hopEnv(r.Context(), r.PathValue("name"), domain, merged.Redirects, req.Canonical)
		if err != nil {
			rt.internalError(w, "api: preview redirect env failed", err, slog.String("domain", domain))
			return
		}
		for _, u := range req.URLs {
			hops, err := trafficpolicy.SimulateHops(env, u)
			ph := policyPreviewHops{URL: u, Hops: hops}
			if err != nil {
				ph.Error = err.Error()
			}
			resp.Hops = append(resp.Hops, ph)
		}
	}
	writeJSON(w, http.StatusOK, resp)
}

// splitValidation flattens a joined validation error into field errors,
// prefixing each with its section kind.
func splitValidation(err error) []trafficpolicy.FieldError {
	type unwrapper interface{ Unwrap() []error }
	var out []trafficpolicy.FieldError
	errs := []error{err}
	if u, ok := err.(unwrapper); ok {
		errs = u.Unwrap()
	}
	for _, e := range errs {
		if ve, ok := trafficpolicy.AsValidationError(e); ok {
			for _, f := range ve.Fields {
				out = append(out, trafficpolicy.FieldError{Field: ve.Kind + "." + f.Field, Message: f.Message})
			}
			continue
		}
		out = append(out, trafficpolicy.FieldError{Message: e.Error()})
	}
	return out
}

type purgeRequest struct {
	Scope string `json:"scope"`
	Value string `json:"value,omitempty"`
}

type purgeResponse struct {
	Domain string `json:"domain"`
	Purged int    `json:"purged"`
}

func (rt *Router) handlePurgeDomainCache(w http.ResponseWriter, r *http.Request) {
	domain, ok := rt.requireOwnedDomain(w, r)
	if !ok {
		return
	}
	var req purgeRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	switch req.Scope {
	case ingress.PurgeAll:
	case ingress.PurgeURL, ingress.PurgePrefix:
		if !strings.HasPrefix(req.Value, "/") || len(req.Value) > 2048 {
			writeError(w, http.StatusBadRequest, "value must be a path starting with / (a URL path and query for scope url)")
			return
		}
	default:
		writeError(w, http.StatusBadRequest, "scope must be url, prefix or all")
		return
	}
	writeJSON(w, http.StatusOK, purgeResponse{Domain: domain, Purged: ingress.DefaultResponseCache().Purge(domain, req.Scope, req.Value)})
}

func (rt *Router) handleGetDomainCacheStats(w http.ResponseWriter, r *http.Request) {
	domain, ok := rt.requireOwnedDomain(w, r)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, ingress.DefaultResponseCache().Stats(domain))
}

type geoLookupResponse struct {
	Status  ingress.GeoStatus `json:"status"`
	IP      string            `json:"ip,omitempty"`
	Country string            `json:"country,omitempty"`
	Source  string            `json:"source,omitempty"`
	Private bool              `json:"private,omitempty"`
	Note    string            `json:"note,omitempty"`
}

// handleGeoIPLookup handles GET /api/v1/system/geoip?ip=: the active
// country sources and, with ip, what the MMDB says about it. The header
// source cannot be tested here because it comes from the visitor's proxy.
func (rt *Router) handleGeoIPLookup(w http.ResponseWriter, r *http.Request) {
	res := ingress.DefaultGeoResolver()
	out := geoLookupResponse{Status: res.Status()}
	raw := strings.TrimSpace(r.URL.Query().Get("ip"))
	if raw == "" {
		writeJSON(w, http.StatusOK, out)
		return
	}
	addr, err := netip.ParseAddr(raw)
	if err != nil {
		writeError(w, http.StatusBadRequest, "ip must be an IPv4 or IPv6 address")
		return
	}
	out.IP = addr.Unmap().String()
	if trafficpolicy.NeverGeoBlocked(addr) {
		out.Private = true
		out.Note = "private, loopback and link-local addresses are never blocked by country"
	}
	out.Country, out.Source = res.Country(addr, "", false)
	if out.Country == "" && out.Note == "" {
		out.Note = "no country found; requests like this follow the rule's unknown setting"
	}
	writeJSON(w, http.StatusOK, out)
}
