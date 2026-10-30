package api

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/GLINCKER/levelrail/internal/dnszones"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/kit/netguard"
)

// registrarLookup is the RDAP registrar hint source; tests and demo mode swap it.
type registrarLookup interface {
	Registrar(ctx context.Context, domain string) (dnszones.RegistrarHint, bool)
}

// dnsNameserverState holds the lazily built delegation checker and registrar lookup.
type dnsNameserverState struct {
	checkerOnce sync.Once
	checker     *dnszones.NSChecker
	rdapOnce    sync.Once
	registrar   registrarLookup
	demoOnce    sync.Once
	demo        *demoZoneProvider
}

// dnsZoneView is one zone in GET /api/v1/dns/zones and the detail route.
type dnsZoneView struct {
	dnszones.Zone
	Nameservers        []string                `json:"nameservers"`
	Delegated          string                  `json:"delegated"`
	DelegationReason   string                  `json:"delegation_reason,omitempty"`
	CurrentNameservers []string                `json:"current_nameservers"`
	RegistrarHint      *dnszones.RegistrarHint `json:"registrar_hint,omitempty"`
}

type dnsZoneDetailResponse struct {
	dnsZoneView
	Capabilities dnszones.Capabilities `json:"capabilities"`
}

// publicNSServers are public resolvers first, the system resolver as fallback.
func publicNSServers() []string {
	all := dnsCheckServers()
	out := make([]string, 0, len(all))
	for _, s := range all {
		if s != dnszones.ServerSystem {
			out = append(out, s)
		}
	}
	return append(out, dnszones.ServerSystem)
}

func (rt *Router) nsChecker() *dnszones.NSChecker {
	rt.dnsNS.checkerOnce.Do(func() {
		q := rt.querier()
		if d := rt.demoDNSZones(); d != nil {
			q = d
		}
		rt.dnsNS.checker = dnszones.NewNSChecker(q, dnszones.NSCheckerOptionsFromEnv(publicNSServers()))
	})
	return rt.dnsNS.checker
}

func (rt *Router) registrarLookup() registrarLookup {
	rt.dnsNS.rdapOnce.Do(func() {
		if rt.dnsNS.registrar != nil {
			return
		}
		if d := rt.demoDNSZones(); d != nil {
			rt.dnsNS.registrar = d
			return
		}
		rt.dnsNS.registrar = dnszones.NewRDAP(netguard.NewClient(), dnszones.RDAPOptionsFromEnv())
	})
	return rt.dnsNS.registrar
}

func newZoneView(z dnszones.Zone, d dnszones.NSDelegation) dnsZoneView {
	return dnsZoneView{Zone: z, Nameservers: d.Expected, Delegated: d.Delegated, DelegationReason: d.Reason, CurrentNameservers: d.Current}
}

// zoneViews checks every zone's delegation; the checker bounds concurrency.
func (rt *Router) zoneViews(ctx context.Context, zones []dnszones.Zone) []dnsZoneView {
	out := make([]dnsZoneView, len(zones))
	var wg sync.WaitGroup
	c := rt.nsChecker()
	for i, z := range zones {
		wg.Add(1)
		go func() {
			defer wg.Done()
			out[i] = newZoneView(z, c.Check(ctx, z))
		}()
	}
	wg.Wait()
	return out
}

// visibleZones drops zones a team scoped caller has no app or mail domain in.
func (rt *Router) visibleZones(r *http.Request, zones []dnszones.Zone) ([]dnszones.Zone, error) {
	out := zones[:0:0]
	for _, z := range zones {
		ok, err := rt.callerSeesZone(r, z.Name)
		if err != nil {
			return nil, err
		}
		if ok {
			out = append(out, z)
		}
	}
	return out, nil
}

// listAllZones lists zones across providers, recording per provider failures instead of failing.
func (rt *Router) listAllZones(ctx context.Context, providers []dnszones.Provider) ([]dnszones.Zone, map[string]dnszones.Capabilities, map[string]string, error) {
	var zones []dnszones.Zone
	caps, errs := map[string]dnszones.Capabilities{}, map[string]string{}
	for _, p := range providers {
		zs, err := p.ListZones(ctx)
		if err != nil {
			if len(providers) == 1 {
				return nil, nil, nil, err
			}
			rt.logger.Warn("api: dns zones: provider listing failed", slog.String("provider", p.Name()), slog.String("error", err.Error()))
			errs[p.Name()] = "could not list zones; check the connected credential"
			continue
		}
		caps[p.Name()] = p.Capabilities()
		zones = append(zones, zs...)
	}
	slices.SortFunc(zones, func(a, b dnszones.Zone) int {
		if c := strings.Compare(a.Name, b.Name); c != 0 {
			return c
		}
		return strings.Compare(a.Provider, b.Provider)
	})
	return zones, caps, errs, nil
}

// handleGetDNSZoneDetail handles GET /api/v1/dns/zones/{provider}/{id}: one zone with
// its assigned nameservers, delegation and a best-effort registrar hint.
func (rt *Router) handleGetDNSZoneDetail(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	all, err := rt.zoneProviders(ctx)
	if err != nil {
		rt.internalError(w, "api: resolve dns zone providers failed", err)
		return
	}
	p := all[r.PathValue("provider")]
	if p == nil {
		writeError(w, http.StatusNotFound, fmt.Sprintf("provider %q is not connected", r.PathValue("provider")))
		return
	}
	zones, err := p.ListZones(ctx)
	if err != nil {
		rt.dnsProviderError(w, "list zones", err)
		return
	}
	z, err := dnszones.FindZone(zones, r.PathValue("id"))
	if err == nil {
		var ok bool
		if ok, err = rt.callerSeesZone(r, z.Name); err != nil {
			rt.internalError(w, "api: dns zone detail: resolve caller visibility", err)
			return
		} else if !ok {
			err = dnszones.ErrZoneNotFound
		}
	}
	if err != nil {
		writeError(w, http.StatusNotFound, fmt.Sprintf("zone %q not found at %s", r.PathValue("id"), p.Name()))
		return
	}
	if full, err := p.GetZone(ctx, z.ID); err == nil {
		z = full
	}
	v := newZoneView(z, rt.nsChecker().Check(ctx, z))
	if !z.Private {
		if h, ok := rt.registrarLookup().Registrar(ctx, z.Name); ok {
			v.RegistrarHint = &h
		}
	}
	writeJSON(w, http.StatusOK, dnsZoneDetailResponse{dnsZoneView: v, Capabilities: p.Capabilities()})
}

