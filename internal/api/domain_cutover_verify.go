package api

import (
	"context"
	"net"
	"net/http"
	"net/netip"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/internal/cutoververify"
	"github.com/GLINCKER/levelrail/internal/domaindoctor"
	"github.com/GLINCKER/levelrail/internal/runtimesettings"
)

const (
	verifyMaxExpectIPs = 16
	// verifyMaxConcurrent caps verifier runs in flight; each makes outbound
	// requests, so a burst must not turn this server into a request cannon.
	verifyMaxConcurrent = 4
	verifyLimiterMax    = 1024
)

var verifyNow = time.Now

// verifyLimit is shared by every request so the caps hold per server.
var verifyLimit = &verifyLimiter{lastActor: map[string]time.Time{}, lastDomain: map[string]time.Time{}}

// verifyLimiter spaces runs per actor and domain, and per domain across actors
// at a quarter of that gap, and bounds how many run at once.
type verifyLimiter struct {
	mu         sync.Mutex
	lastActor  map[string]time.Time
	lastDomain map[string]time.Time
	running    int
}

// acquire returns a release func, or nil and how long to wait.
func (l *verifyLimiter) acquire(actor, domain string, interval time.Duration, now time.Time) (func(), time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	key := actor + "|" + domain
	if t, ok := l.lastActor[key]; ok && now.Sub(t) < interval {
		return nil, interval - now.Sub(t)
	}
	if t, ok := l.lastDomain[domain]; ok && now.Sub(t) < interval/4 {
		return nil, interval/4 - now.Sub(t)
	}
	if l.running >= verifyMaxConcurrent {
		return nil, time.Second
	}
	if len(l.lastActor) >= verifyLimiterMax || len(l.lastDomain) >= verifyLimiterMax {
		clear(l.lastActor)
		clear(l.lastDomain)
	}
	l.lastActor[key], l.lastDomain[domain] = now, now
	l.running++
	return func() {
		l.mu.Lock()
		l.running--
		l.mu.Unlock()
	}, 0
}

func (l *verifyLimiter) reset() {
	l.mu.Lock()
	clear(l.lastActor)
	clear(l.lastDomain)
	l.running = 0
	l.mu.Unlock()
}

// verifySeams are replaced in tests; a zero value uses the real network.
type verifySeams struct {
	resolver cutoververify.Resolver
	prober   cutoververify.Prober
}

// hostResolver resolves addresses through the router's lookup seam.
type hostResolver struct{ lookup lookupHostFunc }

func (h hostResolver) LookupHost(ctx context.Context, name string) ([]string, error) {
	return h.lookup(ctx, name)
}

func (hostResolver) LookupCNAME(ctx context.Context, name string) (string, error) {
	return net.DefaultResolver.LookupCNAME(ctx, name)
}

// handleDomainCutoverVerify handles GET /api/v1/domains/{domain}/verify: a
// read-only check of a domain that is moving here. Only domains this server
// already serves are accepted, and the name is requested only once DNS points
// here or at a known proxy.
func (rt *Router) handleDomainCutoverVerify(w http.ResponseWriter, r *http.Request) {
	app, domain := rt.diagnoseOwner(w, r)
	if app == "" {
		return
	}
	ctx := r.Context()
	cv := rt.cutoverVerifySettings(ctx)
	timeout := cv.Timeout
	server, ok := rt.verifyServerIPs(w, r)
	if !ok {
		return
	}
	release, wait := verifyLimit.acquire(rt.eventActor(r), domain, cv.MinInterval, verifyNow())
	if release == nil {
		w.Header().Set("Retry-After", strconv.Itoa(int(wait.Seconds())+1))
		writeError(w, http.StatusTooManyRequests, "this domain was just checked; wait a few seconds and run the check again")
		return
	}
	defer release()
	d := cutoververify.Deps{
		Resolver:  rt.domainAuto.verify.resolver,
		Prober:    rt.domainAuto.verify.prober,
		IsProxyIP: domaindoctor.IsCloudflareIP,
	}
	if d.Resolver == nil {
		d.Resolver = hostResolver{lookup: rt.lookupHost}
	}
	if d.Prober == nil {
		port := defaultDoctorHTTPSPort
		if s, err := rt.ingressSettings.GetIngressSettings(ctx); err == nil && s.PublicLinkPort() != 0 {
			port = s.PublicLinkPort()
		}
		d.Prober = cutoververify.NetProber{Timeout: timeout, HTTPSPort: port}
	}
	opts := cutoververify.Options{
		CertWarnDays: cv.CertWarnDays,
		CertFailDays: cv.CertFailDays,
		SlowTTFB:     cv.SlowTTFB,
		HSTSMinAge:   cv.HSTSMinAge,
	}
	vctx, cancel := context.WithTimeout(ctx, timeout*4)
	defer cancel()
	writeJSON(w, http.StatusOK, cutoververify.Verify(vctx, d, domain, server, opts))
}

// cutoverVerifySettings reads the thresholds at request time; without a
// settings service the registered defaults apply.
func (rt *Router) cutoverVerifySettings(ctx context.Context) runtimesettings.CutoverVerify {
	svc := rt.runtimeSettings
	if svc == nil {
		svc = runtimesettings.New(nil, func(string) (string, bool) { return "", false })
	}
	return svc.CutoverVerify(ctx)
}

// verifyServerIPs is the address list the domain must resolve to: expect_ip
// when given (public addresses only), else the configured public host and the
// stored or detected public addresses. The Host header is never consulted, so
// a caller cannot choose what the verifier treats as this server.
func (rt *Router) verifyServerIPs(w http.ResponseWriter, r *http.Request) ([]string, bool) {
	ctx := r.Context()
	var server []string
	for _, raw := range r.URL.Query()["expect_ip"] {
		ip, err := netip.ParseAddr(strings.TrimSpace(raw))
		if err != nil || len(server) >= verifyMaxExpectIPs || !cutoververify.IsPublicAddr(ip) {
			writeError(w, http.StatusBadRequest, "expect_ip must be a public IP address (not loopback, private, link-local or reserved), at most 16 of them")
			return nil, false
		}
		server = append(server, ip.Unmap().String())
	}
	if len(server) > 0 {
		return server, true
	}
	if host := rt.advertisedPublicHost(ctx); host != "" {
		lctx, cancel := context.WithTimeout(ctx, domainCheckLookupTimeout)
		server = expectedIPs(lctx, rt.lookupHost, host)
		cancel()
	}
	for _, ip := range rt.detectedPublicIPs(ctx) {
		if !containsString(server, ip) {
			server = append(server, ip)
		}
	}
	if len(server) == 0 {
		writeError(w, http.StatusConflict, "this server's public address is not known: set the public host or the server address in settings, or pass expect_ip")
		return nil, false
	}
	return server, true
}
