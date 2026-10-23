package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/ingress"
	"github.com/GLINCKER/levelrail/internal/store"
)

// Resolution test outcomes.
const (
	dnsTestOK           = "ok"
	dnsTestWrongIP      = "wrong_ip"
	dnsTestNotResolving = "not_resolving"
	dnsTestUnavailable  = "unavailable"
)

// Fix kinds a failed resolution test suggests.
const (
	dnsFixSwitchProvider = "switch_provider"
	dnsFixCustomDomain   = "custom_domain"
)

const dnsTestTimeout = 5 * time.Second

// dnsLookupHost is swapped by tests.
var dnsLookupHost = func(ctx context.Context, host string) ([]string, error) {
	return net.DefaultResolver.LookupHost(ctx, host)
}

type dnsProviderStore interface {
	ingress.ProviderSource
	SetDNSProviderDefault(ctx context.Context, c store.DNSProviderChoice) error
	GetAppDNSProvider(ctx context.Context, app string) (store.DNSProviderChoice, bool, error)
	SetAppDNSProvider(ctx context.Context, app string, c store.DNSProviderChoice) error
}

type dnsProviderEffective struct {
	Provider         string `json:"provider"`
	CustomBaseDomain string `json:"custom_base_domain,omitempty"`
	// Inherited is true when an app follows the instance default.
	Inherited bool `json:"inherited"`
}

type dnsProviderState struct {
	Providers  []ingress.DNSProvider `json:"providers"`
	Default    dnsProviderEffective  `json:"default"`
	Effective  dnsProviderEffective  `json:"effective"`
	PublicHost string                `json:"public_host,omitempty"`
	// Hostname and URL are the app's automatic address under its effective provider.
	Hostname string `json:"hostname,omitempty"`
	URL      string `json:"url,omitempty"`
}

type dnsProviderUpdate struct {
	Provider         string `json:"provider"`
	CustomBaseDomain string `json:"custom_base_domain"`
}

type dnsProviderFix struct {
	Kind     string `json:"kind"`
	Provider string `json:"provider,omitempty"`
}

type dnsProviderTestResult struct {
	Status     string           `json:"status"`
	Provider   string           `json:"provider"`
	Hostname   string           `json:"hostname,omitempty"`
	Expected   []string         `json:"expected,omitempty"`
	Resolved   []string         `json:"resolved,omitempty"`
	Message    string           `json:"message"`
	Fixes      []dnsProviderFix `json:"fixes,omitempty"`
	DigCommand string           `json:"dig_command,omitempty"`
}

func (rt *Router) dnsStore(w http.ResponseWriter) (dnsProviderStore, bool) {
	s, ok := rt.ingressSettings.(dnsProviderStore)
	if !ok {
		writeError(w, http.StatusNotImplemented, "DNS provider settings are not available")
		return nil, false
	}
	return s, true
}

func (rt *Router) dnsState(ctx context.Context, app string) dnsProviderState {
	res := rt.fallbackResolver(ctx)
	def := res.Default()
	st := dnsProviderState{
		Providers:  ingress.DNSProviders(),
		Default:    dnsProviderEffective{Provider: def.Provider, CustomBaseDomain: def.CustomBase, Inherited: true},
		PublicHost: rt.publicHost,
	}
	st.Effective = st.Default
	if app == "" {
		return st
	}
	c, overridden := res.Choice(app)
	st.Effective = dnsProviderEffective{Provider: c.Provider, CustomBaseDomain: c.CustomBase, Inherited: !overridden}
	if host, ok := res.Domain(app); ok {
		st.Hostname = host
		if ing, err := rt.ingressSettings.GetIngressSettings(ctx); err == nil {
			st.URL = rt.ingressHTTPSURL(ing, host)
		}
	}
	return st
}

