package ingress

import (
	"fmt"
	"net/netip"
	"os"
	"strings"
	"sync"

	"github.com/oschwald/maxminddb-golang/v2"

	"github.com/GLINCKER/levelrail/internal/trafficpolicy"
)

// Geo source environment variables.
const (
	EnvGeoIPDB            = "APP_GEOIP_DB"
	EnvGeoIPCountryHeader = "APP_GEOIP_COUNTRY_HEADER"
	EnvGeoIPSource        = "APP_GEOIP_SOURCE"
	defaultCountryHeader  = "CF-IPCountry"
)

// Geo source names, reported by GeoStatus.
const (
	GeoSourceHeader = "header"
	GeoSourceMMDB   = "mmdb"
	GeoSourceNone   = "none"
)

// GeoStatus says which country sources are active and why.
type GeoStatus struct {
	Active       bool     `json:"active"`
	Sources      []string `json:"sources"`
	Header       string   `json:"header,omitempty"`
	HeaderNeeds  string   `json:"header_needs,omitempty"`
	DBPath       string   `json:"db_path,omitempty"`
	DBType       string   `json:"db_type,omitempty"`
	DBBuildEpoch uint     `json:"db_build_epoch,omitempty"`
	Error        string   `json:"error,omitempty"`
}

// GeoResolver maps a client address to an ISO country code. Safe for
// concurrent use; the MMDB reader is read only after Open.
type GeoResolver struct {
	header    string
	useHeader bool
	reader    *maxminddb.Reader
	status    GeoStatus
}

type countryRecord struct {
	Country struct {
		ISOCode string `maxminddb:"iso_code"`
	} `maxminddb:"country"`
	RegisteredCountry struct {
		ISOCode string `maxminddb:"iso_code"`
	} `maxminddb:"registered_country"`
}

// NewGeoResolver builds a resolver from the environment. source is auto
// (header when trusted proxies are configured, MMDB when APP_GEOIP_DB
// loads), header, mmdb or none.
func NewGeoResolver(getenv func(string) string) *GeoResolver {
	if getenv == nil {
		getenv = os.Getenv
	}
	g := &GeoResolver{header: strings.TrimSpace(getenv(EnvGeoIPCountryHeader))}
	if g.header == "" {
		g.header = defaultCountryHeader
	}
	source := strings.ToLower(strings.TrimSpace(getenv(EnvGeoIPSource)))
	if source == "" {
		source = "auto"
	}
	trustedProxies := strings.TrimSpace(getenv(EnvTrustedProxies)) != ""
	switch source {
	case GeoSourceNone:
	case GeoSourceHeader:
		g.useHeader = true
	case GeoSourceMMDB:
		g.openDB(getenv(EnvGeoIPDB))
	default:
		g.useHeader = trustedProxies
		g.openDB(getenv(EnvGeoIPDB))
	}
	if g.useHeader {
		g.status.Sources = append(g.status.Sources, GeoSourceHeader)
		g.status.Header = g.header
		if !trustedProxies {
			g.status.HeaderNeeds = fmt.Sprintf("%s is only honoured from addresses in %s, which is empty", g.header, EnvTrustedProxies)
		}
	}
	if g.reader != nil {
		g.status.Sources = append(g.status.Sources, GeoSourceMMDB)
	}
	g.status.Active = len(g.status.Sources) > 0
	if !g.status.Active {
		g.status.Sources = []string{GeoSourceNone}
	}
	return g
}

func (g *GeoResolver) openDB(path string) {
	path = strings.TrimSpace(path)
	if path == "" {
		return
	}
	g.status.DBPath = path
	r, err := maxminddb.Open(path)
	if err != nil {
		g.status.Error = fmt.Sprintf("open %s: %v", path, err)
		return
	}
	g.reader = r
	g.status.DBType = r.Metadata.DatabaseType
	g.status.DBBuildEpoch = r.Metadata.BuildEpoch
}

// Status reports the active sources.
func (g *GeoResolver) Status() GeoStatus { return g.status }

// HeaderName is the country header honoured from trusted proxies.
func (g *GeoResolver) HeaderName() string { return g.header }

// Country resolves addr. headerValue is the country header's value and
// trustedPeer whether the connection came from a trusted proxy; the header
// is ignored otherwise, since any client can send it.
func (g *GeoResolver) Country(addr netip.Addr, headerValue string, trustedPeer bool) (country, source string) {
	if g == nil {
		return "", GeoSourceNone
	}
	if g.useHeader && trustedPeer {
		if c := strings.ToUpper(strings.TrimSpace(headerValue)); trafficpolicy.IsCountryCode(c) {
			return c, GeoSourceHeader
		}
	}
	if g.reader != nil && addr.IsValid() {
		var rec countryRecord
		if err := g.reader.Lookup(addr.Unmap()).Decode(&rec); err == nil {
			if c := rec.Country.ISOCode; c != "" {
				return strings.ToUpper(c), GeoSourceMMDB
			}
			if c := rec.RegisteredCountry.ISOCode; c != "" {
				return strings.ToUpper(c), GeoSourceMMDB
			}
		}
	}
	return "", GeoSourceNone
}

var (
	geoOnce     sync.Once
	geoMu       sync.RWMutex
	geoResolver *GeoResolver
)

// DefaultGeoResolver is the process-wide resolver the geo handler uses;
// Caddy builds handlers from JSON and cannot receive one any other way.
func DefaultGeoResolver() *GeoResolver {
	geoOnce.Do(func() {
		r := NewGeoResolver(os.Getenv)
		geoMu.Lock()
		if geoResolver == nil {
			geoResolver = r
		}
		geoMu.Unlock()
	})
	geoMu.RLock()
	defer geoMu.RUnlock()
	return geoResolver
}

// SetDefaultGeoResolver replaces the process-wide resolver (tests, reload).
func SetDefaultGeoResolver(r *GeoResolver) {
	geoOnce.Do(func() {})
	geoMu.Lock()
	geoResolver = r
	geoMu.Unlock()
}
