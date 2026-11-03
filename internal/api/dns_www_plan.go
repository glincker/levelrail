package api

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/dnsrecords"
	"github.com/GLINCKER/levelrail/internal/runtimesettings"
	"github.com/libdns/libdns"
)

type wwwPlanRecord struct {
	Name       string              `json:"name"`
	Type       string              `json:"type"`
	Value      string              `json:"value"`
	TTLSeconds int                 `json:"ttl_seconds"`
	Outcome    string              `json:"outcome"`
	Replaces   []dnsRecordResource `json:"replaces"`
}

type wwwPlanRedirect struct {
	From string `json:"from"`
	To   string `json:"to"`
}

type wwwPlanResponse struct {
	Domain    string           `json:"domain"`
	Provider  string           `json:"provider"`
	Zone      string           `json:"zone"`
	Mode      string           `json:"mode"`
	Canonical string           `json:"canonical"`
	Records   []wwwPlanRecord  `json:"records"`
	Redirect  *wwwPlanRedirect `json:"redirect"`
	Message   string           `json:"message"`
}

// handleGetDomainWWWPlan previews the apex and www records for a zone-apex
// domain. It never writes DNS.
func (rt *Router) handleGetDomainWWWPlan(w http.ResponseWriter, r *http.Request) {
	domain, ok := rt.requireOwnedDomain(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	mode, canonical := r.URL.Query().Get("mode"), r.URL.Query().Get("canonical")
	if mode == "" && rt.runtimeSettings != nil {
		mode = rt.runtimeSettings.String(ctx, runtimesettings.KeyDomainsWWWMode)
	}
	if canonical == "" && rt.runtimeSettings != nil {
		canonical = rt.runtimeSettings.String(ctx, runtimesettings.KeyDomainsCanonical)
	}
	if mode == "" {
		mode = dnsrecords.WWWBoth
	}
	if canonical == "" {
		canonical = dnsrecords.CanonicalApex
	}
	if !dnsrecords.ValidWWWMode(mode) {
		writeError(w, http.StatusBadRequest, "mode must be one of both, apex_only, www_only")
		return
	}
	if !dnsrecords.ValidCanonical(canonical) {
		writeError(w, http.StatusBadRequest, "canonical must be one of apex, www")
		return
	}
	res := wwwPlanResponse{Domain: domain, Mode: mode, Canonical: canonical, Records: []wwwPlanRecord{}}

	var existing []libdns.Record
	ttl := dnsAutoTTLDefault
	target, err := rt.resolveDNSTarget(ctx, domain)
	switch {
	case errors.Is(err, dnsrecords.ErrZoneNotFound), err == nil && target == nil:
		target = nil
		res.Message = "no DNS provider manages this domain's zone, so the plan assumes an empty zone"
	case err != nil:
		rt.logger.Error("api: www plan: resolve dns target failed", slog.String("error", err.Error()), slog.String("domain", domain))
		writeError(w, http.StatusBadGateway, "could not reach the DNS provider")
		return
	}
	settings, err := rt.ingressSettings.GetIngressSettings(ctx)
	if err != nil {
		rt.internalError(w, "api: www plan: ingress settings failed", err, slog.String("domain", domain))
		return
	}
	if settings.DNSTTLSeconds > 0 {
		ttl = time.Duration(settings.DNSTTLSeconds) * time.Second
	}
	if target != nil {
		res.Provider, res.Zone = target.Provider, target.Zone
		if dnsrecords.RelativeName(domain, target.Zone) != "@" {
			writeError(w, http.StatusBadRequest, "the www plan applies to a zone apex domain; "+domain+" is a subdomain of "+strings.TrimSuffix(target.Zone, "."))
			return
		}
		if target.Provider == dnsProviderCloudflare && settings.DNSTTLSeconds == 0 {
			ttl = dnsAutoTTLCloudflare
		}
		if existing, err = target.Manager.GetRecords(ctx, target.Zone); err != nil {
			rt.logger.Error("api: www plan: list records failed", slog.String("error", err.Error()), slog.String("domain", domain))
			writeError(w, http.StatusBadGateway, "could not read the zone's records")
			return
		}
	}
	typ, val, err := rt.serverDNSTarget(ctx, settings)
	if err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	items, err := dnsrecords.PlanApexWWW(domain, mode, dnsrecords.Desired{Type: typ, Value: val}, existing, false, ttl)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	for _, it := range items {
		rec := wwwPlanRecord{Name: it.Desired.Name, Type: it.Desired.Type, Value: it.Desired.Value,
			TTLSeconds: int(it.Desired.TTL.Seconds()), Outcome: it.Plan.Outcome, Replaces: []dnsRecordResource{}}
		for _, rr := range it.Plan.Replace {
			rec.Replaces = append(rec.Replaces, rrResource(rr))
		}
		res.Records = append(res.Records, rec)
	}
	if from, to := dnsrecords.CanonicalRedirect(domain, mode, canonical); from != "" {
		res.Redirect = &wwwPlanRedirect{From: from, To: to}
	}
	writeJSON(w, http.StatusOK, res)
}
