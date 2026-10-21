package ingress

import (
	"crypto/tls"
	"math/rand/v2"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/modules/caddyhttp"

	"github.com/GLINCKER/levelrail/internal/splittest"
)

func init() {
	caddy.RegisterModule(splitTestingModule{})
}

// splitHopHeader marks a request already routed by a split so the variant's
// own route never splits it again.
const splitHopHeader = "X-Levelrail-Split-Hop"

// ActiveSplit is one running split test as ingress sees it. VariantHosts maps
// a variant ID to the public host serving it; the production variant has none
// and is served by the route the split sits in.
type ActiveSplit struct {
	TestID        string
	CookieName    string
	CookieTTL     time.Duration
	OverrideParam string
	Variants      []splittest.Variant
	VariantHosts  map[string]string
}

type splitEntry struct {
	split  ActiveSplit
	counts map[string]*atomic.Uint64
}

// Splits is the process-wide registry of running split tests, keyed by the
// lowercase host they apply to. Caddy modules are built from JSON and cannot
// receive a live value, so the handler reads this like DefaultRequestStats.
type Splits struct {
	mu     sync.RWMutex
	byHost map[string]*splitEntry
	byTest map[string]*splitEntry

	rngMu sync.Mutex
	rng   *rand.Rand //nolint:forbidigo // traffic split draw, not a secret
}

var defaultSplits = NewSplits()

// DefaultSplits is the registry the split_testing handler reads.
func DefaultSplits() *Splits { return defaultSplits }

// NewSplits returns an empty registry with a randomly seeded draw source.
func NewSplits() *Splits {
	return &Splits{byHost: map[string]*splitEntry{}, byTest: map[string]*splitEntry{}, rng: rand.New(rand.NewPCG(rand.Uint64(), rand.Uint64()))} //nolint:gosec,forbidigo // traffic split draw, not a secret
}

// SeedForTest replaces the draw source so tests are deterministic.
func (s *Splits) SeedForTest(a, b uint64) {
	s.rngMu.Lock()
	s.rng = rand.New(rand.NewPCG(a, b)) //nolint:gosec,forbidigo // seeded draw for deterministic tests
	s.rngMu.Unlock()
}

// Set activates or updates a split on hosts. Counters survive an update of
// the same test ID so editing weights does not reset the request counts.
func (s *Splits) Set(hosts []string, sp ActiveSplit) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.clearLocked(sp.TestID)
	e := &splitEntry{split: sp, counts: make(map[string]*atomic.Uint64, len(sp.Variants))}
	for _, v := range sp.Variants {
		e.counts[v.ID] = new(atomic.Uint64)
	}
	s.byTest[sp.TestID] = e
	for _, h := range hosts {
		s.byHost[strings.ToLower(h)] = e
	}
}

// Clear deactivates the test on every host.
func (s *Splits) Clear(testID string) {
	s.mu.Lock()
	s.clearLocked(testID)
	s.mu.Unlock()
}

func (s *Splits) clearLocked(testID string) {
	old, ok := s.byTest[testID]
	if !ok {
		return
	}
	for h, e := range s.byHost {
		if e == old {
			delete(s.byHost, h)
		}
	}
	delete(s.byTest, testID)
}

// Active reports whether any of hosts has a running split.
func (s *Splits) Active(hosts []string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, h := range hosts {
		if _, ok := s.byHost[strings.ToLower(h)]; ok {
			return true
		}
	}
	return false
}

// Drain returns the requests counted per test and variant since the last
// call and resets them. A drained count not persisted is lost, which is the
// accepted cost of keeping the request path free of database writes.
func (s *Splits) Drain() map[string]map[string]uint64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := map[string]map[string]uint64{}
	for id, e := range s.byTest {
		for vid, c := range e.counts {
			if n := c.Swap(0); n > 0 {
				if out[id] == nil {
					out[id] = map[string]uint64{}
				}
				out[id][vid] = n
			}
		}
	}
	return out
}

func (s *Splits) lookup(host string) (*splitEntry, bool) {
	s.mu.RLock()
	e, ok := s.byHost[host]
	s.mu.RUnlock()
	return e, ok
}

func (s *Splits) decide(e *splitEntry, forced, cookie string) splittest.Assignment {
	s.rngMu.Lock()
	defer s.rngMu.Unlock()
	return splittest.Decide(e.split.Variants, forced, cookie, s.rng)
}

