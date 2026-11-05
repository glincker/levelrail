package api

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"strconv"
	"strings"

	"github.com/GLINCKER/levelrail/internal/reachability"
	"github.com/GLINCKER/levelrail/internal/runtimesettings"
)

// reachabilityReport is the body of GET /api/v1/doctor/reachability.
type reachabilityReport struct {
	Status          string                    `json:"status"`
	Message         string                    `json:"message"`
	Ports           []reachability.PortReport `json:"ports"`
	DialbackEnabled bool                      `json:"dialback_enabled"`
	DialbackError   string                    `json:"dialback_error,omitempty"`
	Firewall        string                    `json:"firewall,omitempty"`
}

// reachabilityDeps are the seams tests replace.
type reachabilityDeps struct {
	dialer   reachability.Dialer
	firewall func(ctx context.Context) doctorCheckResource
	dialback reachability.DialbackOptions
}

func defaultReachabilityDeps() reachabilityDeps {
	return reachabilityDeps{dialer: &net.Dialer{}, firewall: doctorCheckFirewallCtx}
}

func (rt *Router) reachabilitySettings(ctx context.Context) (dialbackURL string, allowPrivate bool) {
	svc := rt.runtimeSettings
	if svc == nil {
		svc = runtimesettings.New(nil, func(string) (string, bool) { return "", false })
	}
	return strings.TrimSpace(svc.String(ctx, runtimesettings.KeyDoctorReachabilityDialbackURL)),
		svc.Bool(ctx, runtimesettings.KeyDoctorReachabilityDialbackAllowPrivate)
}

// reachabilityTargets reads the configured listen addresses, not guesses.
func (rt *Router) reachabilityTargets() []reachability.Target {
	var ts []reachability.Target
	if host, port, err := net.SplitHostPort(rt.dashboardListenAddr); err == nil {
		if n, err := strconv.Atoi(port); err == nil && n > 0 {
			loop := host == "localhost"
			if a, err := netip.ParseAddr(host); err == nil {
				loop = a.IsLoopback()
			}
			ts = append(ts, reachability.Target{Name: "dashboard", Port: n, LoopbackOnly: loop})
		}
	}
	httpPort, httpsPort := rt.doctorHTTPPort, rt.doctorHTTPSPort
	if httpPort == 0 {
		httpPort = defaultDoctorHTTPPort
	}
	if httpsPort == 0 {
		httpsPort = defaultDoctorHTTPSPort
	}
	return append(ts, reachability.Target{Name: "http", Port: httpPort}, reachability.Target{Name: "https", Port: httpsPort})
}

// primaryAddress is the first global unicast IPv4 on a local interface.
func (rt *Router) primaryAddress() string {
	list := rt.interfaceAddrs
	if list == nil {
		list = net.InterfaceAddrs
	}
	addrs, err := list()
	if err != nil {
		return ""
	}
	for _, a := range addrs {
		p, err := netip.ParsePrefix(a.String())
		if err != nil {
			continue
		}
		if ip := p.Addr().Unmap(); ip.Is4() && ip.IsGlobalUnicast() {
			return ip.String()
		}
	}
	return ""
}

func (rt *Router) buildReachability(ctx context.Context, deps reachabilityDeps) reachabilityReport {
	ports := reachability.ProbeLocal(ctx, deps.dialer, rt.reachabilityTargets(), rt.primaryAddress())
	rep := reachabilityReport{Ports: ports}
	if deps.firewall != nil {
		if fw := deps.firewall(ctx); fw.Status != doctorStatusUnknown {
			rep.Firewall = fw.Message
		}
	}
	dialURL, allowPrivate := rt.reachabilitySettings(ctx)
	if dialURL != "" {
		rep.DialbackEnabled = true
		opt := deps.dialback
		opt.AllowPrivate = allowPrivate
		res, err := reachability.Dialback(ctx, dialURL, reachability.ExposablePorts(ports), opt)
		if err != nil {
			rep.DialbackError = err.Error()
		} else {
			reachability.ApplyDialback(ports, res)
		}
	}
	if rep.Firewall != "" {
		for i := range ports {
			if ports[i].Advice != "" {
				ports[i].Advice += ". Local firewall: " + rep.Firewall
			}
		}
	}
	rep.Status, rep.Message = summarizeReachability(rep)
	return rep
}

func summarizeReachability(rep reachabilityReport) (status, msg string) {
	var down, blocked []string
	for _, p := range rep.Ports {
		label := fmt.Sprintf("%s %d", p.Name, p.Port)
		if !p.Listening {
			down = append(down, label)
		} else if p.External == reachability.ExternalUnreachable {
			blocked = append(blocked, label)
		}
	}
	switch {
	case len(blocked) > 0:
		return doctorStatusWarn, "not reachable from the internet: " + strings.Join(blocked, ", ") + ". Open the port inbound in your cloud firewall or security group"
	case rep.DialbackError != "":
		return doctorStatusWarn, "dial-back check failed: " + rep.DialbackError
	case len(down) > 0:
		return doctorStatusWarn, "nothing listening locally on " + strings.Join(down, ", ")
	case rep.DialbackEnabled:
		return doctorStatusOK, "all ports listen locally and answer from the internet"
	}
	return doctorStatusOK, "all ports listen locally; set doctor.reachability_dialback_url to also check from the internet"
}

func (rt *Router) doctorCheckReachability(ctx context.Context) doctorCheckResource {
	rep := rt.buildReachability(ctx, defaultReachabilityDeps())
	return doctorCheckResource{
		Code: "reachability", Name: "Reachability", Status: rep.Status, Message: rep.Message,
		DocsPath: "/domains-and-ingress#firewall-ports-80-and-443",
	}
}

// handleDoctorReachability handles GET /api/v1/doctor/reachability.
func (rt *Router) handleDoctorReachability(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, rt.buildReachability(r.Context(), defaultReachabilityDeps()))
}
