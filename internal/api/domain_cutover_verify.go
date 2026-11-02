package api

import (
	"context"
	"net"
	"net/http"
	"strings"

	"github.com/GLINCKER/levelrail/internal/cutoververify"
	"github.com/GLINCKER/levelrail/internal/domaindoctor"
	"github.com/GLINCKER/levelrail/internal/runtimesettings"
)

const (
	verifyMaxExpectIPs = 16
)

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
	var server []string
	for _, ip := range r.URL.Query()["expect_ip"] {
		if net.ParseIP(strings.TrimSpace(ip)) == nil || len(server) >= verifyMaxExpectIPs {
			writeError(w, http.StatusBadRequest, "expect_ip must be an IP address, at most 16 of them")
			return
		}
		server = append(server, strings.TrimSpace(ip))
	}
	if len(server) == 0 {
		host, _ := advertisedHost(r, rt.advertisedPublicHost(ctx))
		lctx, cancel := context.WithTimeout(ctx, domainCheckLookupTimeout)
		server = expectedIPs(lctx, rt.lookupHost, host)
		cancel()
		for _, ip := range rt.detectedPublicIPs(ctx) {
			if !containsString(server, ip) {
				server = append(server, ip)
			}
		}
	}
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
