package extdb

import (
	"errors"
	"net/url"
	"strconv"
	"strings"
)

// Provider is a preset for a hosted database service: where it listens, how
// strictly to verify TLS and where its documentation lives. It is data only.
type Provider struct {
	ID       string   `json:"id"`
	Label    string   `json:"label"`
	Brand    string   `json:"brand"`
	Engines  []string `json:"engines"`
	Port     int      `json:"port,omitempty"`
	TLSMode  string   `json:"tls_mode"`
	HostHint string   `json:"host_hint"`
	DocsURL  string   `json:"docs_url"`
	// CAURL is where the provider publishes its CA bundle, set only for
	// providers whose certificates do not chain to the system roots.
	CAURL    string   `json:"ca_url,omitempty"`
	Suffixes []string `json:"-"`
}

// ProviderGeneric is the id used when no preset applies.
const ProviderGeneric = "generic"

var allEngines = []string{EnginePostgres, EngineMySQL, EngineMariaDB, EngineMongoDB, EngineRedis}

var providers = []Provider{
	{ID: "neon", Label: "Neon", Brand: "neon", Engines: []string{EnginePostgres}, TLSMode: TLSVerifyFull,
		HostHint: "ep-name-123456.region.aws.neon.tech", DocsURL: "https://neon.com/docs/connect/connect-from-any-app",
		Suffixes: []string{"neon.tech"}},
	{ID: "aws-rds", Label: "Amazon RDS", Brand: "aws-rds", Engines: []string{EnginePostgres, EngineMySQL, EngineMariaDB}, TLSMode: TLSRequire,
		HostHint: "name.abc123.region.rds.amazonaws.com", DocsURL: "https://docs.aws.amazon.com/AmazonRDS/latest/UserGuide/UsingWithRDS.SSL.html",
		CAURL: "https://truststore.pki.rds.amazonaws.com/global/global-bundle.pem", Suffixes: []string{"rds.amazonaws.com", "rds.amazonaws.com.cn"}},
	{ID: "aws-aurora", Label: "Amazon Aurora", Brand: "aws-aurora", Engines: []string{EnginePostgres, EngineMySQL}, TLSMode: TLSRequire,
		HostHint: "cluster-name.cluster-abc123.region.rds.amazonaws.com", DocsURL: "https://docs.aws.amazon.com/AmazonRDS/latest/AuroraUserGuide/UsingWithRDS.SSL.html",
		CAURL: "https://truststore.pki.rds.amazonaws.com/global/global-bundle.pem"},
	{ID: "supabase", Label: "Supabase", Brand: "supabase", Engines: []string{EnginePostgres}, TLSMode: TLSRequire,
		HostHint: "db.project-ref.supabase.co", DocsURL: "https://supabase.com/docs/guides/database/connecting-to-postgres",
		CAURL: "https://supabase.com/docs/guides/platform/ssl-enforcement", Suffixes: []string{"supabase.co", "supabase.com"}},
	{ID: "planetscale", Label: "PlanetScale", Brand: "planetscale", Engines: []string{EngineMySQL, EnginePostgres}, TLSMode: TLSVerifyFull,
		HostHint: "region.connect.psdb.cloud", DocsURL: "https://planetscale.com/docs/vitess/connecting",
		Suffixes: []string{"psdb.cloud", "planetscale.com"}},
	{ID: "mongodb-atlas", Label: "MongoDB Atlas", Brand: "mongodb", Engines: []string{EngineMongoDB}, TLSMode: TLSVerifyFull,
		HostHint: "cluster0.abc123.mongodb.net", DocsURL: "https://www.mongodb.com/docs/atlas/driver-connection/",
		Suffixes: []string{"mongodb.net"}},
	{ID: "upstash", Label: "Upstash Redis", Brand: "upstash", Engines: []string{EngineRedis}, TLSMode: TLSVerifyFull,
		HostHint: "name-12345.upstash.io", DocsURL: "https://upstash.com/docs/redis/howto/connectclient",
		Suffixes: []string{"upstash.io"}},
	{ID: "azure", Label: "Azure Database", Brand: "azure", Engines: []string{EnginePostgres, EngineMySQL, EngineMariaDB}, TLSMode: TLSVerifyFull,
		HostHint: "name.postgres.database.azure.com", DocsURL: "https://learn.microsoft.com/azure/postgresql/flexible-server/concepts-networking-ssl-tls",
		Suffixes: []string{"database.azure.com"}},
	{ID: "gcp-cloudsql", Label: "Google Cloud SQL", Brand: "google-cloud", Engines: []string{EnginePostgres, EngineMySQL}, TLSMode: TLSRequire,
		HostHint: "instance IP address or name.region.sql.goog", DocsURL: "https://cloud.google.com/sql/docs/postgres/configure-ssl-instance",
		CAURL: "https://cloud.google.com/sql/docs/postgres/configure-ssl-instance", Suffixes: []string{"sql.goog"}},
	{ID: "aiven", Label: "Aiven", Brand: "aiven", Engines: []string{EnginePostgres, EngineMySQL, EngineRedis}, TLSMode: TLSRequire,
		HostHint: "service-project.aivencloud.com", DocsURL: "https://aiven.io/docs/platform/howto/download-ca-cert",
		CAURL: "https://aiven.io/docs/platform/howto/download-ca-cert", Suffixes: []string{"aivencloud.com"}},
	{ID: ProviderGeneric, Label: "Other", Brand: "", Engines: allEngines, TLSMode: TLSRequire,
		HostHint: "any host name or IP address", DocsURL: ""},
}

