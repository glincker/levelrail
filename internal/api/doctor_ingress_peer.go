package api

import (
	"net"
	"net/http"
	"net/netip"
	"strings"
)

const doctorIngressPeerCode = "ingress_peer"

// doctorCheckIngressPeer warns when the dashboard listens on a specific
// non-loopback address and the ingress hop arrives from an address that is
// neither trusted nor the bind address, the signature of a NAT source rewrite.
func (rt *Router) doctorCheckIngressPeer() doctorCheckResource {
	var obs string
	if o := lastIngressPeer.Load(); o != nil {
		obs = o.addr
	}
	return ingressPeerCheck(rt.dashboardListenAddr, obs, peerTrustedByIP)
}

func peerTrustedByIP(host string) bool {
	return peerIsTrustedProxy(&http.Request{RemoteAddr: net.JoinHostPort(host, "0")})
}

func ingressPeerCheck(bind, observed string, trusted func(string) bool) doctorCheckResource {
	c := doctorCheckResource{Code: doctorIngressPeerCode, Name: "Ingress peer address", Status: doctorStatusOK}
	host, _, err := net.SplitHostPort(bind)
	ip, perr := netip.ParseAddr(host)
	if err != nil || perr != nil || ip.IsLoopback() || ip.IsUnspecified() {
		c.Message = "dashboard listens on loopback or all interfaces, so the ingress hop keeps a loopback source"
		return c
	}
	if observed == "" {
		c.Message = "dashboard listens on " + host + "; no ingress request observed yet"
		return c
	}
	obsIP, oerr := netip.ParseAddr(observed)
	switch {
	case oerr == nil && obsIP.IsLoopback(), trusted(observed):
		c.Message = "ingress hop arrives from " + observed + ", which is trusted"
	case oerr == nil && strings.EqualFold(obsIP.String(), ip.String()):
		c.Status = doctorStatusWarn
		c.Message = "ingress hop arrives from " + observed + ", which is not loopback or in " + envTrustedProxies + ". Sign-in works through the in-process ingress marker, but other proxies and IP based policies do not see a trusted peer"
		c.Fix = "bind the dashboard to 127.0.0.1 (APP_HTTP_ADDR) or add " + observed + " to " + envTrustedProxies
	default:
		c.Status = doctorStatusWarn
		c.Message = "dashboard listens on " + host + " but the ingress hop arrives from " + observed + ", so a source rewrite (Docker MASQUERADE) is suspected. Sign-in works through the in-process ingress marker, but the peer address in logs and IP policies is not the real hop"
		c.Fix = "bind the dashboard to 127.0.0.1 (APP_HTTP_ADDR), or add an iptables RETURN rule before the Docker MASQUERADE rule for " + host + " to " + host
	}
	return c
}
