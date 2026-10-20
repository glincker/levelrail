// Package extdb connects to databases this platform does not run: validating
// their addresses, probing them read-only, and opening them for the viewer
// through short-lived helper containers. It never writes to a remote database.
package extdb

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"
)

// Engine ids, identical to the managed engine identifiers.
const (
	EnginePostgres = "postgres"
	EngineMySQL    = "mysql"
	EngineMariaDB  = "mariadb"
	EngineMongoDB  = "mongodb"
	EngineRedis    = "redis"
)

// TLS modes an operator can pick.
const (
	TLSDisable = "disable"
	TLSPrefer  = "prefer"
	TLSRequire = "require"
)

// AllowLinkLocalEnv opts in to link-local and cloud metadata addresses.
const AllowLinkLocalEnv = "APP_EXTERNAL_DB_ALLOW_LINK_LOCAL"

const defaultAuthDB = "admin"

var (
	nameRe  = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$`)
	hostRe  = regexp.MustCompile(`^[A-Za-z0-9]([A-Za-z0-9._-]*[A-Za-z0-9])?$`)
	identRe = regexp.MustCompile(`^[A-Za-z0-9_.@$-]{1,128}$`)
	netRe   = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,127}$`)

	metadataPrefixes = []netip.Prefix{
		netip.MustParsePrefix("169.254.0.0/16"),
		netip.MustParsePrefix("fe80::/10"),
		netip.MustParsePrefix("fd00:ec2::/32"),
		netip.MustParsePrefix("100.100.100.200/32"),
	}
	metadataNames = map[string]bool{"metadata.google.internal": true, "metadata": true}
)

// ErrBlockedAddress marks a host the address policy forbids.
var ErrBlockedAddress = errors.New("extdb: address is not allowed")

// Policy decides which hosts may be connected to.
type Policy struct {
	AllowLinkLocal bool
}

// PolicyFromEnv reads the opt-in from the environment.
func PolicyFromEnv() Policy {
	v := strings.ToLower(strings.TrimSpace(os.Getenv(AllowLinkLocalEnv)))
	return Policy{AllowLinkLocal: v == "1" || v == "true" || v == "yes"}
}

// Conn is everything needed to reach one database. Password is held in
// memory only; the stored record never carries it.
type Conn struct {
	Engine          string
	Host            string
	Port            int
	User            string
	Password        string
	Database        string
	AuthDatabase    string
	TLSMode         string
	Network         string
	SourceContainer string
}

// Supported reports whether engine can be connected to.
func Supported(engine string) bool {
	switch engine {
	case EnginePostgres, EngineMySQL, EngineMariaDB, EngineMongoDB, EngineRedis:
		return true
	}
	return false
}

// DefaultPort is the engine's well known port.
func DefaultPort(engine string) int {
	switch engine {
	case EnginePostgres:
		return 5432
	case EngineMySQL, EngineMariaDB:
		return 3306
	case EngineMongoDB:
		return 27017
	case EngineRedis:
		return 6379
	}
	return 0
}

// DefaultTLSMode is the sensible default for engine.
func DefaultTLSMode(engine string) string {
	switch engine {
	case EnginePostgres, EngineMySQL, EngineMariaDB:
		return TLSPrefer
	}
	return TLSDisable
}

// ValidName reports whether name is usable as a record name.
func ValidName(name string) bool { return nameRe.MatchString(name) }

