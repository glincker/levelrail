package api

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/internal/dnszones"
	"github.com/GLINCKER/levelrail/internal/mailrecords"
)

const envDNSProviderCacheSeconds = "APP_MAIL_DNS_PROVIDER_CACHE_SECONDS"

// hostProviders maps a detected DNS host to the zone provider we can connect for it.
var hostProviders = map[string]string{
	mailrecords.HostCloudflare:  dnszones.ProviderCloudflare,
	mailrecords.HostRoute53:     dnszones.ProviderRoute53,
	mailrecords.HostGoogleCloud: dnszones.ProviderGoogleCloud,
}

var hostLabels = map[string]string{
	mailrecords.HostCloudflare: "Cloudflare", mailrecords.HostRoute53: "Amazon Route 53", mailrecords.HostGoogleCloud: "Google Cloud DNS",
	mailrecords.HostDigitalOcean: "DigitalOcean", mailrecords.HostAzure: "Azure DNS", mailrecords.HostHetzner: "Hetzner DNS",
	mailrecords.HostNamecheap: "Namecheap", mailrecords.HostGoDaddy: "GoDaddy",
}

var cfAccountID = regexp.MustCompile(`^[0-9a-f]{32}$`)

// dnsProviderResponse is GET /api/v1/domains/dns-provider and GET /api/v1/mail/dns/provider.
type dnsProviderResponse struct {
	mailrecords.Detection
	Connected            bool   `json:"connected"`
	Connectable          bool   `json:"connectable"`
	ZoneID               string `json:"zone_id,omitempty"`
	DashboardURL         string `json:"dashboard_url,omitempty"`
	CanIssueCertificates bool   `json:"can_issue_certificates"`
	Hint                 string `json:"hint"`
}

type nsCacheEntry struct {
	det mailrecords.Detection
	err error
	at  time.Time
}

// mailNSCache keeps public NS detections for a short env TTL; nothing refreshes it in the background.
type mailNSCache struct {
	mu sync.Mutex
	m  map[string]nsCacheEntry
}

func dnsProviderCacheTTL() time.Duration {
	return time.Duration(envIntDefault(envDNSProviderCacheSeconds, 300)) * time.Second
}

func (c *mailNSCache) get(domain string) (nsCacheEntry, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.m[domain]
	if !ok || time.Since(e.at) > dnsProviderCacheTTL() {
		return nsCacheEntry{}, false
	}
	return e, true
}

func (c *mailNSCache) put(domain string, e nsCacheEntry) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.m == nil {
		c.m = map[string]nsCacheEntry{}
	}
	if len(c.m) > 4096 {
		clear(c.m)
	}
	c.m[domain] = e
}

func (rt *Router) nsResolver() mailrecords.NSResolver {
	if rt.mail.nsResolver != nil {
		return rt.mail.nsResolver
	}
	return net.DefaultResolver
}

