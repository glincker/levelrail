package api

import (
	"context"
	"crypto/x509"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/GLINCKER/levelrail/internal/reconcile/application"
	"github.com/GLINCKER/levelrail/internal/store"
)

// Failure codes of a reachability probe; the UI maps each to plain copy and
// the fix it links to.
const (
	probeContainerNotRunning  = "container_not_running"
	probeContainerNotListen   = "container_not_listening"
	probeFirewallClosed       = "firewall_closed"
	probeUnreachableFromHost  = "unreachable_from_host"
	probeTLSNotIssued         = "tls_not_issued"
	probeProxyRouteMissing    = "proxy_route_missing"
	probeUpstreamFailing      = "upstream_failing"
	probeAppError             = "app_error"
	probeRequestFailed        = "request_failed"
	reachabilityScopeCPHost   = "control_plane_host"
	reachabilityMaxBodyToRead = 1 << 10
)

type probeTarget struct {
	Kind      string `json:"kind"`
	URL       string `json:"url"`
	OK        bool   `json:"ok"`
	Status    int    `json:"status,omitempty"`
	LatencyMS int64  `json:"latency_ms,omitempty"`
	// Failure is empty on success, otherwise one of the probe* codes.
	Failure string `json:"failure,omitempty"`
	Detail  string `json:"detail,omitempty"`
}

type exposureCheckResource struct {
	App string `json:"app"`
	// Scope says where the probe ran from: only the control plane host is
	// available, so it cannot prove a port is open to the whole internet.
	Scope   string        `json:"scope"`
	Targets []probeTarget `json:"targets"`
}

