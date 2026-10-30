package api

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/dnsrecords"
	"github.com/GLINCKER/levelrail/internal/dnsstatus"
	"github.com/GLINCKER/levelrail/internal/dnszones"
	"github.com/GLINCKER/levelrail/internal/ingress"
	"github.com/GLINCKER/levelrail/internal/mailrecords"
)

// dnsStatusResponse is the body of GET /api/v1/domains/dns-status and /api/v1/mail/dns/status.
type dnsStatusResponse struct {
	Domain string `json:"domain"`
	App    string `json:"app,omitempty"`
	dnsstatus.Status
}

// mailDNSCheckResponse is the live grade plus each wanted record's status.
type mailDNSCheckResponse struct {
	mailrecords.Report
	dnsstatus.Status
}

func (rt *Router) dnsStatusResolver() dnsstatus.Resolver {
	if rt.mail.statusResolver != nil {
		return rt.mail.statusResolver
	}
	return net.DefaultResolver
}

// statusZone returns the connected provider and zone for domain, only when the
// caller may see that zone, so zone contents never cross teams.
func (rt *Router) statusZone(r *http.Request, domain string) (dnszones.Provider, dnszones.Zone, bool) {
	ctx := r.Context()
	det, err := rt.detectDNSHost(ctx, domain)
	if err != nil {
		return nil, dnszones.Zone{}, false
	}
	if ok, err := rt.callerSeesZone(r, det.Zone); err != nil || !ok {
		return nil, dnszones.Zone{}, false
	}
	name := rt.detectedZoneProvider(ctx, domain)
	if name == "" {
		return nil, dnszones.Zone{}, false
	}
	all, err := rt.zoneProviders(ctx)
	if err != nil || all[name] == nil {
		return nil, dnszones.Zone{}, false
	}
	zones, err := all[name].ListZones(ctx)
	if err != nil {
		return nil, dnszones.Zone{}, false
	}
	z, err := dnszones.FindZone(zones, det.Zone)
	if err != nil {
		return nil, dnszones.Zone{}, false
	}
	return all[name], z, true
}

// liveDNSStatus grades wants from the provider zone when visible, else from public DNS.
func (rt *Router) liveDNSStatus(r *http.Request, domain string, wants []dnsstatus.Want, shadow map[string]bool) dnsstatus.Status {
	opts := dnsstatus.OptionsFromEnv()
	p, z, viaProvider := rt.statusZone(r, domain)
	src := dnsstatus.SourcePublicDNS
	if viaProvider {
		src = dnsstatus.SourceProvider
	}
	// Detached from the request: a coalesced call must not die with its first caller.
	ctx := context.WithoutCancel(r.Context())
	return rt.mail.statusCache.Do(dnsstatus.Key(wants, domain, src, z.ID), dnsstatus.CacheTTL(), func() dnsstatus.Status {
		if viaProvider {
			pctx, cancel := context.WithTimeout(ctx, opts.ProviderTimeout)
			sets, err := p.ListRecordSets(pctx, z)
			cancel()
			if err == nil {
				return dnsstatus.FromZone(wants, mailrecords.ZoneSets(sets, z.Name), dnsstatus.ZoneOptions{Shadow: shadow, ProxyAware: p.Capabilities().Proxied}, time.Now())
			}
			rt.logger.Warn("api: dns status: provider listing failed, using public DNS", slog.String("domain", domain), slog.String("error", err.Error()))
		}
		return dnsstatus.FromResolver(ctx, rt.dnsStatusResolver(), wants, opts, time.Now())
	})
}

// mailDNSStatus grades the built mail records; demo mode reads the fake zone.
func (rt *Router) mailDNSStatus(r *http.Request, domain string, desired []mailrecords.Record) dnsstatus.Status {
	wants, shadow := mailrecords.Wants(desired, domain), mailrecords.ShadowNames(desired, domain)
	if d := rt.mail.demo; d != nil {
		sets, _ := d.zone.ListRecordSets(r.Context(), d.zone.ZoneFor(domain))
		return dnsstatus.FromZone(wants, mailrecords.ZoneSets(sets, domain), dnsstatus.ZoneOptions{Shadow: shadow}, time.Now())
	}
	return rt.liveDNSStatus(r, domain, wants, shadow)
}