func decodeDNSChoice(w http.ResponseWriter, r *http.Request, allowInherit bool) (ingress.ProviderChoice, bool) {
	var body dnsProviderUpdate
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return ingress.ProviderChoice{}, false
	}
	c := ingress.ProviderChoice{Provider: body.Provider, CustomBase: body.CustomBaseDomain}
	if allowInherit && strings.TrimSpace(body.Provider) == "" {
		return ingress.ProviderChoice{}, true
	}
	if err := c.Validate(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return c, false
	}
	return c.Normalized(), true
}

// handleGetDNSProvider handles GET /api/v1/settings/dns-provider.
func (rt *Router) handleGetDNSProvider(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, rt.dnsState(r.Context(), ""))
}

// handleSetDNSProvider handles PUT /api/v1/settings/dns-provider.
func (rt *Router) handleSetDNSProvider(w http.ResponseWriter, r *http.Request) {
	st, ok := rt.dnsStore(w)
	if !ok {
		return
	}
	c, ok := decodeDNSChoice(w, r, false)
	if !ok {
		return
	}
	if err := st.SetDNSProviderDefault(r.Context(), store.DNSProviderChoice{Provider: c.Provider, CustomBaseDomain: c.CustomBase}); err != nil {
		rt.internalError(w, "api: set dns provider default failed", err)
		return
	}
	rt.nudgeReconciler()
	writeJSON(w, http.StatusOK, rt.dnsState(r.Context(), ""))
}

func (rt *Router) loadDNSApp(w http.ResponseWriter, r *http.Request) (string, bool) {
	name := r.PathValue("name")
	if _, err := rt.apps.GetDesiredService(r.Context(), name); err != nil {
		if errors.Is(err, store.ErrServiceNotFound) {
			writeError(w, http.StatusNotFound, "app not found")
		} else {
			rt.internalError(w, "api: dns provider: load app failed", err, slog.String("name", name))
		}
		return "", false
	}
	return name, true
}

// handleGetAppDNSProvider handles GET /api/v1/apps/{name}/dns-provider.
func (rt *Router) handleGetAppDNSProvider(w http.ResponseWriter, r *http.Request) {
	name, ok := rt.loadDNSApp(w, r)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, rt.dnsState(r.Context(), name))
}

// handleSetAppDNSProvider handles PUT /api/v1/apps/{name}/dns-provider: an empty provider clears the override.
func (rt *Router) handleSetAppDNSProvider(w http.ResponseWriter, r *http.Request) {
	name, ok := rt.loadDNSApp(w, r)
	if !ok {
		return
	}
	st, ok := rt.dnsStore(w)
	if !ok {
		return
	}
	c, ok := decodeDNSChoice(w, r, true)
	if !ok {
		return
	}
	before, _ := rt.autoHost(r.Context(), name)
	if err := st.SetAppDNSProvider(r.Context(), name, store.DNSProviderChoice{Provider: c.Provider, CustomBaseDomain: c.CustomBase}); err != nil {
		rt.internalError(w, "api: set app dns provider failed", err, slog.String("name", name))
		return
	}
	after, _ := rt.autoHost(r.Context(), name)
	rt.recordAppEvent(r, store.AppEvent{AppName: name, Kind: store.AppEventConfigChange, Title: "Automatic hostname provider changed", Detail: fmt.Sprintf("%s -> %s", before, after)})
	rt.nudgeReconciler()
	writeJSON(w, http.StatusOK, rt.dnsState(r.Context(), name))
}

// handleTestDNSProvider handles POST /api/v1/settings/dns-provider/test: the body optionally names a
// provider to test instead of the instance default.
func (rt *Router) handleTestDNSProvider(w http.ResponseWriter, r *http.Request) {
	rt.testDNSProvider(w, r, "")
}

// handleTestAppDNSProvider handles POST /api/v1/apps/{name}/dns-provider/test.
func (rt *Router) handleTestAppDNSProvider(w http.ResponseWriter, r *http.Request) {
	name, ok := rt.loadDNSApp(w, r)
	if !ok {
		return
	}
	rt.testDNSProvider(w, r, name)
}

