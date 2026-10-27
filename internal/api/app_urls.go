package api

import (
	"net"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"github.com/GLINCKER/levelrail/internal/bindaddr"
	"github.com/GLINCKER/levelrail/internal/network"
	"github.com/GLINCKER/levelrail/internal/store"
)

// Address kinds and reachability hints of GET /api/v1/apps/{name}/urls.
const (
	accessKindLoopback = "loopback"
	accessKindLAN      = "lan"
	accessKindPublic   = "public"
	accessKindDomain   = "domain"
	accessKindInternal = "internal"

	hintReachable      = "reachable"
	hintNotRunning     = "not_running"
	hintBoundLocalhost = "bound_localhost"
	hintRemoteNode     = "remote_node"
	hintUnknown        = "unknown"
)

type appAccessAddress struct {
	Kind          string `json:"kind"`
	Host          string `json:"host,omitempty"`
	URL           string `json:"url,omitempty"`
	Port          int    `json:"port,omitempty"`
	ReachableHint string `json:"reachable_hint"`
}

type appAccessResource struct {
	App           string             `json:"app"`
	ContainerPort int                `json:"container_port"`
	HostPort      int                `json:"host_port,omitempty"`
	BindAddress   string             `json:"bind_address"`
	Running       bool               `json:"running"`
	Node          string             `json:"node,omitempty"`
	LocalNode     bool               `json:"local_node"`
	Addresses     []appAccessAddress `json:"addresses"`
	// HTTPFallbackURL is a plain-http address the ingress really routes for
	// this app's automatic hostname. Set only when a TLS-terminating proxy
	// fronts the ingress (it then serves routes on its http port); with the
	// default redirect there is no plain-http route, so it is omitted.
	HTTPFallbackURL string `json:"http_fallback_url,omitempty"`
}

// hostInterfaceAddrs lists this host's interface addresses by interface
// name; replaced in tests.
var hostInterfaceAddrs = func() map[string][]net.IP {
	out := map[string][]net.IP{}
	ifaces, err := net.Interfaces()
	if err != nil {
		return out
	}
	for _, ifc := range ifaces {
		if ifc.Flags&net.FlagUp == 0 {
			continue
		}
		addrs, aerr := ifc.Addrs()
		if aerr != nil {
			continue
		}
		for _, a := range addrs {
			if ipn, ok := a.(*net.IPNet); ok {
				out[ifc.Name] = append(out[ifc.Name], ipn.IP)
			}
		}
	}
	return out
}

func skipInterface(name string) bool {
	return name == "lo" || name == "docker0" || strings.HasPrefix(name, "br-") ||
		strings.HasPrefix(name, "veth") || strings.HasPrefix(name, "docker")
}

// reachableIPv4s returns the host's routable IPv4 addresses, sorted by
// interface name then address order, skipping loopback, link-local and
// container bridge interfaces.
func reachableIPv4s(byIface map[string][]net.IP) []net.IP {
	names := make([]string, 0, len(byIface))
	for n := range byIface {
		names = append(names, n)
	}
	sort.Strings(names)
	var out []net.IP
	seen := map[string]bool{}
	for _, n := range names {
		if skipInterface(n) {
			continue
		}
		for _, ip := range byIface[n] {
			v4 := ip.To4()
			if v4 == nil || v4.IsLoopback() || v4.IsLinkLocalUnicast() || v4.IsUnspecified() || seen[v4.String()] {
				continue
			}
			seen[v4.String()] = true
			out = append(out, v4)
		}
	}
	return out
}

func httpURL(host string, port int) string {
	return "http://" + net.JoinHostPort(host, strconv.Itoa(port))
}