// Validate normalizes c in place and reports the first problem. It is pure:
// no DNS lookups happen here, see CheckResolved.
func (c *Conn) Validate(p Policy) error {
	if !Supported(c.Engine) {
		return fmt.Errorf("engine %q is not supported, use one of postgres, mysql, mariadb, mongodb, redis", c.Engine)
	}
	c.Host = strings.TrimSpace(c.Host)
	if err := checkHost(c.Host, p); err != nil {
		return err
	}
	if c.Port == 0 {
		c.Port = DefaultPort(c.Engine)
	}
	if c.Port < 1 || c.Port > 65535 {
		return errors.New("port must be between 1 and 65535")
	}
	switch c.TLSMode {
	case "":
		c.TLSMode = DefaultTLSMode(c.Engine)
	case TLSDisable, TLSPrefer, TLSRequire:
	default:
		return fmt.Errorf("tls_mode %q is not valid, use disable, prefer or require", c.TLSMode)
	}
	for field, v := range map[string]string{"user": c.User, "database": c.Database, "auth_database": c.AuthDatabase} {
		if v != "" && !identRe.MatchString(v) {
			return fmt.Errorf("%s contains characters that are not allowed", field)
		}
	}
	if c.Network != "" && !netRe.MatchString(c.Network) {
		return errors.New("network contains characters that are not allowed")
	}
	if c.SourceContainer != "" && !netRe.MatchString(c.SourceContainer) {
		return errors.New("source container contains characters that are not allowed")
	}
	if strings.ContainsAny(c.Password, "\x00\r\n") {
		return errors.New("password contains characters that are not allowed")
	}
	if c.AuthDatabase == "" && c.Engine == EngineMongoDB {
		c.AuthDatabase = defaultAuthDB
	}
	return nil
}

func checkHost(host string, p Policy) error {
	if host == "" {
		return errors.New("host is required")
	}
	if len(host) > 253 {
		return errors.New("host is too long")
	}
	lower := strings.ToLower(host)
	if lower == "localhost" || strings.HasSuffix(lower, ".localhost") {
		return fmt.Errorf("%w: localhost points at the helper container itself, use the container name, its network address or the host's LAN address", ErrBlockedAddress)
	}
	if addr, err := netip.ParseAddr(strings.Trim(host, "[]")); err == nil {
		return checkAddr(addr, p)
	}
	if !hostRe.MatchString(host) {
		return errors.New("host must be a hostname or IP address")
	}
	if metadataNames[lower] && !p.AllowLinkLocal {
		return fmt.Errorf("%w: %s is a cloud metadata name (set %s=true to permit)", ErrBlockedAddress, host, AllowLinkLocalEnv)
	}
	return nil
}

func checkAddr(addr netip.Addr, p Policy) error {
	addr = addr.Unmap()
	switch {
	case !addr.IsValid() || addr.IsUnspecified() || addr.IsMulticast():
		return fmt.Errorf("%w: %s is not a connectable address", ErrBlockedAddress, addr)
	case addr.IsLoopback():
		return fmt.Errorf("%w: %s is loopback and points at the helper container itself, use the container name, its network address or the host's LAN address", ErrBlockedAddress, addr)
	}
	if p.AllowLinkLocal {
		return nil
	}
	for _, pre := range metadataPrefixes {
		if pre.Contains(addr) {
			return fmt.Errorf("%w: %s is a link-local or cloud metadata address (set %s=true to permit)", ErrBlockedAddress, addr, AllowLinkLocalEnv)
		}
	}
	return nil
}

// Resolver looks a hostname up; net.DefaultResolver satisfies it.
type Resolver interface {
	LookupHost(ctx context.Context, host string) ([]string, error)
}

// CheckResolved rejects a hostname that resolves to a forbidden address.
// A lookup failure is not an error: container names only resolve inside
// Docker networks.
func (c Conn) CheckResolved(ctx context.Context, p Policy, r Resolver) error {
	if _, err := netip.ParseAddr(strings.Trim(c.Host, "[]")); err == nil {
		return nil
	}
	if r == nil {
		r = net.DefaultResolver
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	addrs, err := r.LookupHost(ctx, c.Host)
	if err != nil {
		return nil
	}
	for _, a := range addrs {
		ip, perr := netip.ParseAddr(a)
		if perr != nil {
			continue
		}
		if cerr := checkAddr(ip, p); cerr != nil {
			return fmt.Errorf("host %s resolves to a forbidden address: %w", c.Host, cerr)
		}
	}
	return nil
}

// Scrub replaces the password, raw or URL-escaped, with a placeholder.
func (c Conn) Scrub(text string) string {
	if c.Password == "" {
		return text
	}
	for _, form := range []string{c.Password, url.QueryEscape(c.Password), url.PathEscape(c.Password)} {
		text = strings.ReplaceAll(text, form, "[redacted]")
	}
	return text
}
