package api

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/dnszones"
	"github.com/GLINCKER/levelrail/internal/store"
)

// dnsZoneProvidersFunc returns every configured zone provider, keyed by name.
type dnsZoneProvidersFunc func(ctx context.Context) (map[string]dnszones.Provider, error)

// dnsZoneAuditPrefix anchors every dns_zone.* and dns_record.* audit row's path.
const dnsZoneAuditPrefix = "/api/v1/dns/zones/"

// Audit actions for zone and record writes.
const (
	auditDNSZoneCreate     = "dns_zone.create"
	auditDNSZoneDelete     = "dns_zone.delete"
	auditDNSRecordCreate   = "dns_record.create"
	auditDNSRecordUpdate   = "dns_record.update"
	auditDNSRecordDelete   = "dns_record.delete"
	auditDNSRecordImport   = "dns_record.import"
	auditDNSRecordTemplate = "dns_record.template"
	auditDNSHealthCreate   = "dns_health_check.create"
	auditDNSHealthDelete   = "dns_health_check.delete"
)

const dnsNoProviderMessage = "no DNS provider is connected: connect Cloudflare or Route53 under Domains, DNS provider"

// dnsDefaultTTL is APP_DNS_DEFAULT_TTL, else 300 seconds.
func dnsDefaultTTL() int {
	if v, err := strconv.Atoi(os.Getenv("APP_DNS_DEFAULT_TTL")); err == nil && v > 0 {
		return v
	}
	return 300
}

func (rt *Router) querier() dnszones.Querier {
	if rt.dnsQuerier != nil {
		return rt.dnsQuerier
	}
	return dnszones.NewDNSQuerier(domainCheckLookupTimeout)
}

// dnsCheckServers is system plus the public resolvers, unless
// APP_DNS_PUBLIC_RESOLVERS turns the public ones off.
func dnsCheckServers() []string {
	if v := strings.ToLower(os.Getenv("APP_DNS_PUBLIC_RESOLVERS")); v == "off" || v == "false" || v == "0" {
		return []string{dnszones.ServerSystem}
	}
	return dnszones.DefaultCheckServers
}

func (rt *Router) zoneProviders(ctx context.Context) (map[string]dnszones.Provider, error) {
	if rt.dnsZoneProviders != nil {
		return rt.dnsZoneProviders(ctx)
	}
	return rt.resolveDNSZoneProviders(ctx)
}

// resolveDNSZoneProviders builds providers from the DNS-01 credentials
// already stored for Cloudflare and Route53.
func (rt *Router) resolveDNSZoneProviders(ctx context.Context) (map[string]dnszones.Provider, error) {
	out := map[string]dnszones.Provider{}
	if rt.cloudflareDNSTokenResolver != nil && rt.cloudflareDNS != nil {
		s, err := rt.cloudflareDNS.GetCloudflareDNSSettings(ctx)
		if err != nil {
			return nil, fmt.Errorf("api: get cloudflare dns settings: %w", err)
		}
		if s.Enabled {
			token, err := rt.cloudflareDNSTokenResolver.Resolve(ctx, store.CloudflareDNSSecretsKey(), store.CloudflareDNSTokenEnvKey)
			if err != nil {
				return nil, fmt.Errorf("api: resolve cloudflare dns token: %w", err)
			}
			if token != "" {
				out[dnszones.ProviderCloudflare] = dnszones.NewCloudflare(token)
			}
		}
	}
	if rt.route53DNSCredentialResolver != nil && rt.route53DNS != nil {
		s, err := rt.route53DNS.GetRoute53DNSSettings(ctx)
		if err != nil {
			return nil, fmt.Errorf("api: get route53 dns settings: %w", err)
		}
		if s.Enabled {
			key := store.Route53DNSSecretsKey()
			id, err1 := rt.route53DNSCredentialResolver.Resolve(ctx, key, store.Route53DNSAccessKeyIDEnvKey)
			secret, err2 := rt.route53DNSCredentialResolver.Resolve(ctx, key, store.Route53DNSSecretAccessKeyEnvKey)
			if err := errors.Join(err1, err2); err != nil {
				return nil, fmt.Errorf("api: resolve route53 credentials: %w", err)
			}
			if id != "" && secret != "" {
				out[dnszones.ProviderRoute53] = dnszones.NewRoute53(id, secret, s.Region, s.HostedZoneID)
			}
		}
	}
	return out, nil
}