// mailSpecFromQuery reads the mail record spec from query parameters named like the plan body.
func mailSpecFromQuery(q url.Values) (mailrecords.Spec, error) {
	s := mailrecords.Spec{
		Domain: dnszones.NormalizeDomain(q.Get("domain")), MailHost: strings.ToLower(strings.TrimSpace(q.Get("mail_host"))),
		IPv4: q.Get("ipv4"), IPv6: q.Get("ipv6"), Preset: q.Get("preset"), SPFMode: q.Get("spf_mode"),
		DKIMSelector: q.Get("dkim_selector"), DKIMPublicKey: q.Get("dkim_public_key"),
		DMARCPolicy: q.Get("dmarc_policy"), DMARCRua: q.Get("dmarc_rua"), MTASTSMode: q.Get("mta_sts_mode"),
		TLSRPT: q.Get("tls_rpt") == "true", Autoconfig: q.Get("autoconfig") == "true",
	}
	if v := q.Get("spf_includes"); v != "" {
		s.SPFIncludes = strings.Split(v, ",")
	}
	if v := q.Get("mx_priority"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return s, errors.New("mx_priority must be a number")
		}
		s.MXPriority = n
	}
	return s, nil
}

// handleMailDNSStatus handles GET /api/v1/mail/dns/status?domain=: each mail record's live state.
func (rt *Router) handleMailDNSStatus(w http.ResponseWriter, r *http.Request) {
	spec, err := mailSpecFromQuery(r.URL.Query())
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	rt.mailDemoFillSpec(&spec)
	desired, _, err := mailrecords.Build(spec)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, dnsStatusResponse{Domain: spec.Domain, Status: rt.mailDNSStatus(r, spec.Domain, desired)})
}

// appDomainStatus grades the record that points domain at this server.
func (rt *Router) appDomainStatus(r *http.Request, domain string) (dnsstatus.Status, error) {
	s, err := rt.ingressSettings.GetIngressSettings(r.Context())
	if err != nil {
		return dnsstatus.Status{}, err
	}
	typ, val, err := rt.serverDNSTarget(r.Context(), s)
	if err != nil {
		return dnsstatus.Status{}, err
	}
	ttl := s.DNSTTLSeconds
	if ttl <= 0 {
		ttl = int(dnsAutoTTLDefault.Seconds())
	}
	want := dnsstatus.Want{ID: "app-" + strings.ToLower(typ), Type: typ, Name: domain, Value: strings.TrimSuffix(val, "."), TTL: ttl}
	return rt.liveDNSStatus(r, domain, []dnsstatus.Want{want}, nil), nil
}

// handleDomainDNSStatus handles GET /api/v1/domains/dns-status?domain=&app=: the app record's live state.
func (rt *Router) handleDomainDNSStatus(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	domain := dnszones.NormalizeDomain(q.Get("domain"))
	if mailrecords.ValidHost(strings.TrimPrefix(domain, "*.")) != nil || ingress.ValidateWildcardDomain(domain) != nil {
		writeError(w, http.StatusBadRequest, "domain must be a domain name such as app.example.com")
		return
	}
	app := strings.TrimSpace(q.Get("app"))
	if app != "" {
		canSee, err := rt.appVisibilityFilter(r)
		if err != nil {
			rt.internalError(w, "api: dns status: visibility", err)
			return
		}
		if !canSee(app) {
			writeError(w, http.StatusNotFound, "app not found")
			return
		}
	}
	st, err := rt.appDomainStatus(r, domain)
	switch {
	case errors.Is(err, dnsrecords.ErrNoTarget):
		writeError(w, http.StatusConflict, err.Error())
		return
	case err != nil:
		rt.internalError(w, "api: dns status: required records", err)
		return
	}
	writeJSON(w, http.StatusOK, dnsStatusResponse{Domain: domain, App: app, Status: st})
}
