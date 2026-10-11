package api

import (
	"context"
	"crypto/tls"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/proxycoexist"
)

type reverseProxyHolder = proxycoexist.Holder

type reverseProxyGuideResource struct {
	Holders  []reverseProxyHolder `json:"holders"`
	Detected string               `json:"detected_proxy,omitempty"`
	Plan     *proxycoexist.Plan   `json:"plan,omitempty"`
	Check    *reverseProxyCheck   `json:"check,omitempty"`
	Message  string               `json:"message,omitempty"`
}

// reverseProxyCheck is the outcome of asking whether the domain already
// reaches this dashboard through the proxy.
type reverseProxyCheck struct {
	ResolvesToHost bool   `json:"resolves_to_host"`
	Reachable      bool   `json:"reachable"`
	Detail         string `json:"detail,omitempty"`
}

// portHolders lists running containers that publish a host port the ingress
// wants, which is how an existing proxy shows up.
func (rt *Router) portHolders(ctx context.Context) []reverseProxyHolder {
	if rt.execRuntime == nil {
		return nil
	}
	runtime, err := rt.execRuntime("")
	if err != nil {
		return nil
	}
	lister, ok := runtime.(LocalContainerLister)
	if !ok {
		return nil
	}
	all, err := lister.ListLocalContainers(ctx)
	if err != nil {
		return nil
	}
	var out []reverseProxyHolder
	for _, c := range all {
		if !c.Running {
			continue
		}
		for _, p := range c.Published {
			if p != 80 && p != 443 {
				continue
			}
			h := reverseProxyHolder{Port: p, Container: c.Name, Image: c.Image, NetworkGateway: firstGateway(c)}
			if k, ok := proxycoexist.KindFromImage(c.Image); ok {
				h.Kind = k
			}
			out = append(out, h)
		}
	}
	return out
}

func firstGateway(c docker.LocalContainer) string {
	for _, n := range c.Networks {
		if n.Gateway != "" {
			return n.Gateway
		}
	}
	return ""
}

// handleReverseProxyGuide handles GET /api/v1/system/reverse-proxy: who holds
// ports 80 and 443, and the configuration that puts the dashboard behind
// that proxy. With ?domain= it also builds the plan, and with &verify=true it
// checks the domain reaches this dashboard. With ?app= the snippet routes
// that app's domain to the ingress HTTP listener instead.
func (rt *Router) handleReverseProxyGuide(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	out := reverseProxyGuideResource{Holders: rt.portHolders(r.Context())}
	if out.Holders == nil {
		out.Holders = []reverseProxyHolder{}
	}
	var holder *reverseProxyHolder
	for i := range out.Holders {
		if out.Holders[i].Kind != "" {
			holder = &out.Holders[i]
			out.Detected = string(holder.Kind)
			break
		}
	}
	domain := strings.TrimSpace(q.Get("domain"))
	target := proxycoexist.Target{Domain: domain, ListenAddr: rt.dashboardListenAddr, Name: rt.brand.BinaryName + "-dashboard", EnvVar: "APP_HTTP_ADDR"}
	if app := strings.TrimSpace(q.Get("app")); app != "" {
		t, status, msg := rt.appGuideTarget(r.Context(), app, domain)
		if status != 0 {
			writeError(w, status, msg)
			return
		}
		target, domain = t, t.Domain
	}
	if domain == "" {
		writeJSON(w, http.StatusOK, out)
		return
	}
	kind := proxycoexist.Kind(out.Detected)
	if p := q.Get("proxy"); p != "" {
		k, err := proxycoexist.ParseKind(p)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		kind = k
	}
	if kind == "" {
		kind = proxycoexist.Caddy
	}
	gateway := ""
	if holder != nil {
		gateway = holder.NetworkGateway
	}
	target.Kind, target.Gateway = kind, gateway
	plan, err := proxycoexist.BuildTarget(target)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	out.Plan = &plan
	if q.Get("verify") == "true" {
		out.Check = rt.checkDomainReachesDashboard(r.Context(), domain)
	}
	writeJSON(w, http.StatusOK, out)
}

// checkDomainReachesDashboard only probes a domain that resolves to this
// host's own public address, so it cannot be pointed at other networks.
func (rt *Router) checkDomainReachesDashboard(ctx context.Context, domain string) *reverseProxyCheck {
	res := &reverseProxyCheck{}
	ips, err := net.DefaultResolver.LookupHost(ctx, domain)
	if err != nil {
		res.Detail = "the domain does not resolve yet: add its A record"
		return res
	}
	for _, ip := range ips {
		if rt.publicHost != "" && ip == rt.publicHost {
			res.ResolvesToHost = true
		}
	}
	if !res.ResolvesToHost {
		res.Detail = "the domain resolves to " + strings.Join(ips, ", ") + ", not to this server's address " + rt.publicHost
		return res
	}
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://"+domain+"/healthz", nil) //nolint:gosec // domain is validated and resolves to this host's own public address
	if err != nil {
		res.Detail = "invalid domain"
		return res
	}
	// Dial this host's own address regardless of what DNS says now, so a
	// rebinding answer between the lookup above and this request cannot
	// redirect the probe elsewhere.
	dialer := &net.Dialer{}
	client := &http.Client{
		Timeout: 8 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12, ServerName: domain},
			DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
				return dialer.DialContext(ctx, network, net.JoinHostPort(rt.publicHost, "443"))
			},
		},
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	resp, err := client.Do(req) //nolint:gosec // same request, see above
	if err != nil {
		res.Detail = "https://" + domain + " is not reachable yet: " + err.Error()
		return res
	}
	defer func() { _ = resp.Body.Close() }()
	res.Reachable = resp.StatusCode == http.StatusOK
	if !res.Reachable {
		res.Detail = "the proxy answered with status " + http.StatusText(resp.StatusCode) + ", so the route is not pointing at this dashboard"
	}
	return res
}

// appGuideTarget routes app's domain (the first one when domain is empty) to
// the ingress HTTP listener. A non-zero status is the error to return.
func (rt *Router) appGuideTarget(ctx context.Context, app, domain string) (proxycoexist.Target, int, string) {
	svc, err := rt.apps.GetDesiredService(ctx, app)
	if err != nil || svc == nil {
		return proxycoexist.Target{}, http.StatusNotFound, "app " + app + " not found"
	}
	switch {
	case domain == "" && len(svc.Domains) == 0:
		return proxycoexist.Target{}, http.StatusBadRequest, "app " + app + " has no domains yet: add one first"
	case domain == "":
		domain = svc.Domains[0]
	case !containsString(svc.Domains, strings.ToLower(domain)):
		return proxycoexist.Target{}, http.StatusBadRequest, domain + " is not a domain of app " + app
	}
	return proxycoexist.Target{Domain: domain, ListenAddr: rt.ingressHTTPListenAddr(), Name: rt.brand.BinaryName + "-" + app, EnvVar: "APP_INGRESS_HTTP_ADDR"}, 0, ""
}

// ingressHTTPListenAddr is where the ingress serves plain HTTP.
func (rt *Router) ingressHTTPListenAddr() string {
	if rt.proxyIntegration != nil && rt.proxyIntegration.syncer.Ingress.Addr != "" {
		return rt.proxyIntegration.syncer.Ingress.Addr
	}
	port := rt.doctorHTTPPort
	if port == 0 {
		port = defaultDoctorHTTPPort
	}
	return ":" + strconv.Itoa(port)
}