func providerNames(m map[string]dnszones.Provider) []string {
	names := make([]string, 0, len(m))
	for _, n := range []string{dnszones.ProviderCloudflare, dnszones.ProviderRoute53} {
		if _, ok := m[n]; ok {
			names = append(names, n)
		}
	}
	return names
}

// pickProvider selects ?provider= or the first configured one (Cloudflare first,
// matching the DNS-01 tie break). It writes the error response itself.
func (rt *Router) pickProvider(w http.ResponseWriter, r *http.Request) (dnszones.Provider, []string, bool) {
	all, err := rt.zoneProviders(r.Context())
	if err != nil {
		rt.internalError(w, "api: resolve dns zone providers failed", err)
		return nil, nil, false
	}
	names := providerNames(all)
	if want := r.URL.Query().Get("provider"); want != "" {
		p, ok := all[want]
		if !ok {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("provider %q is not connected (connected: %s)", want, strings.Join(names, ", ")))
			return nil, names, false
		}
		return p, names, true
	}
	if len(names) == 0 {
		return nil, names, true
	}
	return all[names[0]], names, true
}

// requireZone resolves {zone} (id or name) at the chosen provider.
func (rt *Router) requireZone(w http.ResponseWriter, r *http.Request) (dnszones.Provider, dnszones.Zone, bool) {
	p, _, ok := rt.pickProvider(w, r)
	if !ok {
		return nil, dnszones.Zone{}, false
	}
	if p == nil {
		writeError(w, http.StatusNotImplemented, dnsNoProviderMessage)
		return nil, dnszones.Zone{}, false
	}
	zones, err := p.ListZones(r.Context())
	if err != nil {
		rt.dnsProviderError(w, "list zones", err)
		return nil, dnszones.Zone{}, false
	}
	z, err := dnszones.FindZone(zones, r.PathValue("zone"))
	if err != nil {
		writeError(w, http.StatusNotFound, fmt.Sprintf("zone %q not found at %s", r.PathValue("zone"), p.Name()))
		return nil, dnszones.Zone{}, false
	}
	return p, z, true
}

// dnsProviderError maps an upstream failure to 502 with the provider's own
// message, which never contains the credential.
func (rt *Router) dnsProviderError(w http.ResponseWriter, op string, err error) {
	rt.logger.Warn("api: dns provider call failed", slog.String("op", op), slog.String("error", err.Error()))
	writeError(w, http.StatusBadGateway, fmt.Sprintf("DNS provider rejected %s: %v", op, err))
}

// auditDNS writes a named audit row for a zone or record write.
func (rt *Router) auditDNS(r *http.Request, action string, zone dnszones.Zone, subject string, status int) {
	a := rt.accessActor(r)
	id, err := store.NewAuditEntryID()
	if err != nil {
		rt.logger.Warn("api: dns audit id failed", slog.String("error", err.Error()))
		return
	}
	path := dnsZoneAuditPrefix + zone.ID
	if subject != "" {
		path += "#" + subject
	}
	entry := store.AuditEntry{
		ID: id, ActorType: a.kind, ActorID: a.id, ActorName: a.name,
		Ability: AbilityRoot, Method: r.Method, Path: path, StatusCode: status,
		RemoteAddr: clientIP(r), CreatedAt: store.FormatAuditTime(time.Now()),
		ClientKind: clientKindFromUserAgent(r.Header.Get("User-Agent")), Action: action,
	}
	if err := rt.auditLog.SaveAuditEntry(r.Context(), entry); err != nil {
		rt.logger.Warn("api: dns audit save failed", slog.String("error", err.Error()), slog.String("zone", zone.Name))
	}
}

