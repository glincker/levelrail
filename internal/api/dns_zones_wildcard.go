package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/dnszones"
	"github.com/GLINCKER/levelrail/internal/ingress"
)

const wildcardNeedsDNS01 = "wildcard_needs_dns01"

type wildcardRejection struct {
	Error  string `json:"error"`
	Code   string `json:"code"`
	FixURL string `json:"fix_url"`
	FixCLI string `json:"fix_cli"`
}

func wildcardsIn(domains []string) []string {
	var out []string
	for _, d := range domains {
		if ingress.IsWildcardDomain(d) {
			out = append(out, d)
		}
	}
	return out
}

// requireWildcardDNS01 rejects wildcard domains while no DNS-01 provider is
// active: HTTP-01 can never issue a wildcard certificate.
func (rt *Router) requireWildcardDNS01(w http.ResponseWriter, r *http.Request, domains []string) bool {
	wild := wildcardsIn(domains)
	if len(wild) == 0 || rt.activeDNSProvider(r.Context()) != dnsProviderNone {
		return true
	}
	writeJSON(w, http.StatusBadRequest, wildcardRejection{
		Error:  fmt.Sprintf("%s needs a DNS-01 provider for its certificate (HTTP-01 cannot issue wildcards): connect Cloudflare or Route53 first", strings.Join(wild, ", ")),
		Code:   wildcardNeedsDNS01,
		FixURL: "/domains#dns-provider",
		FixCLI: "domains cloudflare-dns set --token-stdin",
	})
	return false
}

// wildcardProbeHost returns a random label under a wildcard, so a DNS check
// exercises the wildcard record rather than a cached exact name.
func wildcardProbeHost(domain string) string {
	if !ingress.IsWildcardDomain(domain) {
		return domain
	}
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	return "lr-probe-" + hex.EncodeToString(b) + strings.TrimPrefix(domain, "*")
}

// ingressIPv4 is the address a wildcard A record should point at.
func (rt *Router) ingressIPv4(ctx context.Context) string {
	if ip := net.ParseIP(rt.publicHost); ip != nil && ip.To4() != nil {
		return rt.publicHost
	}
	for _, a := range rt.detectedPublicIPs(ctx) {
		if ip := net.ParseIP(a); ip != nil && ip.To4() != nil {
			return a
		}
	}
	return ""
}

// ensureWildcardRecords creates the "*" A record for each wildcard domain in
// the background, so a slow provider never delays the domain edit itself.
func (rt *Router) ensureWildcardRecords(r *http.Request, domains []string) {
	wild := wildcardsIn(domains)
	if len(wild) == 0 {
		return
	}
	bg := r.Clone(context.WithoutCancel(r.Context()))
	go func() {
		ctx, cancel := context.WithTimeout(bg.Context(), 30*time.Second)
		defer cancel()
		rt.createWildcardRecords(ctx, bg, wild)
	}()
}

// createWildcardRecords adds "*" A records in the longest matching zone. An
// existing set at that name is left alone: it may be someone else's.
func (rt *Router) createWildcardRecords(ctx context.Context, r *http.Request, wild []string) []string {
	ip := rt.ingressIPv4(ctx)
	if ip == "" {
		return nil
	}
	all, err := rt.zoneProviders(ctx)
	if err != nil {
		rt.logger.Warn("api: wildcard dns: providers", slog.String("error", err.Error()))
		return nil
	}
	var created []string
	for _, d := range wild {
		fqdn := dnszones.NormalizeDomain(d)
		for _, pn := range providerNames(all) {
			p := all[pn]
			z, ok := longestZone(ctx, p, strings.TrimPrefix(fqdn, "*."))
			if !ok {
				continue
			}
			rs := dnszones.RecordSet{Name: dnszones.RelativeName(fqdn, z.Name), Type: "A", Values: []string{ip}}
			rs, err := dnszones.Normalize(rs, z.Name, dnsDefaultTTL())
			if err != nil {
				break
			}
			existing, err := p.ListRecordSets(ctx, z)
			if err != nil || slices.ContainsFunc(existing, func(e dnszones.RecordSet) bool { return e.Name == rs.Name && (e.Type == "A" || e.Type == "CNAME") }) {
				break
			}
			if err := p.UpsertRecordSet(ctx, z, rs); err != nil {
				rt.logger.Warn("api: wildcard dns: create record failed", slog.String("domain", d), slog.String("error", err.Error()))
				break
			}
			rt.auditDNS(r.WithContext(ctx), auditDNSRecordCreate, z, rs.Name+"/A wildcard", http.StatusCreated)
			created = append(created, fqdn)
			break
		}
	}
	return created
}

func longestZone(ctx context.Context, p dnszones.Provider, host string) (dnszones.Zone, bool) {
	zones, err := p.ListZones(ctx)
	if err != nil {
		return dnszones.Zone{}, false
	}
	var best dnszones.Zone
	for _, z := range zones {
		if (host == z.Name || strings.HasSuffix(host, "."+z.Name)) && len(z.Name) > len(best.Name) && !z.Private {
			best = z
		}
	}
	return best, best.ID != ""
}