// SplitTestingHandler is the wire shape of the split_testing handler. It
// carries no settings: the running tests live in DefaultSplits.
type SplitTestingHandler struct {
	Handler string `json:"handler"`
}

// NewSplitTestingHandler builds the handler.
func NewSplitTestingHandler() SplitTestingHandler {
	return SplitTestingHandler{Handler: "split_testing"}
}

type splitTestingModule struct{}

func (splitTestingModule) CaddyModule() caddy.ModuleInfo {
	return caddy.ModuleInfo{
		ID:  "http.handlers.split_testing",
		New: func() caddy.Module { return new(splitTestingModule) },
	}
}

func (splitTestingModule) ServeHTTP(w http.ResponseWriter, r *http.Request, next caddyhttp.Handler) error {
	reg := DefaultSplits()
	e, ok := reg.lookup(requestHost(r))
	if !ok || r.Header.Get(splitHopHeader) != "" {
		return next.ServeHTTP(w, r)
	}
	sp := e.split
	cookie := ""
	if c, err := r.Cookie(sp.CookieName); err == nil {
		cookie = c.Value
	}
	a := reg.decide(e, r.URL.Query().Get(sp.OverrideParam), cookie)
	host := sp.VariantHosts[a.Variant.ID]
	local, hasLocal := r.Context().Value(http.LocalAddrContextKey).(net.Addr)
	if host != "" && !hasLocal {
		a.Variant, host = productionVariant(sp), ""
	}
	if a.SetCookie {
		http.SetCookie(w, &http.Cookie{ //nolint:gosec // Secure follows the request scheme so plain http still gets the cookie
			Name: sp.CookieName, Value: a.Variant.ID, Path: "/", MaxAge: int(sp.CookieTTL.Seconds()),
			HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: requestIsHTTPS(r),
		})
	}
	if c := e.counts[a.Variant.ID]; c != nil {
		c.Add(1)
	}
	if host == "" {
		return next.ServeHTTP(w, r)
	}
	proxyToVariant(w, r, host, local.String())
	return nil
}

func productionVariant(sp ActiveSplit) splittest.Variant {
	for _, v := range sp.Variants {
		if v.Kind == splittest.KindProduction {
			return v
		}
	}
	return sp.Variants[0]
}

func requestIsHTTPS(r *http.Request) bool {
	return r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}

// proxyToVariant forwards to this server's own listener with the variant's
// host, so the variant is served by its normal route (TLS, auth, policies).
func proxyToVariant(w http.ResponseWriter, r *http.Request, host, localAddr string) {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	rp := &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.Out.URL = &url.URL{Scheme: scheme, Host: localAddr, Path: pr.In.URL.Path, RawPath: pr.In.URL.RawPath, RawQuery: pr.In.URL.RawQuery}
			pr.Out.Host = host
			pr.SetXForwarded()
			pr.Out.Header.Set(splitHopHeader, "1")
		},
		Transport: variantTransport(host),
		ErrorHandler: func(rw http.ResponseWriter, _ *http.Request, _ error) {
			http.Error(rw, "split test variant unavailable", http.StatusBadGateway)
		},
	}
	rp.ServeHTTP(w, r)
}

var variantTransports sync.Map

// variantTransport is one pooled transport per variant host, so SNI matches
// the host being served and connections are reused across requests.
func variantTransport(host string) *http.Transport {
	if t, ok := variantTransports.Load(host); ok {
		return t.(*http.Transport) //nolint:errcheck,forcetypeassert // only *http.Transport is stored
	}
	// Loopback hop to this same server: the browser leg already verified the
	// public certificate, so only the SNI name matters here.
	t := &http.Transport{TLSClientConfig: &tls.Config{ServerName: host, InsecureSkipVerify: true}, MaxIdleConnsPerHost: 8, IdleConnTimeout: 60 * time.Second} //nolint:gosec // loopback to this same server
	actual, _ := variantTransports.LoadOrStore(host, t)
	return actual.(*http.Transport) //nolint:errcheck,forcetypeassert // only *http.Transport is stored
}

var (
	_ caddyhttp.MiddlewareHandler = splitTestingModule{}
	_ caddy.Module                = splitTestingModule{}
)
