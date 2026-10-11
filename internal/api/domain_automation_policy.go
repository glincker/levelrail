package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
)

// www_policy values.
const (
	wwwPolicyOff            = "off"
	wwwPolicyRedirectApex   = "redirect_to_apex"
	wwwPolicyRedirectToWWW  = "redirect_to_www"
	domainAutomationMaxBody = 8 << 10
)

// domainAutomationPolicy is the effective go-live policy.
type domainAutomationPolicy struct {
	AutoDNS               bool   `json:"auto_dns"`
	AutoProxyRoute        bool   `json:"auto_proxy_route"`
	ForceHTTPS            bool   `json:"force_https"`
	WWWPolicy             string `json:"www_policy"`
	AttachWWWCounterpart  bool   `json:"attach_www_counterpart"`
	WildcardForBaseDomain bool   `json:"wildcard_for_base_domain"`
	VerifyAfter           bool   `json:"verify_after"`
}

// domainAutomationOverride is the stored (and per-request) form: an omitted
// key keeps the default, which for auto_dns and auto_proxy_route depends on
// what is connected.
type domainAutomationOverride struct {
	AutoDNS               *bool   `json:"auto_dns,omitempty"`
	AutoProxyRoute        *bool   `json:"auto_proxy_route,omitempty"`
	ForceHTTPS            *bool   `json:"force_https,omitempty"`
	WWWPolicy             *string `json:"www_policy,omitempty"`
	AttachWWWCounterpart  *bool   `json:"attach_www_counterpart,omitempty"`
	WildcardForBaseDomain *bool   `json:"wildcard_for_base_domain,omitempty"`
	VerifyAfter           *bool   `json:"verify_after,omitempty"`
}

func validWWWPolicy(v string) bool {
	switch v {
	case wwwPolicyOff, wwwPolicyRedirectApex, wwwPolicyRedirectToWWW:
		return true
	}
	return false
}

func (o domainAutomationOverride) validate() string {
	if o.WWWPolicy != nil && !validWWWPolicy(*o.WWWPolicy) {
		return "www_policy must be off, redirect_to_apex or redirect_to_www"
	}
	return ""
}

// apply returns p with every key o sets.
func (o domainAutomationOverride) apply(p domainAutomationPolicy) domainAutomationPolicy {
	if o.AutoDNS != nil {
		p.AutoDNS = *o.AutoDNS
	}
	if o.AutoProxyRoute != nil {
		p.AutoProxyRoute = *o.AutoProxyRoute
	}
	if o.ForceHTTPS != nil {
		p.ForceHTTPS = *o.ForceHTTPS
	}
	if o.WWWPolicy != nil {
		p.WWWPolicy = *o.WWWPolicy
	}
	if o.AttachWWWCounterpart != nil {
		p.AttachWWWCounterpart = *o.AttachWWWCounterpart
	}
	if o.WildcardForBaseDomain != nil {
		p.WildcardForBaseDomain = *o.WildcardForBaseDomain
	}
	if o.VerifyAfter != nil {
		p.VerifyAfter = *o.VerifyAfter
	}
	return p
}

// defaultDomainAutomationPolicy is the policy with nothing stored.
func defaultDomainAutomationPolicy(providerActive, proxyEnabled bool) domainAutomationPolicy {
	return domainAutomationPolicy{
		AutoDNS: providerActive, AutoProxyRoute: proxyEnabled, ForceHTTPS: true,
		WWWPolicy: wwwPolicyOff, VerifyAfter: true,
	}
}

type domainAutomationStore interface {
	GetDomainAutomationRaw(ctx context.Context) (string, error)
	SetDomainAutomationRaw(ctx context.Context, raw string) error
}

func (rt *Router) automationStore() domainAutomationStore {
	s, _ := rt.apps.(domainAutomationStore)
	return s
}

func (rt *Router) storedAutomation(ctx context.Context) domainAutomationOverride {
	var o domainAutomationOverride
	if s := rt.automationStore(); s != nil {
		if raw, err := s.GetDomainAutomationRaw(ctx); err == nil && raw != "" {
			_ = json.Unmarshal([]byte(raw), &o)
		}
	}
	return o
}

