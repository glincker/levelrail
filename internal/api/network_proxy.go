package api

// This file: GET /api/v1/network/proxy, a per-domain reachability join
// for the Traffic/Proxy dashboard page. It surfaces exactly the gap
// doctor_cross_node_ingress.go's check already detects (an app placed on
// a different node than this control plane's own ingress, so its
// domain(s) silently never route, see internal/reconcile/ingress's
// CrossNodeIngress condition) plus GET /api/v1/certificates' own TLS
// status/issuer, joined into one table-shaped response instead of a
// flat pass/fail doctor list.

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/alerting"
)

// networkProxyDomainResource is one row: a domain, the app it routes to,
// where that app actually runs, whether this control plane's own
// ingress can reach it, and its certificate state.
type networkProxyDomainResource struct {
	Domain      string `json:"domain"`
	App         string `json:"app"`
	NodeID      string `json:"node_id"`
	NodeName    string `json:"node_name,omitempty"`
	IsLocalNode bool   `json:"is_local_node"`
	Port        int    `json:"port"`
	// Reachable is false exactly when CrossNodeIngress's own condition
	// fires for this app: placed on a different node with no mesh path
	// to it yet.
	Reachable bool `json:"reachable"`
	// FixCommand mirrors doctor_cross_node_ingress.go's own Fix string,
	// empty when Reachable is true.
	FixCommand string `json:"fix_command,omitempty"`
	TLSStatus  string `json:"tls_status,omitempty"`
	TLSIssuer  string `json:"tls_issuer,omitempty"`
	// TLSSource is "acme" or "custom", mirrors certificateStatus.Source.
	TLSSource string `json:"tls_source,omitempty"`
}

// networkProxyResponse is GET /api/v1/network/proxy's response body.
type networkProxyResponse struct {
	Domains []networkProxyDomainResource `json:"domains"`
}

// handleGetNetworkProxy handles GET /api/v1/network/proxy. See this
// file's own header for what it joins and why.
func (rt *Router) handleGetNetworkProxy(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	services, err := rt.apps.ListDesiredServices(ctx)
	if err != nil {
		rt.internalError(w, "api: get network proxy: list services failed", err)
		return
	}

	nodeNames := rt.doctorNodeNames(ctx)

	certByDomain, err := rt.proxyCertsByDomain(ctx)
	if err != nil {
		rt.internalError(w, "api: get network proxy: list certificates failed", err)
		return
	}

	out := make([]networkProxyDomainResource, 0, len(services))
	for _, svc := range services {
		for _, domain := range svc.Domains {
			row := networkProxyDomainResource{
				Domain:      domain,
				App:         svc.Name,
				NodeID:      svc.NodeID,
				NodeName:    nodeNames[svc.NodeID],
				IsLocalNode: rt.isLocalNode(svc.NodeID),
				Port:        svc.Port,
				Reachable:   rt.isLocalNode(svc.NodeID),
			}
			if !row.Reachable {
				row.FixCommand = fmt.Sprintf(
					"levelrail-cli apps set-node %s <this control plane's own node id>   # or: levelrail-cli apps clear-node %s",
					svc.Name, svc.Name,
				)
			}
			if cert, ok := certByDomain[strings.ToLower(domain)]; ok {
				row.TLSStatus = cert.Status
				row.TLSIssuer = cert.Issuer
				row.TLSSource = cert.Source
			}
			out = append(out, row)
		}
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].Domain != out[j].Domain {
			return out[i].Domain < out[j].Domain
		}
		return out[i].App < out[j].App
	})

	writeJSON(w, http.StatusOK, networkProxyResponse{Domains: out})
}

// proxyCertInfo is the slice of certificateStatus a proxy row needs.
type proxyCertInfo struct {
	Status string
	Issuer string
	Source string
}

// proxyCertsByDomain mirrors handleListCertificates' own computation
// (alerting.ListCertificates plus customTLSCertDomains), keyed by every
// domain and SAN a certificate covers, lowercased: the same multi-key
// join certVisible already performs for visibility filtering.
func (rt *Router) proxyCertsByDomain(ctx context.Context) (map[string]proxyCertInfo, error) {
	warningWindow := rt.certExpiryWarningWindow
	if warningWindow <= 0 {
		warningWindow = alerting.DefaultCertExpiryWarningWindow
	}
	infos, err := alerting.ListCertificates(ctx, rt.certs, warningWindow, time.Now(), rt.logger)
	if err != nil {
		return nil, fmt.Errorf("list certificates: %w", err)
	}
	customDomains, err := rt.customTLSCertDomains(ctx)
	if err != nil {
		return nil, err
	}

	out := make(map[string]proxyCertInfo, len(infos))
	for _, info := range infos {
		source := "acme"
		if customDomains[strings.ToLower(info.Domain)] {
			source = "custom"
		}
		ci := proxyCertInfo{Status: info.Status, Issuer: info.Issuer, Source: source}
		for _, d := range append([]string{info.Domain}, info.SANs...) {
			out[strings.ToLower(d)] = ci
		}
	}
	return out, nil
}
