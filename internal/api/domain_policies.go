package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/trafficpolicy"
)

// DomainTrafficPolicyStore is the store surface for the per-domain traffic
// controls (headers, forwarders, geo, cache, redirects).
type DomainTrafficPolicyStore interface {
	GetDomainTrafficPolicy(ctx context.Context, domain, kind string) (store.DomainTrafficPolicy, bool, error)
	ListDomainTrafficPoliciesFor(ctx context.Context, domain string) ([]store.DomainTrafficPolicy, error)
	SetDomainTrafficPolicy(ctx context.Context, domain, kind string, spec []byte) error
	DeleteDomainTrafficPolicy(ctx context.Context, domain, kind string) error
}

// domainPolicyDeps groups what the traffic control handlers need beyond
// the core stores.
type domainPolicyDeps struct {
	store DomainTrafficPolicyStore
	// resolver checks forwarder hosts; nil uses net.DefaultResolver.
	resolver trafficpolicy.Resolver
	getenv   func(string) string
}

func (d domainPolicyDeps) env(key string) string {
	if d.getenv != nil {
		return d.getenv(key)
	}
	return os.Getenv(key)
}

func (d domainPolicyDeps) limits() trafficpolicy.Limits {
	return trafficpolicy.LimitsFromEnv(d.env)
}

func (d domainPolicyDeps) dns() trafficpolicy.Resolver {
	if d.resolver != nil {
		return d.resolver
	}
	return net.DefaultResolver
}

const maxPolicyBody = 256 << 10

// domainPolicyResource is GET/PUT .../{kind}'s wire shape. Spec is the
// section's JSON; an unconfigured section returns its empty default.
type domainPolicyResource struct {
	Domain     string          `json:"domain"`
	Kind       string          `json:"kind"`
	Configured bool            `json:"configured"`
	Spec       json.RawMessage `json:"spec"`
	UpdatedAt  string          `json:"updated_at,omitempty"`
}

// policyValidationResponse is a 400 with per-field errors for inline display.
type policyValidationResponse struct {
	Error  string                     `json:"error"`
	Fields []trafficpolicy.FieldError `json:"fields"`
}

func defaultSpec(kind string) json.RawMessage {
	switch kind {
	case trafficpolicy.KindHeaders, trafficpolicy.KindForwarders:
		return json.RawMessage(`{"rules":[]}`)
	case trafficpolicy.KindCache:
		return json.RawMessage(`{"enabled":false,"rules":[]}`)
	case trafficpolicy.KindRedirects:
		return json.RawMessage(`{"force_https":false}`)
	}
	return json.RawMessage(`null`)
}

func toDomainPolicyResource(domain, kind string, row store.DomainTrafficPolicy, found bool) domainPolicyResource {
	res := domainPolicyResource{Domain: domain, Kind: kind, Configured: found, Spec: defaultSpec(kind)}
	if found {
		res.Spec = json.RawMessage(row.Spec)
		res.UpdatedAt = row.UpdatedAt
	}
	return res
}

func (rt *Router) handleGetDomainPolicy(kind string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		domain, ok := rt.requireOwnedDomain(w, r)
		if !ok {
			return
		}
		row, found, err := rt.domainPolicy.store.GetDomainTrafficPolicy(r.Context(), domain, kind)
		if err != nil {
			rt.internalError(w, "api: get domain policy failed", err, slog.String("domain", domain), slog.String("kind", kind))
			return
		}
		writeJSON(w, http.StatusOK, toDomainPolicyResource(domain, kind, row, found))
	}
}

// decodePolicy strictly parses one section from the request body.
func decodePolicy(r *http.Request, w http.ResponseWriter, kind string) (trafficpolicy.Policy, []byte, error) {
	body := http.MaxBytesReader(w, r.Body, maxPolicyBody)
	var raw json.RawMessage
	if err := json.NewDecoder(body).Decode(&raw); err != nil {
		return trafficpolicy.Policy{}, nil, fmt.Errorf("invalid request body: %w", err)
	}
	var p trafficpolicy.Policy
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	var target any
	switch kind {
	case trafficpolicy.KindHeaders:
		p.Headers = &trafficpolicy.Headers{}
		target = p.Headers
	case trafficpolicy.KindForwarders:
		p.Forwarders = &trafficpolicy.Forwarders{}
		target = p.Forwarders
	case trafficpolicy.KindGeo:
		p.Geo = &trafficpolicy.Geo{}
		target = p.Geo
	case trafficpolicy.KindCache:
		p.Cache = &trafficpolicy.Cache{}
		target = p.Cache
	case trafficpolicy.KindRedirects:
		p.Redirects = &trafficpolicy.Redirects{}
		target = p.Redirects
	}
	if err := dec.Decode(target); err != nil {
		return trafficpolicy.Policy{}, nil, fmt.Errorf("invalid %s policy: %w", kind, err)
	}
	canonical, err := json.Marshal(target)
	if err != nil {
		return trafficpolicy.Policy{}, nil, fmt.Errorf("encode %s policy: %w", kind, err)
	}
	return p, canonical, nil
}

