package secretsource

import (
	"context"
	"time"
)

// Value is a resolved secret. Its formatting verbs print a mask so a stray
// log line or %v cannot leak it; use Reveal at the single point of use.
type Value struct {
	data      []byte
	Version   string
	FetchedAt time.Time
	TTL       time.Duration // 0 means do not cache
}

// NewValue copies data into a Value.
func NewValue(data []byte, version string) Value {
	return Value{data: append([]byte(nil), data...), Version: version, FetchedAt: time.Now()}
}

// Reveal returns a copy of the secret bytes.
func (v Value) Reveal() []byte { return append([]byte(nil), v.data...) }

// Len is the byte length, safe to log.
func (v Value) Len() int { return len(v.data) }

// String masks the value.
func (v Value) String() string { return "[redacted]" }

// GoString masks the value for %#v.
func (v Value) GoString() string { return "secretsource.Value{[redacted]}" }

// Lease identifies a renewable dynamic credential.
type Lease struct {
	ID      string
	Expires time.Time
}

// Capabilities lets callers skip unsupported calls without probing.
type Capabilities struct {
	Renew    bool // provider implements Renewer
	Versions bool // honors Ref.Version
	Dynamic  bool // returns leased values
}

// Provider adapts one secret backend. Implementations should return the
// package sentinels (or wrap them) and never include secret values in errors.
type Provider interface {
	Name() string
	Capabilities() Capabilities
	Resolve(ctx context.Context, ref Ref) (Value, error)
}

// Renewer is the optional lease renewal side of a Provider.
type Renewer interface {
	Renew(ctx context.Context, l Lease) (Lease, error)
}
