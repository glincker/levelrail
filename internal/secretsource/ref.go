package secretsource

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
)

const scheme = "secret://"

// Limits caps reference parts. Zero fields fall back to env, then defaults.
type Limits struct {
	MaxRef      int
	MaxConn     int
	MaxPath     int
	MaxSegments int
	MaxKey      int
}

func envInt(name string, def int) int {
	if v, err := strconv.Atoi(os.Getenv(name)); err == nil && v > 0 {
		return v
	}
	return def
}

// LimitsFromEnv reads APP_SECRETREF_MAX_{REF,CONN,PATH,SEGMENTS,KEY}, the seed for the runtime settings.
func LimitsFromEnv() Limits {
	return Limits{
		MaxRef:      envInt("APP_SECRETREF_MAX_REF", 1024),
		MaxConn:     envInt("APP_SECRETREF_MAX_CONN", 63),
		MaxPath:     envInt("APP_SECRETREF_MAX_PATH", 512),
		MaxSegments: envInt("APP_SECRETREF_MAX_SEGMENTS", 16),
		MaxKey:      envInt("APP_SECRETREF_MAX_KEY", 128),
	}
}

var limitsSource atomic.Pointer[func() Limits]

// SetLimitsSource makes the runtime settings the limit source; nil goes back to env.
func SetLimitsSource(f func() Limits) {
	if f == nil {
		limitsSource.Store(nil)
		return
	}
	limitsSource.Store(&f)
}

// CurrentLimits is the live limit set: the runtime source when wired, else env then defaults.
func CurrentLimits() Limits {
	if f := limitsSource.Load(); f != nil {
		return (*f)()
	}
	return LimitsFromEnv()
}

// Ref points at one secret value in one named connection. It holds no value.
type Ref struct {
	Conn    string
	Path    string
	Key     string
	Version int // 0 means latest
}

// String is the canonical URI form.
func (r Ref) String() string {
	s := scheme + r.Conn + "/" + r.Path + "#" + r.Key
	if r.Version > 0 {
		s += "?version=" + strconv.Itoa(r.Version)
	}
	return s
}

// Spec is the YAML/JSON object form: {secret: {conn, path, key, version}}.
type Spec struct {
	Secret RefSpec `json:"secret" yaml:"secret"`
}

// RefSpec carries the parts of the object form.
type RefSpec struct {
	Conn    string `json:"conn" yaml:"conn"`
	Path    string `json:"path" yaml:"path"`
	Key     string `json:"key" yaml:"key"`
	Version int    `json:"version,omitempty" yaml:"version,omitempty"`
}

// Ref validates the object form with the same rules as the URI form.
func (s Spec) Ref() (Ref, error) { return s.Secret.Ref() }

// Ref validates the parts with the current limits.
func (s RefSpec) Ref() (Ref, error) { return s.RefWith(CurrentLimits()) }

// RefWith validates the parts against explicit limits.
func (s RefSpec) RefWith(l Limits) (Ref, error) {
	if s.Version < 0 {
		return Ref{}, invalid("version must not be negative")
	}
	r := Ref(s)
	if err := r.ValidateWith(l); err != nil {
		return Ref{}, err
	}
	return r, nil
}

// IsRef reports whether s looks like a secret:// reference.
func IsRef(s string) bool { return strings.HasPrefix(s, scheme) }

// ParseRef parses secret://conn/path#key[?version=N] using the current limits.
func ParseRef(s string) (Ref, error) { return ParseRefWith(s, CurrentLimits()) }

// ParseRefWith parses against explicit limits. Errors never echo the input
// beyond a short reason, since a mistyped value may be a secret itself.
func ParseRefWith(s string, l Limits) (Ref, error) {
	if len(s) > l.MaxRef {
		return Ref{}, invalid("reference too long")
	}
	if !strings.HasPrefix(s, scheme) {
		return Ref{}, invalid("missing secret:// scheme")
	}
	rest := s[len(scheme):]
	loc, frag, ok := strings.Cut(rest, "#")
	if !ok || strings.Contains(frag, "#") {
		return Ref{}, invalid("exactly one #key is required")
	}
	if strings.ContainsAny(loc, "?") {
		return Ref{}, invalid("query is only allowed after #key")
	}
	key, query, hasQuery := strings.Cut(frag, "?")
	version := 0
	if hasQuery {
		v, found := strings.CutPrefix(query, "version=")
		n, err := strconv.Atoi(v)
		if !found || err != nil || n < 1 || strconv.Itoa(n) != v {
			return Ref{}, invalid("query must be version=N with N >= 1")
		}
		version = n
	}
	conn, path, ok := strings.Cut(loc, "/")
	if !ok {
		return Ref{}, invalid("path is required")
	}
	r := Ref{Conn: conn, Path: path, Key: key, Version: version}
	if err := r.ValidateWith(l); err != nil {
		return Ref{}, err
	}
	return r, nil
}

// Validate checks the parts with the current limits.
func (r Ref) Validate() error { return r.ValidateWith(CurrentLimits()) }

// ValidateWith checks every part. Connection names are lowercase only, so
// two spellings can never name two connections. Paths and keys keep case.
func (r Ref) ValidateWith(l Limits) error {
	if err := ValidConnName(r.Conn, l.MaxConn); err != nil {
		return err
	}
	if r.Path == "" || len(r.Path) > l.MaxPath {
		return invalid("path empty or too long")
	}
	segs := strings.Split(r.Path, "/")
	if len(segs) > l.MaxSegments {
		return invalid("too many path segments")
	}
	for _, seg := range segs {
		if seg == "" || seg == "." || seg == ".." || !allChars(seg, isPathChar) {
			return invalid("path segment empty, dot, traversal or has a disallowed character")
		}
	}
	if r.Key == "" || len(r.Key) > l.MaxKey || !allChars(r.Key, isPathChar) || r.Key == "." || r.Key == ".." {
		return invalid("key empty, too long or has a disallowed character")
	}
	if r.Version < 0 {
		return invalid("version must not be negative")
	}
	return nil
}

// ValidConnName enforces [a-z0-9][a-z0-9_-]* up to max bytes.
func ValidConnName(name string, maxLen int) error {
	if name == "" || len(name) > maxLen {
		return invalid("connection name empty or too long")
	}
	for i := 0; i < len(name); i++ {
		c := name[i]
		lower := c >= 'a' && c <= 'z'
		digit := c >= '0' && c <= '9'
		if !lower && !digit && (i == 0 || (c != '-' && c != '_')) {
			return invalid("connection name must be lowercase [a-z0-9_-] starting alphanumeric")
		}
	}
	return nil
}

// '%' is excluded so encoded traversal like %2e%2e is never decoded later.
func isPathChar(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' ||
		c == '-' || c == '_' || c == '.' || c == '@' || c == '+' || c == '='
}

func allChars(s string, ok func(byte) bool) bool {
	for i := 0; i < len(s); i++ {
		if !ok(s[i]) {
			return false
		}
	}
	return true
}

func invalid(reason string) error {
	return fmt.Errorf("%w: %s", ErrInvalidRef, reason)
}