// Derives local, LAN and public rows from the bound host port.
func hostPortAddresses(hostPort int, running bool, bindIP string, ips []net.IP, publicHost string, direct bool) []appAccessAddress {
	hint := hintReachable
	if !running || hostPort == 0 {
		hint = hintNotRunning
	}
	loopbackOnly := bindIP == "127.0.0.1" || bindIP == "::1"
	out := []appAccessAddress{}
	if hostPort == 0 {
		out = append(out, appAccessAddress{Kind: accessKindLoopback, ReachableHint: hintNotRunning})
	} else {
		out = append(out, appAccessAddress{Kind: accessKindLoopback, Host: "127.0.0.1", Port: hostPort, URL: httpURL("127.0.0.1", hostPort), ReachableHint: hint})
	}
	if loopbackOnly {
		return append(out, appAccessAddress{Kind: accessKindLAN, Port: hostPort, ReachableHint: hintBoundLocalhost})
	}
	for _, ip := range ips {
		if bindIP != "0.0.0.0" && bindIP != ip.String() {
			continue
		}
		kind := accessKindLAN
		if !ip.IsPrivate() {
			kind = accessKindPublic
		}
		a := appAccessAddress{Kind: kind, Host: ip.String(), Port: hostPort, ReachableHint: hint}
		if hostPort > 0 {
			a.URL = httpURL(a.Host, hostPort)
		}
		out = append(out, a)
	}
	if direct && publicHost != "" && hostPort > 0 {
		dup := false
		for _, a := range out {
			dup = dup || a.Host == publicHost
		}
		if !dup {
			out = append(out, appAccessAddress{Kind: accessKindPublic, Host: publicHost, Port: hostPort, URL: httpURL(publicHost, hostPort), ReachableHint: hint})
		}
	}
	return out
}

// handleGetAppURLs handles GET /api/v1/apps/{name}/urls: every way to reach
// one app right now, derived only from what the control plane knows.
func (rt *Router) handleGetAppURLs(w http.ResponseWriter, r *http.Request) {
	svc, ok := rt.loadExposureApp(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	hostPort, running := rt.observedHostPort(ctx, svc)
	if hostPort == 0 && svc.HostPort != nil {
		hostPort = *svc.HostPort
	}
	bindIP, err := bindaddr.Resolve(svc.BindAddress)
	if err != nil {
		bindIP = "127.0.0.1"
	}
	res := appAccessResource{
		App: svc.Name, ContainerPort: svc.Port, HostPort: hostPort, BindAddress: svc.BindAddress,
		Running: running, Node: svc.NodeID, LocalNode: svc.NodeID == "", Addresses: []appAccessAddress{},
	}
	if res.LocalNode {
		res.Addresses = hostPortAddresses(hostPort, running, bindIP, reachableIPv4s(hostInterfaceAddrs()), rt.publicHost, isDirectExposure(svc))
	} else {
		res.Addresses = append(res.Addresses, appAccessAddress{Kind: accessKindLAN, Port: hostPort, ReachableHint: hintRemoteNode})
	}
	ing, ierr := rt.ingressSettings.GetIngressSettings(ctx)
	if ierr != nil {
		ing = store.IngressSettings{}
	}
	for _, d := range svc.Domains {
		u := rt.ingressHTTPSURL(ing, d)
		res.Addresses = append(res.Addresses, appAccessAddress{Kind: accessKindDomain, Host: d, URL: u, ReachableHint: hintUnknown})
	}
	if len(svc.Domains) == 0 && !ing.FallbackDomainsDisabled {
		if fb, ok := rt.autoHost(ctx, svc.Name); ok {
			u := rt.ingressHTTPSURL(ing, fb)
			res.Addresses = append(res.Addresses, appAccessAddress{Kind: accessKindDomain, Host: hostOfURL(u, fb), URL: u, ReachableHint: hintUnknown})
			res.HTTPFallbackURL = rt.plainHTTPFallbackURL(ing, fb, u)
		}
	}
	if dns := dnsNameFor(svc.Name, network.Zone(rt.brand.ShortName)); dns != "" {
		res.Addresses = append(res.Addresses, appAccessAddress{Kind: accessKindInternal, Host: dns, Port: svc.Port, ReachableHint: hintUnknown})
	}
	writeJSON(w, http.StatusOK, res)
}

// plainHTTPFallbackURL is the http link for host, or "" when the ingress
// serves no plain-http routes (only with TLSTerminatedUpstream, see
// internal/ingress BuildRoutesConfig PlainHTTP) or the https link is already http.
func (rt *Router) plainHTTPFallbackURL(s store.IngressSettings, host, httpsLink string) string {
	if !s.TLSTerminatedUpstream || strings.HasPrefix(httpsLink, "http://") {
		return ""
	}
	port := rt.doctorHTTPPort
	if port == 0 || port == 80 {
		return "http://" + host
	}
	return "http://" + net.JoinHostPort(host, strconv.Itoa(port))
}

func hostOfURL(raw, fallback string) string {
	if u, err := url.Parse(raw); err == nil && u.Hostname() != "" {
		return u.Hostname()
	}
	return fallback
}
