package ingress

import (
	"fmt"
	"net"
	"net/http"
	"net/netip"

	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/modules/caddyhttp"

	"github.com/GLINCKER/levelrail/internal/trafficpolicy"
)

func init() {
	caddy.RegisterModule(geoModule{})
}

// GeoHandler is the wire shape of the levelrail_geo handler.
type GeoHandler struct {
	Handler string            `json:"handler"`
	Rule    trafficpolicy.Geo `json:"rule"`
}

func newGeoHandler(g trafficpolicy.Geo) GeoHandler {
	return GeoHandler{Handler: "levelrail_geo", Rule: g}
}

// geoModule (http.handlers.levelrail_geo) enforces one domain's country rule
// using the process-wide GeoResolver.
type geoModule struct {
	Rule   trafficpolicy.Geo `json:"rule"`
	exempt []netip.Prefix
}

func (geoModule) CaddyModule() caddy.ModuleInfo {
	return caddy.ModuleInfo{
		ID:  "http.handlers.levelrail_geo",
		New: func() caddy.Module { return new(geoModule) },
	}
}

// Provision parses the exempt list once per config load.
func (m *geoModule) Provision(caddy.Context) error {
	for _, e := range m.Rule.Exempt {
		p, err := trafficpolicy.ParseIPOrPrefix(e)
		if err != nil {
			return fmt.Errorf("ingress: geo exempt %q: %w", e, err)
		}
		m.exempt = append(m.exempt, p)
	}
	return nil
}

func (m *geoModule) ServeHTTP(w http.ResponseWriter, r *http.Request, next caddyhttp.Handler) error {
	client := requestClientAddr(r)
	trusted, _ := caddyhttp.GetVar(r.Context(), caddyhttp.TrustedProxyVarKey).(bool)
	res := DefaultGeoResolver()
	country, _ := res.Country(client, r.Header.Get(res.HeaderName()), trusted)
	if d := m.Rule.Decide(client, country, m.exempt); !d.Blocked {
		return next.ServeHTTP(w, r)
	}
	writeGeoBlock(w, m.Rule)
	return nil
}

func writeGeoBlock(w http.ResponseWriter, g trafficpolicy.Geo) {
	w.Header().Set("Cache-Control", "no-store")
	switch g.Action {
	case trafficpolicy.GeoActionRedirect:
		w.Header().Set("Location", g.RedirectURL)
		w.WriteHeader(http.StatusFound)
	case trafficpolicy.GeoActionErrorPage:
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(g.BlockStatus())
		_, _ = w.Write([]byte(g.Body))
	default:
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(g.BlockStatus())
		_, _ = w.Write([]byte("This site is not available in your region.\n"))
	}
}

// requestClientAddr prefers Caddy's client_ip var, which already applies
// the trusted proxies and client IP header settings.
func requestClientAddr(r *http.Request) netip.Addr {
	if s, ok := caddyhttp.GetVar(r.Context(), caddyhttp.ClientIPVarKey).(string); ok && s != "" {
		if a, err := netip.ParseAddr(s); err == nil {
			return a.Unmap()
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	a, _ := netip.ParseAddr(host)
	return a.Unmap()
}

var (
	_ caddyhttp.MiddlewareHandler = (*geoModule)(nil)
	_ caddy.Provisioner           = (*geoModule)(nil)
)