type dnsZonesResponse struct {
	Provider     string                `json:"provider"`
	Providers    []string              `json:"providers"`
	Capabilities dnszones.Capabilities `json:"capabilities"`
	Zones        []dnszones.Zone       `json:"zones"`
}

// handleListDNSZones handles GET /api/v1/dns/zones. No provider is a 200 with
// an empty list, so the dashboard can render its connect empty state.
func (rt *Router) handleListDNSZones(w http.ResponseWriter, r *http.Request) {
	p, names, ok := rt.pickProvider(w, r)
	if !ok {
		return
	}
	resp := dnsZonesResponse{Provider: "none", Providers: names, Zones: []dnszones.Zone{}}
	if p == nil {
		writeJSON(w, http.StatusOK, resp)
		return
	}
	zones, err := p.ListZones(r.Context())
	if err != nil {
		rt.dnsProviderError(w, "list zones", err)
		return
	}
	slices.SortFunc(zones, func(a, b dnszones.Zone) int { return strings.Compare(a.Name, b.Name) })
	resp.Provider, resp.Capabilities, resp.Zones = p.Name(), p.Capabilities(), zones
	writeJSON(w, http.StatusOK, resp)
}

type createDNSZoneRequest struct {
	Name      string `json:"name"`
	AccountID string `json:"account_id,omitempty"`
}

// handleCreateDNSZone handles POST /api/v1/dns/zones.
func (rt *Router) handleCreateDNSZone(w http.ResponseWriter, r *http.Request) {
	var req createDNSZoneRequest
	if err := decodeJSONBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	name := dnszones.NormalizeDomain(req.Name)
	if err := validateZoneName(name); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	p, _, ok := rt.pickProvider(w, r)
	if !ok {
		return
	}
	if p == nil {
		writeError(w, http.StatusNotImplemented, dnsNoProviderMessage)
		return
	}
	if cf, isCF := p.(*dnszones.Cloudflare); isCF && req.AccountID != "" {
		cf.AccountID = req.AccountID
	}
	if zones, err := p.ListZones(r.Context()); err == nil {
		if _, ferr := dnszones.FindZone(zones, name); ferr == nil {
			writeError(w, http.StatusConflict, fmt.Sprintf("zone %s already exists at %s", name, p.Name()))
			return
		}
	}
	z, err := p.CreateZone(r.Context(), name)
	if err != nil {
		rt.dnsProviderError(w, "create zone", err)
		return
	}
	rt.auditDNS(r, auditDNSZoneCreate, z, z.Name, http.StatusCreated)
	writeJSON(w, http.StatusCreated, z)
}

// validateZoneName accepts a registrable looking domain: two or more labels,
// no wildcard, no single label TLD.
func validateZoneName(name string) error {
	labels := strings.Split(name, ".")
	if len(labels) < 2 {
		return errors.New("zone name must be a domain such as example.com")
	}
	for _, l := range labels {
		if l == "" || len(l) > 63 || strings.ContainsAny(l, "*_ /") {
			return fmt.Errorf("zone name %q is not a valid domain", name)
		}
	}
	return nil
}

type dnsZoneOverview struct {
	Zone          dnszones.Zone         `json:"zone"`
	Capabilities  dnszones.Capabilities `json:"capabilities"`
	Counts        map[string]int        `json:"counts"`
	Total         int                   `json:"total"`
	LastChanged   string                `json:"last_changed,omitempty"`
	LastChangedBy string                `json:"last_changed_by,omitempty"`
	LastAction    string                `json:"last_action,omitempty"`
}