// findConnectedZone finds the zone holding domain at any connected provider.
func (rt *Router) findConnectedZone(ctx context.Context, domain string) (string, dnszones.Zone, bool) {
	all, err := rt.zoneProviders(ctx)
	if err != nil {
		return "", dnszones.Zone{}, false
	}
	for _, name := range providerNames(all) {
		zones, err := all[name].ListZones(ctx)
		if err != nil {
			continue
		}
		if z, err := dnszones.FindZone(zones, domain); err == nil {
			return name, z, true
		}
	}
	return "", dnszones.Zone{}, false
}

var zoneProviderLabels = map[string]string{
	dnszones.ProviderCloudflare: "Cloudflare", dnszones.ProviderRoute53: "Route 53", dnszones.ProviderGoogleCloud: "Google Cloud DNS",
}

// delegationReasonText explains, in plain words, why zone records are not live yet.
func delegationReasonText(provider string, d dnszones.NSDelegation) string {
	label := zoneProviderLabels[provider]
	if label == "" {
		label = provider
	}
	current := strings.Join(d.Current, ", ")
	switch d.Delegated {
	case dnszones.DelegatedNo:
		return "These records are in your " + label + " zone, but your domain's nameservers still point to " + current + ", so they are not live yet."
	case dnszones.DelegatedPartial:
		return "These records are in your " + label + " zone, but only some of your domain's nameservers (" + current + ") point to it, so some resolvers will not see them yet."
	}
	return ""
}

// domainDelegation classifies z against the public NS set detection found for domain.
func (rt *Router) domainDelegation(ctx context.Context, provider string, z dnszones.Zone, domain string) string {
	det, err := rt.detectDNSHost(ctx, domain)
	d := dnszones.ClassifyDelegation(z.Private, z.NameServers, det.Nameservers, err)
	return delegationReasonText(provider, d)
}

// visibleDelegationWarning is the warning for domain's connected zone, "" when
// delegated, unknown or not visible to the caller.
func (rt *Router) visibleDelegationWarning(r *http.Request, domain string) string {
	if rt.mail.demo != nil {
		return ""
	}
	p, z, ok := rt.statusZone(r, domain)
	if !ok {
		return ""
	}
	return rt.domainDelegation(r.Context(), p.Name(), z, domain)
}

// ledgerKey identifies a record name and type in the mail DNS ledger.
func ledgerKey(name, typ string) string { return strings.ToLower(name) + "/" + strings.ToUpper(typ) }

// mailLedgerKeys returns the zone records successful mail DNS applies wrote, from the audit ledger.
func (rt *Router) mailLedgerKeys(ctx context.Context, z dnszones.Zone) map[string]bool {
	out := map[string]bool{}
	if rt.auditLog == nil {
		return out
	}
	rows, err := rt.auditLog.ListAuditEntries(ctx, 200, nil, store.AuditEntryFilter{Action: auditMailDNSApply, Search: z.ID + "#mail_dns "})
	if err != nil {
		rt.logger.Warn("api: dns records: read mail ledger", slog.String("zone", z.ID), slog.String("error", err.Error()))
		return out
	}
	for _, row := range rows {
		if row.StatusCode != http.StatusOK || !strings.HasPrefix(row.Path, dnsZoneAuditPrefix+z.ID+"#") {
			continue
		}
		_, subject, _ := strings.Cut(row.Path, "#")
		f := strings.Fields(subject)
		if len(f) < 2 {
			continue
		}
		e, ok := parseMailLedger(row.Path, strings.TrimPrefix(f[1], "domain="))
		if !ok {
			continue
		}
		for _, a := range e.Records {
			if a.Action != "delete" {
				out[ledgerKey(dnszones.RelativeName(a.Name, z.Name), a.Type)] = true
			}
		}
	}
	return out
}

// pageRecords applies limit (APP_DNS_RECORDS_PAGE_LIMIT default, capped by
// APP_DNS_RECORDS_PAGE_MAX) and an opaque offset cursor.
func pageRecords(sets []dnszones.RecordSet, limitArg, cursorArg string) ([]dnszones.RecordSet, string, error) {
	limit, maxLimit := envIntDefault("APP_DNS_RECORDS_PAGE_LIMIT", 500), envIntDefault("APP_DNS_RECORDS_PAGE_MAX", 1000)
	if limitArg != "" {
		n, err := strconv.Atoi(limitArg)
		if err != nil || n <= 0 {
			return nil, "", errors.New("limit must be a positive number")
		}
		limit = n
	}
	limit = min(limit, maxLimit)
	start := 0
	if cursorArg != "" {
		n, err := strconv.Atoi(cursorArg)
		if err != nil || n < 0 || n > len(sets) {
			return nil, "", errors.New("cursor is not valid for this listing")
		}
		start = n
	}
	end := min(start+limit, len(sets))
	next := ""
	if end < len(sets) {
		next = strconv.Itoa(end)
	}
	return sets[start:end], next, nil
}
