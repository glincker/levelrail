package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"strings"

	"github.com/GLINCKER/levelrail/internal/store"
)

// Where the advertised server address came from.
const (
	serverAddressSourceOverride = "override"
	serverAddressSourceDetected = "detected"
	serverAddressSourceEnv      = "env"
	serverAddressSourceNone     = "none"
)

type serverAddressStore interface {
	GetServerAddressOverride(ctx context.Context) (store.ServerAddressOverride, error)
	SetServerAddressOverride(ctx context.Context, o store.ServerAddressOverride) error
}

func (rt *Router) serverAddressStore() serverAddressStore {
	s, _ := rt.apps.(serverAddressStore)
	return s
}

// serverAddressOverrideIPs is the operator override, nil when none is set.
func (rt *Router) serverAddressOverrideIPs(ctx context.Context) []string {
	st := rt.serverAddressStore()
	if st == nil {
		return nil
	}
	o, err := st.GetServerAddressOverride(ctx)
	if err != nil {
		rt.logger.Warn("api: read server address override", slog.String("error", err.Error()))
		return nil
	}
	var out []string
	for _, ip := range []string{o.IPv4, o.IPv6} {
		if ip != "" {
			out = append(out, ip)
		}
	}
	return out
}

// advertisedPublicHost is APP_PUBLIC_HOST, else the override's first address.
func (rt *Router) advertisedPublicHost(ctx context.Context) string {
	if rt.publicHost != "" {
		return rt.publicHost
	}
	if ips := rt.serverAddressOverrideIPs(ctx); len(ips) > 0 {
		return ips[0]
	}
	return ""
}

type serverAddressPair struct {
	IPv4 string `json:"ipv4,omitempty"`
	IPv6 string `json:"ipv6,omitempty"`
}

type dnsRecordSuggestion struct {
	Type  string `json:"type"`
	Name  string `json:"name"`
	Value string `json:"value"`
	// Alternative marks a CNAME offered instead of the A/AAAA records.
	Alternative bool `json:"alternative,omitempty"`
}

// serverAddressResource is GET /api/v1/system/server-address.
type serverAddressResource struct {
	serverAddressPair
	Source           string            `json:"source"`
	Detected         serverAddressPair `json:"detected"`
	Override         serverAddressPair `json:"override"`
	PublicHost       string            `json:"public_host,omitempty"`
	PublicHostSource string            `json:"public_host_source,omitempty"`
	// BehindNAT is true when the public IPv4 is not on any local interface:
	// a router or cloud 1:1 mapping must forward ports 80 and 443 here.
	BehindNAT             bool                  `json:"behind_nat"`
	TLSTerminatedUpstream bool                  `json:"tls_terminated_upstream"`
	AppsBaseDomain        string                `json:"apps_base_domain,omitempty"`
	Domain                string                `json:"domain,omitempty"`
	Records               []dnsRecordSuggestion `json:"records,omitempty"`
}

func splitFamilies(ips []string) serverAddressPair {
	var p serverAddressPair
	for _, s := range ips {
		a, err := netip.ParseAddr(s)
		if err != nil {
			continue
		}
		if a.Unmap().Is4() {
			if p.IPv4 == "" {
				p.IPv4 = a.Unmap().String()
			}
		} else if p.IPv6 == "" {
			p.IPv6 = a.String()
		}
	}
	return p
}