// handleGetDNSZone handles GET /api/v1/dns/zones/{zone}: counts by type and
// the last change made through this control plane.
func (rt *Router) handleGetDNSZone(w http.ResponseWriter, r *http.Request) {
	p, z, ok := rt.requireZone(w, r)
	if !ok {
		return
	}
	if full, err := p.GetZone(r.Context(), z.ID); err == nil {
		z.NameServers, z.Status = full.NameServers, full.Status
	}
	sets, err := p.ListRecordSets(r.Context(), z)
	if err != nil {
		rt.dnsProviderError(w, "list records", err)
		return
	}
	ov := dnsZoneOverview{Zone: z, Capabilities: p.Capabilities(), Counts: map[string]int{}}
	for _, s := range sets {
		ov.Counts[s.Type]++
		ov.Total++
	}
	if rt.auditLog != nil {
		entries, err := rt.auditLog.ListAuditEntries(r.Context(), 1, nil, store.AuditEntryFilter{Search: dnsZoneAuditPrefix + z.ID})
		if err == nil && len(entries) > 0 {
			ov.LastChanged, ov.LastChangedBy, ov.LastAction = entries[0].CreatedAt, entries[0].ActorName, entries[0].Action
		}
	}
	if ov.LastChanged == "" {
		ov.LastChanged = z.ModifiedAt
	}
	writeJSON(w, http.StatusOK, ov)
}

type deleteDNSZoneRequest struct {
	Confirm string `json:"confirm"`
	Force   bool   `json:"force,omitempty"`
}

// handleDeleteDNSZone handles DELETE /api/v1/dns/zones/{zone}. The body must
// repeat the zone name, and a zone with records beyond NS and SOA needs force.
func (rt *Router) handleDeleteDNSZone(w http.ResponseWriter, r *http.Request) {
	var req deleteDNSZoneRequest
	if err := decodeJSONBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	p, z, ok := rt.requireZone(w, r)
	if !ok {
		return
	}
	if dnszones.NormalizeDomain(req.Confirm) != z.Name {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("type the zone name %q in confirm to delete it", z.Name))
		return
	}
	sets, err := p.ListRecordSets(r.Context(), z)
	if err != nil {
		rt.dnsProviderError(w, "list records", err)
		return
	}
	var user []dnszones.RecordSet
	for _, s := range sets {
		if !dnszones.IsManagedApexType(s) {
			user = append(user, s)
		}
	}
	if len(user) > 0 && !req.Force {
		writeError(w, http.StatusConflict, fmt.Sprintf("zone %s still has %d record sets beyond NS and SOA; export them first, then delete with force", z.Name, len(user)))
		return
	}
	if p.Name() == dnszones.ProviderRoute53 {
		for _, s := range user {
			if err := p.DeleteRecordSet(r.Context(), z, s.Key()); err != nil {
				rt.dnsProviderError(w, "delete record set before zone", err)
				return
			}
		}
	}
	if err := p.DeleteZone(r.Context(), z.ID); err != nil {
		rt.dnsProviderError(w, "delete zone", err)
		return
	}
	rt.auditDNS(r, auditDNSZoneDelete, z, fmt.Sprintf("%s records=%d", z.Name, len(user)), http.StatusOK)
	writeJSON(w, http.StatusOK, map[string]any{"deleted": z.Name, "records_deleted": len(user)})
}

// handleDNSZoneNameServers handles GET /api/v1/dns/zones/{zone}/nameservers.
func (rt *Router) handleDNSZoneNameServers(w http.ResponseWriter, r *http.Request) {
	p, z, ok := rt.requireZone(w, r)
	if !ok {
		return
	}
	full, err := p.GetZone(r.Context(), z.ID)
	if err != nil {
		rt.dnsProviderError(w, "get zone", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"zone": full.Name, "provider": p.Name(), "name_servers": full.NameServers})
}

// handleDNSZoneDelegation handles GET /api/v1/dns/zones/{zone}/delegation:
// what the system resolver and public resolvers say the domain's NS set is,
// against the zone's assigned name servers.
func (rt *Router) handleDNSZoneDelegation(w http.ResponseWriter, r *http.Request) {
	p, z, ok := rt.requireZone(w, r)
	if !ok {
		return
	}
	full, err := p.GetZone(r.Context(), z.ID)
	if err != nil {
		rt.dnsProviderError(w, "get zone", err)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 2*domainCheckLookupTimeout)
	defer cancel()
	d := dnszones.CheckDelegation(ctx, rt.querier(), full.Name, full.NameServers, dnsCheckServers())
	writeJSON(w, http.StatusOK, map[string]any{"delegation": d, "checked_at": time.Now().UTC().Format(time.RFC3339)})
}
