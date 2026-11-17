package api

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/dnszones"
	"github.com/GLINCKER/levelrail/internal/mailrecords"
	"github.com/GLINCKER/levelrail/internal/runtimesettings"
)

// dnsAuthorityDeps lets tests replace the live resolver and the local address list.
type dnsAuthorityDeps struct {
	resolver dnszones.AuthorityResolver
	localIPs func(ctx context.Context) []netip.Addr
}

// internalNameSuffixes are names that only make sense on a private network, so they are never looked up.
var internalNameSuffixes = []string{".localhost", ".local", ".internal", ".lan", ".home.arpa", ".intranet", ".corp"}

// validateAuthorityDomain accepts only public-looking host names: no IP literals,
// no single-label or internal-suffix names, so the lookup cannot probe the private network.
func validateAuthorityDomain(raw string) (string, error) {
	d := dnszones.NormalizeDomain(raw)
	if d == "" {
		return "", errors.New("domain is required")
	}
	if _, err := netip.ParseAddr(d); err == nil {
		return "", errors.New("domain must be a name, not an IP address")
	}
	if err := mailrecords.ValidHost(d); err != nil {
		return "", errors.New("domain must be a domain name such as app.example.com")
	}
	for _, s := range internalNameSuffixes {
		if strings.HasSuffix(d, s) {
			return "", errors.New("internal names are not looked up")
		}
	}
	return d, nil
}

func privateAnswers(answers []string) bool {
	if len(answers) == 0 {
		return false
	}
	for _, a := range answers {
		ip, err := netip.ParseAddr(a)
		if err != nil {
			return false
		}
		ip = ip.Unmap()
		if !ip.IsPrivate() && !ip.IsLoopback() && !ip.IsLinkLocalUnicast() && !ip.IsUnspecified() {
			return false
		}
	}
	return true
}

func interfaceAddrs() []netip.Addr {
	var out []netip.Addr
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return nil
	}
	for _, a := range addrs {
		if p, err := netip.ParsePrefix(a.String()); err == nil && !p.Addr().IsLoopback() {
			out = append(out, p.Addr().Unmap())
		}
	}
	return out
}

func (rt *Router) authorityLocalIPs(ctx context.Context) []netip.Addr {
	if rt.dnsAuthority.localIPs != nil {
		return rt.dnsAuthority.localIPs(ctx)
	}
	out := interfaceAddrs()
	for _, s := range rt.serverPublicIPs(ctx) {
		if ip, err := netip.ParseAddr(s); err == nil {
			out = append(out, ip.Unmap())
		}
	}
	return out
}

// handleDNSAuthority handles GET /api/v1/dns/authority?domain=: who answers DNS for
// a domain and the failover risks that follow. Read only, public names only.
func (rt *Router) handleDNSAuthority(w http.ResponseWriter, r *http.Request) {
	domain, err := validateAuthorityDomain(r.URL.Query().Get("domain"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	var res dnszones.AuthorityResolver = dnszones.NetAuthorityResolver{}
	if rt.dnsAuthority.resolver != nil {
		res = rt.dnsAuthority.resolver
	}
	var warn time.Duration
	if rt.runtimeSettings != nil {
		warn = time.Duration(rt.runtimeSettings.Int(r.Context(), runtimesettings.KeyDNSAuthorityWarnTTL)) * time.Second
	}
	ctx, cancel := context.WithTimeout(r.Context(), 3*domainCheckLookupTimeout)
	defer cancel()
	out, err := dnszones.CheckAuthority(ctx, domain, dnszones.AuthorityOptions{
		Resolver: res, LocalIPs: rt.authorityLocalIPs(ctx), WarnTTL: warn,
	})
	if err != nil {
		if errors.Is(err, dnszones.ErrAuthorityLookup) {
			writeError(w, http.StatusBadGateway, "could not look up "+domain+": the lookup failed or timed out")
			return
		}
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if privateAnswers(out.Answers) {
		writeError(w, http.StatusBadRequest, "internal names are not looked up")
		return
	}
	writeJSON(w, http.StatusOK, out)
}
