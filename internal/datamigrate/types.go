// Package datamigrate copies live data from a source database into a managed
// database here, and checks a cutover is safe. It never stores source credentials.
package datamigrate

import (
	"errors"
	"fmt"
	"net/netip"
	"net/url"
	"regexp"
	"strings"
)

// Copy statuses reported per database.
const (
	StatusPending     = "pending"
	StatusCopying     = "copying"
	StatusVerified    = "verified"
	StatusFailed      = "failed"
	StatusUnsupported = "unsupported"
)

// Source locates the database to copy from. Password is held in memory only.
type Source struct {
	Host     string `json:"host"`
	Port     int    `json:"port,omitempty"`
	User     string `json:"user,omitempty"`
	Password string `json:"password,omitempty"`
	Database string `json:"database,omitempty"`
	// AuthDatabase is MongoDB's authentication database, "admin" when empty.
	AuthDatabase string `json:"auth_database,omitempty"`
	TLS          bool   `json:"tls,omitempty"`
}

// TableCount is one table (or collection) compared across both sides.
type TableCount struct {
	Name   string `json:"name"`
	Source int64  `json:"source"`
	Target int64  `json:"target"`
	OK     bool   `json:"ok"`
}

// Verification is the outcome of comparing per-table row counts.
type Verification struct {
	Checked    int          `json:"checked"`
	Mismatched int          `json:"mismatched"`
	Tables     []TableCount `json:"tables,omitempty"`
}

// Engine ids this package can copy.
const (
	EnginePostgres = "postgres"
	EngineMySQL    = "mysql"
	EngineMariaDB  = "mariadb"
	EngineMongoDB  = "mongodb"
	EngineRedis    = "redis"
)

// Supported reports whether Copy can move engine.
func Supported(engine string) bool {
	switch engine {
	case EnginePostgres, EngineMySQL, EngineMariaDB, EngineMongoDB, EngineRedis:
		return true
	}
	return false
}

var (
	hostRe  = regexp.MustCompile(`^[A-Za-z0-9]([A-Za-z0-9._-]*[A-Za-z0-9])?$`)
	identRe = regexp.MustCompile(`^[A-Za-z0-9_.@$-]{1,128}$`)
)

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

// Validate checks the source is well formed for engine and fills the default port.
func (s *Source) Validate(engine string) error {
	if !Supported(engine) {
		return fmt.Errorf("copying data is not supported for engine %q", engine)
	}
	s.Host = strings.TrimSpace(s.Host)
	if s.Host == "" {
		return errors.New("host is required")
	}
	if addr, err := netip.ParseAddr(strings.Trim(s.Host, "[]")); err == nil {
		if addr.IsLinkLocalUnicast() || addr.IsUnspecified() || addr.IsMulticast() {
			return errors.New("host is a link-local, unspecified or multicast address")
		}
	} else if !hostRe.MatchString(s.Host) {
		return errors.New("host must be a hostname or IP address")
	}
	if s.Port == 0 {
		s.Port = DefaultPort(engine)
	}
	if s.Port < 1 || s.Port > 65535 {
		return errors.New("port must be between 1 and 65535")
	}
	for name, v := range map[string]string{"user": s.User, "database": s.Database, "auth_database": s.AuthDatabase} {
		if v != "" && !identRe.MatchString(v) {
			return fmt.Errorf("%s contains characters that are not allowed", name)
		}
	}
	if engine != EngineRedis && engine != EngineMongoDB && s.Database == "" {
		return errors.New("database is required")
	}
	return nil
}

// Scrub replaces the source password, raw or URL-escaped, with a placeholder.
func (s Source) Scrub(text string) string {
	if s.Password == "" {
		return text
	}
	for _, form := range []string{s.Password, url.QueryEscape(s.Password), url.PathEscape(s.Password)} {
		text = strings.ReplaceAll(text, form, "[redacted]")
	}
	return text
}