// Providers returns the presets in display order. The slice is a copy.
func Providers() []Provider {
	out := make([]Provider, len(providers))
	copy(out, providers)
	return out
}

// ProviderByID returns the preset with that id.
func ProviderByID(id string) (Provider, bool) {
	for _, p := range providers {
		if p.ID == id {
			return p, true
		}
	}
	return Provider{}, false
}

// DetectProvider names the hosting service from a host name, or "" when the
// host matches no preset.
func DetectProvider(host string) string {
	h := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(host), "."))
	if h == "" {
		return ""
	}
	for _, p := range providers {
		for _, sfx := range p.Suffixes {
			if h == sfx || strings.HasSuffix(h, "."+sfx) {
				if p.ID == "aws-rds" && strings.Contains(h, ".cluster-") {
					return "aws-aurora"
				}
				return p.ID
			}
		}
	}
	return ""
}

// Parsed is a connection string split into fields. The password is carried
// here for the caller to store; it must never be returned over the API.
type Parsed struct {
	Engine       string
	Host         string
	Port         int
	User         string
	Password     string
	Database     string
	AuthDatabase string
	TLSMode      string
	Provider     string
}

const maxConnectionURLLen = 2048

// ErrBadConnectionURL carries no part of the input, so it is safe to log.
var ErrBadConnectionURL = errors.New("connection string is not valid: use postgres://, mysql://, mongodb://, mongodb+srv:// or rediss://user:password@host:port/database")

var schemeEngine = map[string]string{
	"postgres": EnginePostgres, "postgresql": EnginePostgres,
	"mysql": EngineMySQL, "mariadb": EngineMariaDB,
	"mongodb": EngineMongoDB, "mongodb+srv": EngineMongoDB,
	"redis": EngineRedis, "rediss": EngineRedis,
}

// ParseURL splits a database connection string. Errors never quote the input
// because it usually contains a password.
func ParseURL(raw string) (Parsed, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || len(raw) > maxConnectionURLLen {
		return Parsed{}, ErrBadConnectionURL
	}
	scheme, _, ok := strings.Cut(raw, "://")
	engine, known := schemeEngine[strings.ToLower(scheme)]
	if !ok || !known {
		return Parsed{}, ErrBadConnectionURL
	}
	u, err := url.Parse(firstMongoHost(raw))
	if err != nil || u.Hostname() == "" {
		return Parsed{}, ErrBadConnectionURL
	}
	p := Parsed{Engine: engine, Host: u.Hostname(), Database: strings.TrimPrefix(u.Path, "/")}
	if db, derr := url.PathUnescape(p.Database); derr == nil {
		p.Database = db
	}
	if ps := u.Port(); ps != "" {
		n, perr := strconv.Atoi(ps)
		if perr != nil || n < 1 || n > 65535 {
			return Parsed{}, ErrBadConnectionURL
		}
		p.Port = n
	}
	if u.User != nil {
		p.User = u.User.Username()
		p.Password, _ = u.User.Password()
	}
	q := u.Query()
	p.AuthDatabase = q.Get("authSource")
	if engine == EngineRedis && p.Database != "" {
		// A redis path is a numeric database index, not a name.
		p.Database = ""
	}
	p.Provider = DetectProvider(p.Host)
	var off bool
	p.TLSMode, off = urlTLSMode(engine, strings.ToLower(scheme), q)
	if pr, found := ProviderByID(p.Provider); found && p.Provider != ProviderGeneric && !off {
		p.TLSMode = strongerTLS(p.TLSMode, pr.TLSMode)
	}
	if p.Port == 0 && !strings.HasSuffix(strings.ToLower(scheme), "+srv") {
		p.Port = DefaultPort(engine)
	}
	return p, nil
}