// detectDNSHost runs (or reuses) the public NS and Domain Connect lookups for a domain.
func (rt *Router) detectDNSHost(ctx context.Context, domain string) (mailrecords.Detection, error) {
	domain = dnszones.NormalizeDomain(domain)
	if e, ok := rt.mail.nsCache.get(domain); ok {
		return e.det, e.err
	}
	if _, err := mailrecords.RegistrableApex(domain); err != nil {
		return mailrecords.Detection{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, 5*domainCheckLookupTimeout)
	defer cancel()
	det, err := mailrecords.Detect(ctx, rt.nsResolver(), domain)
	rt.mail.nsCache.put(domain, nsCacheEntry{det: det, err: err, at: time.Now()})
	return det, err
}

// errBadDomain marks input that is not a registrable public domain.
var errBadDomain = errors.New("bad domain")

// dnsProviderReport classifies the host and, when reveal is true, reports whether a stored credential sees the zone.
func (rt *Router) dnsProviderReport(ctx context.Context, domain string, reveal bool) (dnsProviderResponse, error) {
	if _, err := mailrecords.RegistrableApex(dnszones.NormalizeDomain(domain)); err != nil {
		return dnsProviderResponse{}, fmt.Errorf("%w: %w", errBadDomain, err)
	}
	det, err := rt.detectDNSHost(ctx, domain)
	out := dnsProviderResponse{Detection: det}
	if err != nil {
		out.Hint = "No nameservers answer for this domain yet. Check the nameservers at your registrar."
		return out, nil
	}
	label := hostLabels[det.Provider]
	if label == "" {
		label = strings.Join(det.Nameservers, ", ")
	}
	name, ok := hostProviders[det.Provider]
	out.Connectable = ok
	switch {
	case !ok:
		out.Hint = "DNS for " + det.Zone + " is hosted at " + label + ". Add the records there by hand; this host cannot be connected yet."
		return out, nil
	case !reveal:
		out.Hint = "DNS for " + det.Zone + " is hosted at " + label + ". Ask an instance admin whether it is connected."
		return out, nil
	}
	providers, err := rt.zoneProviders(ctx)
	if err != nil {
		return out, fmt.Errorf("resolve dns zone providers: %w", err)
	}
	p := providers[name]
	if p == nil {
		out.Hint = "DNS for " + det.Zone + " is hosted at " + label + ". Connect a " + label + " credential under DNS settings to add the records in one step."
		return out, nil
	}
	zones, err := p.ListZones(ctx)
	if err != nil {
		out.Hint = "The connected " + label + " credential could not list zones; check that it is still valid."
		return out, nil
	}
	z, err := dnszones.FindZone(zones, det.Zone)
	if err != nil {
		out.Hint = "The connected " + label + " credential cannot see the zone " + det.Zone + ". Give it access to this zone (DNS edit and zone read), then check again."
		return out, nil
	}
	out.Connected, out.ZoneID = true, z.ID
	out.CanIssueCertificates = name == dnszones.ProviderCloudflare || name == dnszones.ProviderRoute53
	if name == dnszones.ProviderCloudflare && cfAccountID.MatchString(z.AccountID) {
		out.DashboardURL = "https://dash.cloudflare.com/" + z.AccountID + "/" + url.PathEscape(z.Name) + "/dns/records"
	}
	out.Hint = "DNS for " + det.Zone + " is at " + label + " and the connected credential manages it. Records can be added in one step."
	return out, nil
}

// inZone reports whether name is the zone or a name under it.
func inZone(name, zone string) bool {
	name, zone = dnszones.NormalizeDomain(name), dnszones.NormalizeDomain(zone)
	return zone != "" && (name == zone || strings.HasSuffix(name, "."+zone))
}

// callerSeesZone is true for unscoped callers, and for team callers only when an
// app or mail domain they can read sits in the zone, so zone ids never cross teams.
func (rt *Router) callerSeesZone(r *http.Request, zone string) (bool, error) {
	canRead, filtered, err := rt.callerAppVisibility(r)
	if err != nil {
		return false, err
	}
	if !filtered {
		return true, nil
	}
	if rt.domains != nil {
		ds, err := rt.domains.ListServiceDomains(r.Context())
		if err != nil {
			return false, err
		}
		for _, d := range ds {
			if canRead(d.ServiceName) && inZone(d.Domain, zone) {
				return true, nil
			}
		}
	}
	if rt.mail.boxes != nil {
		md, err := rt.visibleMailDomains(r, rt.mail.boxes)
		if err != nil {
			return false, err
		}
		for _, d := range md {
			if inZone(d.Domain, zone) {
				return true, nil
			}
		}
	}
	return false, nil
}

// serveDNSProvider is shared by the general domains route and the mail route.
func (rt *Router) serveDNSProvider(w http.ResponseWriter, r *http.Request) {
	domain := dnszones.NormalizeDomain(r.URL.Query().Get("domain"))
	reveal := false
	if det, err := rt.detectDNSHost(r.Context(), domain); err == nil {
		ok, err := rt.callerSeesZone(r, det.Zone)
		if err != nil {
			rt.internalError(w, "api: dns provider: resolve caller visibility", err)
			return
		}
		reveal = ok
	}
	out, err := rt.dnsProviderReport(r.Context(), domain, reveal)
	switch {
	case errors.Is(err, errBadDomain):
		writeError(w, http.StatusBadRequest, "domain must be a public domain name such as example.com")
		return
	case err != nil:
		rt.internalError(w, "api: dns provider detection failed", err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// handleDomainDNSProvider handles GET /api/v1/domains/dns-provider?domain=: where the domain's DNS lives.
func (rt *Router) handleDomainDNSProvider(w http.ResponseWriter, r *http.Request) {
	rt.serveDNSProvider(w, r)
}

// handleMailDNSProvider handles GET /api/v1/mail/dns/provider?domain=: the same detection for the mail panel.
func (rt *Router) handleMailDNSProvider(w http.ResponseWriter, r *http.Request) {
	if rt.mail.demo != nil {
		writeJSON(w, http.StatusOK, mailDemoProvider(r.URL.Query().Get("domain"), rt.mail.demo.zone))
		return
	}
	rt.serveDNSProvider(w, r)
}

// detectedZoneProvider returns the connected provider name detection points at, or "".
func (rt *Router) detectedZoneProvider(ctx context.Context, domain string) string {
	det, err := rt.detectDNSHost(ctx, domain)
	if err != nil {
		return ""
	}
	name, ok := hostProviders[det.Provider]
	if !ok {
		return ""
	}
	all, err := rt.zoneProviders(ctx)
	if err != nil || all[name] == nil {
		return ""
	}
	return name
}