// handleGetServerAddress handles GET /api/v1/system/server-address
// (?domain= adds the exact DNS records to create for it).
func (rt *Router) handleGetServerAddress(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	res := serverAddressResource{PublicHost: rt.publicHost, PublicHostSource: rt.publicHostSource}
	res.Detected = splitFamilies(rt.probedPublicIPs(ctx))
	if st := rt.serverAddressStore(); st != nil {
		if o, err := st.GetServerAddressOverride(ctx); err == nil {
			res.Override = serverAddressPair{IPv4: o.IPv4, IPv6: o.IPv6}
		}
	}
	switch {
	case res.Override.IPv4 != "" || res.Override.IPv6 != "":
		res.serverAddressPair, res.Source = res.Override, serverAddressSourceOverride
	case res.Detected.IPv4 != "" || res.Detected.IPv6 != "":
		res.serverAddressPair, res.Source = res.Detected, serverAddressSourceDetected
	default:
		res.Source = serverAddressSourceNone
		if ip, err := netip.ParseAddr(rt.publicHost); err == nil {
			res.serverAddressPair, res.Source = splitFamilies([]string{ip.String()}), serverAddressSourceEnv
		}
	}
	res.BehindNAT = rt.behindNAT(res.IPv4)
	if s, err := rt.ingressSettings.GetIngressSettings(ctx); err == nil {
		res.TLSTerminatedUpstream = s.TLSTerminatedUpstream
		res.AppsBaseDomain = s.AppsBaseDomain
	}
	if d := normalizeStandaloneDomain(r.URL.Query().Get("domain")); d != "" {
		if msg := validateStandaloneDomainName(d); msg != "" {
			writeError(w, http.StatusBadRequest, msg)
			return
		}
		res.Domain = d
		res.Records = dnsRecordsFor(d, res.serverAddressPair, res.AppsBaseDomain, rt.publicHost)
	}
	writeJSON(w, http.StatusOK, res)
}

func dnsRecordsFor(domain string, ips serverAddressPair, baseDomain, publicHost string) []dnsRecordSuggestion {
	var out []dnsRecordSuggestion
	if ips.IPv4 != "" {
		out = append(out, dnsRecordSuggestion{Type: "A", Name: domain, Value: ips.IPv4})
	}
	if ips.IPv6 != "" {
		out = append(out, dnsRecordSuggestion{Type: "AAAA", Name: domain, Value: ips.IPv6})
	}
	target := ""
	switch {
	case baseDomain != "" && !strings.EqualFold(baseDomain, domain) && !strings.HasSuffix(domain, "."+baseDomain):
		target = baseDomain
	case publicHost != "" && net.ParseIP(publicHost) == nil && !strings.EqualFold(publicHost, domain):
		target = publicHost
	}
	if target != "" && !strings.HasPrefix(domain, "*.") {
		out = append(out, dnsRecordSuggestion{Type: "CNAME", Name: domain, Value: target, Alternative: len(out) > 0})
	}
	return out
}

// behindNAT reports a public IPv4 that no local interface carries.
func (rt *Router) behindNAT(publicIPv4 string) bool {
	if publicIPv4 == "" {
		return false
	}
	list := rt.interfaceAddrs
	if list == nil {
		list = net.InterfaceAddrs
	}
	addrs, err := list()
	if err != nil {
		return false
	}
	for _, a := range addrs {
		if p, err := netip.ParsePrefix(a.String()); err == nil && p.Addr().Unmap().String() == publicIPv4 {
			return false
		}
	}
	return true
}

type serverAddressRequest struct {
	IPv4 string `json:"ipv4"`
	IPv6 string `json:"ipv6"`
}

// handleSetServerAddress handles PUT /api/v1/system/server-address: the
// manual override; both fields empty clears it.
func (rt *Router) handleSetServerAddress(w http.ResponseWriter, r *http.Request) {
	st := rt.serverAddressStore()
	if st == nil {
		writeError(w, http.StatusNotImplemented, "server address override is not supported by this store")
		return
	}
	var req serverAddressRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, standaloneBodyLimit)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	o := store.ServerAddressOverride{IPv4: strings.TrimSpace(req.IPv4), IPv6: strings.TrimSpace(req.IPv6)}
	if o.IPv4 != "" {
		a, err := netip.ParseAddr(o.IPv4)
		if err != nil || !a.Is4() {
			writeError(w, http.StatusBadRequest, "ipv4 must be an IPv4 address")
			return
		}
		o.IPv4 = a.String()
	}
	if o.IPv6 != "" {
		a, err := netip.ParseAddr(o.IPv6)
		if err != nil || !a.Is6() || a.Is4In6() || a.Zone() != "" {
			writeError(w, http.StatusBadRequest, "ipv6 must be an IPv6 address")
			return
		}
		o.IPv6 = a.String()
	}
	if err := st.SetServerAddressOverride(r.Context(), o); err != nil {
		rt.internalError(w, "api: set server address override", err)
		return
	}
	rt.logger.Info("api: server address override set", slog.String("ipv4", o.IPv4), slog.String("ipv6", o.IPv6))
	rt.handleGetServerAddress(w, r)
}