// firstMongoHost reduces a multi-host mongodb seed list to its first host so
// the standard parser accepts it. Other schemes pass through unchanged.
func firstMongoHost(raw string) string {
	if !strings.HasPrefix(strings.ToLower(raw), "mongodb") {
		return raw
	}
	_, rest, _ := strings.Cut(raw, "://")
	end := strings.IndexAny(rest, "/?")
	if end < 0 {
		end = len(rest)
	}
	auth := rest[:end]
	at := strings.LastIndex(auth, "@")
	hosts := auth[at+1:]
	if !strings.Contains(hosts, ",") {
		return raw
	}
	first, _, _ := strings.Cut(hosts, ",")
	scheme, _, _ := strings.Cut(raw, "://")
	return scheme + "://" + auth[:at+1] + first + rest[end:]
}

// urlTLSMode reads the TLS setting a connection string carries. The second
// result is true only when the string explicitly turned TLS off.
func urlTLSMode(engine, scheme string, q url.Values) (string, bool) {
	falsy := func(v string) bool {
		switch strings.ToLower(v) {
		case "false", "0", "no", "disable", "disabled":
			return true
		}
		return false
	}
	truthy := func(v string) bool {
		switch strings.ToLower(v) {
		case "true", "1", "yes", "required", "require":
			return true
		}
		return false
	}
	switch engine {
	case EnginePostgres:
		switch m := strings.ToLower(q.Get("sslmode")); m {
		case TLSDisable:
			return TLSDisable, true
		case TLSRequire, TLSVerifyCA, TLSVerifyFull:
			return m, false
		case "allow", TLSPrefer:
			return TLSPrefer, false
		}
		return TLSRequire, false
	case EngineMySQL, EngineMariaDB:
		v := strings.ToLower(firstNonEmpty(q.Get("ssl-mode"), q.Get("sslmode")))
		switch v {
		case "disabled", TLSDisable:
			return TLSDisable, true
		case "verify_ca", TLSVerifyCA:
			return TLSVerifyCA, false
		case "verify_identity", TLSVerifyFull:
			return TLSVerifyFull, false
		}
		if falsy(firstNonEmpty(q.Get("ssl"), q.Get("tls"))) {
			return TLSDisable, true
		}
		return TLSRequire, false
	case EngineMongoDB:
		v := firstNonEmpty(q.Get("tls"), q.Get("ssl"))
		if falsy(v) && v != "" {
			return TLSDisable, true
		}
		if strings.HasSuffix(scheme, "+srv") || truthy(v) {
			return TLSRequire, false
		}
		return TLSDisable, false
	case EngineRedis:
		if scheme == "rediss" {
			return TLSRequire, false
		}
		return TLSDisable, false
	}
	return TLSRequire, false
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

var tlsRank = map[string]int{TLSDisable: 0, TLSPrefer: 1, TLSRequire: 2, TLSVerifyCA: 3, TLSVerifyFull: 4}

// strongerTLS lifts the mode to the preset's stricter one. The caller skips
// it when the connection string explicitly turned TLS off.
func strongerTLS(fromURL, preset string) string {
	if tlsRank[preset] <= tlsRank[fromURL] {
		return fromURL
	}
	return preset
}

// PresetTLSMode is the TLS mode to use when a request names a hosting
// preset and gives none. Generic and unknown providers return "".
func PresetTLSMode(provider string) string {
	if p, ok := ProviderByID(provider); ok && provider != ProviderGeneric {
		return p.TLSMode
	}
	return ""
}

// SuggestName derives a record name from a host name.
func SuggestName(provider, host string) string {
	label, _, _ := strings.Cut(strings.ToLower(host), ".")
	base := label
	if provider != "" && provider != ProviderGeneric {
		base = provider + "-" + label
	}
	var b strings.Builder
	for _, ch := range base {
		switch {
		case ch >= 'a' && ch <= 'z', ch >= '0' && ch <= '9':
			b.WriteRune(ch)
		default:
			b.WriteByte('-')
		}
	}
	name := strings.Trim(b.String(), "-")
	if len(name) > 63 {
		name = strings.Trim(name[:63], "-")
	}
	return name
}