func (rt *Router) testDNSProvider(w http.ResponseWriter, r *http.Request, app string) {
	var body dnsProviderUpdate
	if r.ContentLength != 0 {
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}
	}
	var choice ingress.ProviderChoice
	if strings.TrimSpace(body.Provider) != "" {
		choice = ingress.ProviderChoice{Provider: body.Provider, CustomBase: body.CustomBaseDomain}
		if err := choice.Validate(); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
	} else {
		choice, _ = rt.fallbackResolver(r.Context()).Choice(app)
	}
	writeJSON(w, http.StatusOK, rt.runDNSProviderTest(r.Context(), choice.Normalized()))
}

func (rt *Router) runDNSProviderTest(ctx context.Context, c ingress.ProviderChoice) dnsProviderTestResult {
	res := dnsProviderTestResult{Provider: c.Label()}
	if strings.EqualFold(strings.TrimSpace(rt.publicHost), ingress.LocalHost) {
		res.Status, res.Message = dnsTestUnavailable, "Local mode names apps under localhost, which needs no DNS."
		return res
	}
	host, ok := ingress.SampleHost(rt.publicHost, c)
	if !ok {
		res.Status = dnsTestUnavailable
		res.Message = "No automatic hostname can be built: this server has no public IP address known to the control plane (or the provider does not support its address family)."
		res.Fixes = dnsFixesFor(c)
		return res
	}
	res.Hostname = host
	res.DigCommand = "dig +short " + host
	res.Expected = rt.expectedDNSAddrs(ctx)
	if len(res.Expected) == 0 {
		res.Status, res.Message = dnsTestUnavailable, "This server's public IP is unknown, so the answer cannot be compared."
		return res
	}
	lctx, cancel := context.WithTimeout(ctx, dnsTestTimeout)
	defer cancel()
	got, err := dnsLookupHost(lctx, host)
	res.Resolved = got
	switch {
	case err != nil || len(got) == 0:
		res.Status = dnsTestNotResolving
		res.Message = fmt.Sprintf("%s did not resolve. The provider may be down, blocked by your resolver, or the wildcard record is missing.", host)
		res.Fixes = dnsFixesFor(c)
	case anyOverlap(got, res.Expected):
		res.Status, res.Message = dnsTestOK, fmt.Sprintf("%s resolves to this server.", host)
	default:
		res.Status = dnsTestWrongIP
		res.Message = fmt.Sprintf("%s resolves to %s, not to this server (%s).", host, strings.Join(got, ", "), strings.Join(res.Expected, ", "))
		res.Fixes = dnsFixesFor(c)
	}
	return res
}

func (rt *Router) expectedDNSAddrs(ctx context.Context) []string {
	if ip := net.ParseIP(strings.TrimSpace(rt.publicHost)); ip != nil {
		return []string{ip.String()}
	}
	if h := strings.TrimSpace(rt.publicHost); h != "" {
		lctx, cancel := context.WithTimeout(ctx, dnsTestTimeout)
		defer cancel()
		if got, err := dnsLookupHost(lctx, h); err == nil && len(got) > 0 {
			return got
		}
	}
	return ingress.DetectPublicIPs(ctx)
}

func anyOverlap(a, b []string) bool {
	for _, x := range a {
		xi := net.ParseIP(x)
		for _, y := range b {
			if yi := net.ParseIP(y); xi != nil && yi != nil && xi.Equal(yi) {
				return true
			}
		}
	}
	return false
}

// dnsFixesFor offers every other built-in provider, then the operator's own domain.
func dnsFixesFor(current ingress.ProviderChoice) []dnsProviderFix {
	var out []dnsProviderFix
	for _, p := range ingress.DNSProviders() {
		if p.ID != current.Provider {
			out = append(out, dnsProviderFix{Kind: dnsFixSwitchProvider, Provider: p.ID})
		}
	}
	if current.Provider != ingress.ProviderCustom {
		out = append(out, dnsProviderFix{Kind: dnsFixCustomDomain})
	}
	return out
}