// handleCheckAppExposure handles POST /api/v1/apps/{name}/exposure/check.
func (rt *Router) handleCheckAppExposure(w http.ResponseWriter, r *http.Request) {
	svc, ok := rt.loadExposureApp(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	out := exposureCheckResource{App: svc.Name, Scope: reachabilityScopeCPHost, Targets: []probeTarget{}}
	plan, _ := rt.planExposure(ctx, svc, exposureRequest{})
	hostPort, running := rt.observedHostPort(ctx, svc)
	path := "/"
	if svc.Health != nil && svc.Health.Readiness != nil && svc.Health.Readiness.Path != "" {
		path = svc.Health.Readiness.Path
	}
	local := probeTarget{Kind: "container", URL: "http://" + net.JoinHostPort("127.0.0.1", strconv.Itoa(hostPort)) + path}
	ingressOnly := running && hostPort == 0
	switch {
	case !running:
		local.URL = ""
		local.Failure = probeContainerNotRunning
	case !ingressOnly:
		rt.probeHTTP(ctx, &local, "127.0.0.1:"+strconv.Itoa(hostPort), probeContainerNotListen)
	}
	// An app served only through the ingress publishes no host port, so there is
	// nothing to probe directly; its addresses are still checked below.
	if !ingressOnly {
		out.Targets = append(out.Targets, local)
	}
	if local.OK || ingressOnly {
		out.Targets = append(out.Targets, rt.probeExposureURLs(ctx, plan, path)...)
	}
	writeJSON(w, http.StatusOK, out)
}

func (rt *Router) probeExposureURLs(ctx context.Context, plan exposurePlan, path string) []probeTarget {
	var out []probeTarget
	for _, u := range plan.URLs {
		t := probeTarget{Kind: u.Kind, URL: u.URL}
		switch u.Kind {
		case "direct":
			t.URL = u.URL + path
			addr := net.JoinHostPort(rt.publicHost, strconv.Itoa(plan.HostPort))
			rt.probeHTTP(ctx, &t, addr, probeFirewallClosed)
		default:
			parsed, err := url.Parse(u.URL)
			if err != nil {
				continue
			}
			port := parsed.Port()
			if port == "" {
				port = "443"
			}
			t.URL = u.URL + path
			rt.probeHTTP(ctx, &t, net.JoinHostPort("127.0.0.1", port), probeUnreachableFromHost)
			if u.Kind == "fallback" && t.OK && rt.publicHost != "" {
				rt.probePublicDial(ctx, &t, net.JoinHostPort(rt.publicHost, port))
			}
		}
		out = append(out, t)
	}
	return out
}

// probePublicDial fails t when this server cannot open its own public address:
// the ingress answers locally, but behind NAT or closed ports a browser never gets there.
func (rt *Router) probePublicDial(ctx context.Context, t *probeTarget, addr string) {
	dctx, cancel := context.WithTimeout(ctx, rt.doctorNetworkTimeoutOrDefault())
	defer cancel()
	conn, err := rt.doctorDialContextOrDefault()(dctx, "tcp", addr)
	if err != nil {
		t.OK, t.Failure, t.Detail = false, probeFirewallClosed, err.Error()
		return
	}
	_ = conn.Close()
}

// observedHostPort is the host port Docker bound for the running container.
func (rt *Router) observedHostPort(ctx context.Context, svc *store.DesiredService) (port int, running bool) {
	if rt.execRuntime == nil {
		return 0, false
	}
	runtime, err := rt.execRuntime(svc.NodeID)
	if err != nil {
		return 0, false
	}
	ictx, cancel := context.WithTimeout(ctx, dockerInspectTimeout)
	defer cancel()
	st, err := runtime.InspectByName(ictx, application.ContainerName(svc.Name, application.NameImage(*svc), svc.RestartNonce))
	if err != nil || st == nil || !st.Running {
		return 0, false
	}
	if len(st.Ports) > 0 {
		return st.Ports[0].HostPort, true
	}
	return 0, true
}

// probeHTTP dials dialAddr (so a check from the control plane host can hit
// the ingress or a published port directly), then GETs t.URL. connectFailure
// is the code to report when the TCP connect itself fails.
func (rt *Router) probeHTTP(ctx context.Context, t *probeTarget, dialAddr, connectFailure string) {
	timeout := rt.doctorNetworkTimeoutOrDefault()
	dial := rt.doctorDialContextOrDefault()
	start := time.Now()
	dctx, cancel := context.WithTimeout(ctx, timeout)
	conn, err := dial(dctx, "tcp", dialAddr)
	cancel()
	if err != nil {
		t.Failure, t.Detail = connectFailure, err.Error()
		return
	}
	_ = conn.Close()
	client := &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{DialContext: func(c context.Context, network, _ string) (net.Conn, error) {
			return dial(c, network, dialAddr)
		}},
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, t.URL, nil)
	if err != nil {
		t.Failure, t.Detail = probeRequestFailed, err.Error()
		return
	}
	resp, err := client.Do(req)
	t.LatencyMS = time.Since(start).Milliseconds()
	if err != nil {
		var unknown x509.UnknownAuthorityError
		var invalid x509.CertificateInvalidError
		var hostErr x509.HostnameError
		if errors.As(err, &unknown) || errors.As(err, &invalid) || errors.As(err, &hostErr) {
			t.Failure = probeTLSNotIssued
		} else {
			t.Failure = probeRequestFailed
		}
		t.Detail = err.Error()
		rt.logger.Debug("api: exposure check request failed", slog.String("url", t.URL), slog.String("error", err.Error()))
		return
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.CopyN(io.Discard, resp.Body, reachabilityMaxBodyToRead)
	t.Status = resp.StatusCode
	switch {
	case resp.StatusCode == http.StatusNotFound && t.Kind != "container" && t.Kind != "direct":
		t.Failure = probeProxyRouteMissing
	case resp.StatusCode >= 502 && resp.StatusCode <= 504 && t.Kind != "container":
		t.Failure = probeUpstreamFailing
	case resp.StatusCode >= 500:
		t.Failure = probeAppError
	default:
		t.OK = true
	}
}
