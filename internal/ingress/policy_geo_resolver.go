package ingress

import (
	"fmt"
	"net/netip"
	"os"
	"strings"
	"sync"

	"github.com/oschwald/maxminddb-golang/v2"

	"github.com/glincker/levelrail/internal/trafficpolicy"
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
	source    string
	seedTrust string
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
	g.source = source
	g.seedTrust = getenv(EnvTrustedProxies)
	switch source {
	case GeoSourceNone, GeoSourceHeader:
	default:
		g.openDB(getenv(EnvGeoIPDB))
	}
	return g
}

// headerOn is evaluated per call so a trusted proxy list saved later applies
// without rebuilding the resolver.
func (g *GeoResolver) headerOn() bool {
	switch g.source {
	case GeoSourceNone, GeoSourceMMDB:
		return false
	case GeoSourceHeader:
		return true
	}
	return TrustedProxiesConfigured(g.seedTrust)
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
func (g *GeoResolver) Status() GeoStatus {
	st := g.status
	st.Sources = nil
	if g.headerOn() {
		st.Sources = append(st.Sources, GeoSourceHeader)
		st.Header = g.header
		if !TrustedProxiesConfigured(g.seedTrust) {
			st.HeaderNeeds = fmt.Sprintf("%s is only honoured from addresses in %s, which is empty", g.header, EnvTrustedProxies)
		}
	}
	if g.reader != nil {
		st.Sources = append(st.Sources, GeoSourceMMDB)
	}
	st.Active = len(st.Sources) > 0
	if !st.Active {
		st.Sources = []string{GeoSourceNone}
	}
	return st
}

// HeaderName is the country header honoured from trusted proxies.
func (g *GeoResolver) HeaderName() string { return g.header }

// Country resolves addr. headerValue is the country header's value and
// trustedPeer whether the connection came from a trusted proxy; the header
// is ignored otherwise, since any client can send it.
func (g *GeoResolver) Country(addr netip.Addr, headerValue string, trustedPeer bool) (country, source string) {
	if g == nil {
		return "", GeoSourceNone
	}
	if g.headerOn() && trustedPeer {
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
