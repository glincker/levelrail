package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/domaindoctor"
	"github.com/GLINCKER/levelrail/internal/ingress"
)

const (
	hstsProbeMaxHosts = 8
	hstsProbeTimeout  = 4 * time.Second
)

var hstsPreloadedTLDs = map[string]bool{"app": true, "dev": true, "page": true, "new": true, "zip": true, "mov": true}

type hstsHostResult struct {
	Host     string `json:"host"`
	Verified bool   `json:"verified"`
	Issuer   string `json:"issuer,omitempty"`
	Error    string `json:"error,omitempty"`
}

type hstsProbeResult struct {
	Enabled  bool             `json:"enabled"`
	Ready    bool             `json:"ready"`
	Hosts    []hstsHostResult `json:"hosts"`
	Warnings []string         `json:"warnings,omitempty"`
	Error    string           `json:"error,omitempty"`
}

type hstsHostProbe func(ctx context.Context, host string, port int) hstsHostResult

func defaultHSTSHostProbe(ctx context.Context, host string, port int) hstsHostResult {
	ctx, cancel := context.WithTimeout(ctx, hstsProbeTimeout)
	defer cancel()
	res := hstsHostResult{Host: host}
	pc, err := domaindoctor.NetTLS{}.Handshake(ctx, net.JoinHostPort(host, strconv.Itoa(port)), host)
	if err != nil {
		res.Error = "could not reach the host over HTTPS"
		return res
	}
	res.Issuer = pc.Issuer
	if !pc.Verified || isInternalIssuer(pc.Issuer) {
		res.Error = "the certificate is self-signed or from the internal issuer"
		return res
	}
	res.Verified = true
	return res
}

func (rt *Router) hstsProbeHosts(ctx context.Context) ([]string, int, error) {
	s, err := rt.ingressSettings.GetIngressSettings(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("api: hsts probe: load ingress settings: %w", err)
	}
	set := map[string]bool{}
	if d := strings.ToLower(strings.TrimSpace(s.PrimaryDomain)); d != "" {
		set[d] = true
	}
	domains, err := rt.domains.ListServiceDomains(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("api: hsts probe: list domains: %w", err)
	}
	for _, d := range domains {
		if h := strings.ToLower(strings.TrimSpace(d.Domain)); ingress.ACMEIssuable(h) && !strings.HasPrefix(h, "*.") {
			set[h] = true
		}
	}
	hosts := make([]string, 0, len(set))
	for h := range set {
		hosts = append(hosts, h)
	}
	sort.Strings(hosts)
	if len(hosts) > hstsProbeMaxHosts {
		hosts = hosts[:hstsProbeMaxHosts]
	}
	port := s.PublicHTTPSPort
	if port == 0 {
		port = 443
	}
	return hosts, port, nil
}

// runHSTSProbe is ready only when every probed host serves a publicly trusted certificate,
// because browsers give no click-through once HSTS has been seen.
func (rt *Router) runHSTSProbe(ctx context.Context) (hstsProbeResult, error) {
	hosts, port, err := rt.hstsProbeHosts(ctx)
	if err != nil {
		return hstsProbeResult{}, err
	}
	probe := rt.hstsHostProbe
	if probe == nil {
		probe = defaultHSTSHostProbe
	}
	out := hstsProbeResult{Enabled: rt.hstsEnabled || rt.hstsEnabledFromDB(ctx), Hosts: []hstsHostResult{}}
	allOK := len(hosts) > 0
	for _, h := range hosts {
		res := probe(ctx, h, port)
		out.Hosts = append(out.Hosts, res)
		if !res.Verified {
			allOK = false
		}
		if i := strings.LastIndexByte(h, '.'); i >= 0 && hstsPreloadedTLDs[h[i+1:]] {
			out.Warnings = append(out.Warnings, "."+h[i+1:]+" domains are on the browser HSTS preload list already")
		}
	}
	if len(hosts) == 0 {
		out.Warnings = append(out.Warnings, "no public domain is configured yet")
	}
	out.Warnings = append(out.Warnings, "HSTS covers subdomains and browsers remember it for 180 days, so it cannot be undone for visitors who already saw it")
	out.Ready = allOK
	return out, nil
}

func (rt *Router) handleGetHSTS(w http.ResponseWriter, r *http.Request) {
	res, err := rt.runHSTSProbe(r.Context())
	if err != nil {
		rt.internalError(w, "api: hsts probe failed", err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

type hstsRequest struct {
	Enabled bool `json:"enabled"`
}

// handlePutHSTS turns HSTS on only after a fresh probe passes; turning it off is always allowed.
func (rt *Router) handlePutHSTS(w http.ResponseWriter, r *http.Request) {
	var req hstsRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<10)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Enabled {
		res, err := rt.runHSTSProbe(r.Context())
		if err != nil {
			rt.internalError(w, "api: hsts enable: probe failed", err)
			return
		}
		if !res.Ready {
			res.Error = "HSTS not enabled: every public domain must serve a publicly trusted certificate first"
			writeJSON(w, http.StatusConflict, res)
			return
		}
	}
	s, err := rt.ingressSettings.GetIngressSettings(r.Context())
	if err != nil {
		rt.internalError(w, "api: hsts update: load ingress settings", err)
		return
	}
	s.HSTSEnabled = req.Enabled
	if err := rt.ingressSettings.UpdateIngressSettings(r.Context(), s); err != nil {
		rt.internalError(w, "api: hsts update failed", err)
		return
	}
	rt.hstsDBEnabled.Store(req.Enabled)
	res, err := rt.runHSTSProbe(r.Context())
	if err != nil {
		rt.internalError(w, "api: hsts update: probe failed", err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}