// effectiveAutomation merges defaults, the stored policy and an optional
// per-request override.
func (rt *Router) effectiveAutomation(ctx context.Context, r *http.Request, ov *domainAutomationOverride) domainAutomationPolicy {
	proxy := false
	if v := rt.goLiveProxyView(ctx, r); v != nil {
		proxy = v.Enabled
	}
	p := defaultDomainAutomationPolicy(rt.activeDNSProvider(ctx) != dnsProviderNone, proxy)
	p = rt.storedAutomation(ctx).apply(p)
	if ov != nil {
		p = ov.apply(p)
	}
	return p
}

type domainAutomationResource struct {
	domainAutomationPolicy
	DNSProvider      string `json:"dns_provider"`
	ProxyIntegration bool   `json:"proxy_integration_enabled"`
	// WildcardDNS is set on PUT when the wildcard record for the apps base
	// domain was planned or written.
	WildcardDNS *domainDNSResult `json:"wildcard_dns,omitempty"`
}

// handleGetDomainAutomation handles GET /api/v1/settings/domain-automation.
func (rt *Router) handleGetDomainAutomation(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, rt.automationResource(r, nil))
}

func (rt *Router) automationResource(r *http.Request, wildcard *domainDNSResult) domainAutomationResource {
	proxy := false
	if v := rt.goLiveProxyView(r.Context(), r); v != nil {
		proxy = v.Enabled
	}
	return domainAutomationResource{
		domainAutomationPolicy: rt.effectiveAutomation(r.Context(), r, nil),
		DNSProvider:            rt.activeDNSProvider(r.Context()),
		ProxyIntegration:       proxy,
		WildcardDNS:            wildcard,
	}
}

// ensureBaseWildcard writes the *.<base> record when the policy asks for it.
func (rt *Router) ensureBaseWildcard(r *http.Request) *domainDNSResult {
	ctx := r.Context()
	base := rt.appsBaseDomain(ctx)
	if base == "" || !rt.effectiveAutomation(ctx, r, nil).WildcardForBaseDomain {
		return nil
	}
	d, _ := rt.applyDomainDNS(ctx, r, "", "*."+base, dnsOpts{Mode: dnsModeAuto, CanWrite: true})
	return &d
}

// handleUpdateDomainAutomation handles PUT /api/v1/settings/domain-automation
// (root): the body is the full stored policy; an omitted key returns to its
// default.
func (rt *Router) handleUpdateDomainAutomation(w http.ResponseWriter, r *http.Request) {
	store := rt.automationStore()
	if store == nil {
		writeError(w, http.StatusNotImplemented, "domain automation is not supported by this store")
		return
	}
	var ov domainAutomationOverride
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, domainAutomationMaxBody)).Decode(&ov); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if msg := ov.validate(); msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}
	raw, err := json.Marshal(ov)
	if err != nil {
		rt.internalError(w, "api: encode domain automation", err)
		return
	}
	if err := store.SetDomainAutomationRaw(r.Context(), string(raw)); err != nil {
		rt.internalError(w, "api: save domain automation", err)
		return
	}
	writeJSON(w, http.StatusOK, rt.automationResource(r, rt.ensureBaseWildcard(r)))
}

// counterpartHost returns the www/apex partner of domain. zone may be empty
// when no provider is connected, in which case a two-label name is the apex.
func counterpartHost(domain, zoneRelative string, zoneKnown bool) (host string, domainIsWWW, ok bool) {
	if rest, found := strings.CutPrefix(domain, "www."); found && strings.Contains(rest, ".") {
		return rest, true, true
	}
	apex := false
	if zoneKnown {
		apex = zoneRelative == "@"
	} else {
		apex = strings.Count(domain, ".") == 1
	}
	if apex {
		return "www." + domain, false, true
	}
	return "", false, false
}

// canonicalFor decides which of the pair is canonical under policy: the
// redirect source is the other host.
func canonicalFor(domain, counterpart string, domainIsWWW bool, policy string) (canonical, source string, redirect bool) {
	apex, www := domain, counterpart
	if domainIsWWW {
		apex, www = counterpart, domain
	}
	switch policy {
	case wwwPolicyRedirectApex:
		return apex, www, true
	case wwwPolicyRedirectToWWW:
		return www, apex, true
	}
	return domain, "", false
}