// policyContext gathers what validation needs about the domain.
func (rt *Router) policyContext(ctx context.Context, domain string) (trafficpolicy.Context, error) {
	settings, err := rt.ingressSettings.GetIngressSettings(ctx)
	if err != nil {
		return trafficpolicy.Context{}, fmt.Errorf("get ingress settings: %w", err)
	}
	_, byo, err := rt.domainTLSCert.GetDomainTLSCert(ctx, domain)
	if err != nil {
		return trafficpolicy.Context{}, fmt.Errorf("get domain tls cert: %w", err)
	}
	return trafficpolicy.Context{
		Domain:  domain,
		TLSReal: settings.EffectiveACMEEnabled() || byo || settings.TLSTerminatedUpstream,
		AppExists: func(name string) bool {
			_, err := rt.apps.GetDesiredService(ctx, name)
			return err == nil
		},
	}, nil
}

// checkForwarderTargets runs the egress guard on every external URL so a
// rule pointing at a private or metadata address is refused at save time.
// The reconciler repeats the check each pass to catch DNS changes.
func (rt *Router) checkForwarderTargets(ctx context.Context, fw *trafficpolicy.Forwarders) error {
	guard := trafficpolicy.EgressGuardFromEnv(rt.domainPolicy.env)
	var fields []trafficpolicy.FieldError
	for i, f := range fw.Rules {
		if f.Action != trafficpolicy.ActionURL {
			continue
		}
		cctx, cancel := context.WithTimeout(ctx, 3*time.Second)
		_, err := guard.CheckTarget(cctx, rt.domainPolicy.dns(), f.URL)
		cancel()
		if err != nil {
			fields = append(fields, trafficpolicy.FieldError{Field: fmt.Sprintf("rules[%d].url", i), Message: err.Error()})
		}
	}
	if len(fields) > 0 {
		return &trafficpolicy.ValidationError{Kind: trafficpolicy.KindForwarders, Fields: fields}
	}
	return nil
}

func writePolicyError(w http.ResponseWriter, err error) {
	if ve, ok := trafficpolicy.AsValidationError(err); ok {
		writeJSON(w, http.StatusBadRequest, policyValidationResponse{Error: ve.Error(), Fields: ve.Fields})
		return
	}
	writeError(w, http.StatusBadRequest, err.Error())
}

func (rt *Router) handleSetDomainPolicy(kind string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		domain, ok := rt.requireOwnedDomain(w, r)
		if !ok {
			return
		}
		rt.saveDomainPolicy(w, r, domain, kind)
	}
}

func (rt *Router) saveDomainPolicy(w http.ResponseWriter, r *http.Request, domain, kind string) {
	p, canonical, err := decodePolicy(r, w, kind)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	pctx, err := rt.policyContext(r.Context(), domain)
	if err != nil {
		rt.internalError(w, "api: domain policy context failed", err, slog.String("domain", domain))
		return
	}
	if err := p.Validate(rt.domainPolicy.limits(), pctx); err != nil {
		writePolicyError(w, err)
		return
	}
	if p.Forwarders != nil {
		if err := rt.checkForwarderTargets(r.Context(), p.Forwarders); err != nil {
			writePolicyError(w, err)
			return
		}
	}
	if err := rt.domainPolicy.store.SetDomainTrafficPolicy(r.Context(), domain, kind, canonical); err != nil {
		rt.internalError(w, "api: set domain policy failed", err, slog.String("domain", domain), slog.String("kind", kind))
		return
	}
	rt.nudgeReconciler()
	row, found, err := rt.domainPolicy.store.GetDomainTrafficPolicy(r.Context(), domain, kind)
	if err != nil {
		rt.internalError(w, "api: get domain policy failed", err, slog.String("domain", domain))
		return
	}
	writeJSON(w, http.StatusOK, toDomainPolicyResource(domain, kind, row, found))
}

func (rt *Router) handleDeleteDomainPolicy(kind string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		domain, ok := rt.requireOwnedDomain(w, r)
		if !ok {
			return
		}
		if err := rt.domainPolicy.store.DeleteDomainTrafficPolicy(r.Context(), domain, kind); err != nil {
			rt.internalError(w, "api: delete domain policy failed", err, slog.String("domain", domain), slog.String("kind", kind))
			return
		}
		rt.nudgeReconciler()
		writeJSON(w, http.StatusOK, toDomainPolicyResource(domain, kind, store.DomainTrafficPolicy{}, false))
	}
}

// loadDomainPolicy decodes every stored section for domain.
func (rt *Router) loadDomainPolicy(ctx context.Context, domain string) (trafficpolicy.Policy, map[string]string, error) {
	rows, err := rt.domainPolicy.store.ListDomainTrafficPoliciesFor(ctx, domain)
	if err != nil {
		return trafficpolicy.Policy{}, nil, err
	}
	var p trafficpolicy.Policy
	updated := map[string]string{}
	var errs []error
	for _, row := range rows {
		if err := p.Decode(row.Kind, row.Spec); err != nil {
			errs = append(errs, err)
			continue
		}
		updated[row.Kind] = row.UpdatedAt
	}
	return p, updated, errors.Join(errs...)
}
