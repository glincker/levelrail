package demo

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	// VisitorUserID is the read-only account every demo session signs in as.
	VisitorUserID = "user_demo_visitor"
	// VisitorUsername is the login name shown on the demo login form.
	VisitorUsername = "demo"
	// VisitorPassword is the published, read-only demo password.
	VisitorPassword = "demo-readonly"

	seededUserPrefix = "user_demo_"
	historyDays      = 14
)

var (
	// ErrNotEmpty is returned when the target store already holds users but no demo marker.
	ErrNotEmpty = errors.New("demo: refusing to seed a store that already holds users")
	// ErrSeedMismatch is returned when the store was seeded with different options.
	ErrSeedMismatch = errors.New("demo: store was seeded with a different seed or anchor")
)

// IsSeededUserID reports whether id belongs to a user the seeder creates.
func IsSeededUserID(id string) bool { return strings.HasPrefix(id, seededUserPrefix) }

// Options controls a seed run. The same Seed, Anchor and DomainSuffix always
// produce the same rows.
type Options struct {
	Seed   int64
	Anchor time.Time
	// DomainSuffix is appended to every seeded hostname; defaults to a reserved example domain.
	DomainSuffix string
}

// Manifest names what a seed produced, for the generator and fakes.
type Manifest struct {
	Services  []string
	Databases []string
	Nodes     []string
}

// Seed populates an empty store with the fictional demo estate and writes the
// marker last. Re-running with identical options is a no-op; a data dir that
// failed halfway must be discarded, not retried.
func Seed(ctx context.Context, db *store.DB, opts Options) (Manifest, error) {
	if opts.Anchor.IsZero() {
		return Manifest{}, errors.New("demo: seed anchor is required")
	}
	opts.Anchor = opts.Anchor.UTC()
	if opts.DomainSuffix == "" {
		opts.DomainSuffix = "demo.example.com"
	}
	m := planManifest()

	marker, err := db.GetDemoMarker(ctx)
	switch {
	case err == nil:
		if marker.Seed != opts.Seed || !marker.Anchor.Equal(opts.Anchor) {
			return Manifest{}, ErrSeedMismatch
		}
		return m, nil
	case !errors.Is(err, store.ErrDemoMarkerNotFound):
		return Manifest{}, fmt.Errorf("demo: read marker: %w", err)
	}

	n, err := db.CountUsers(ctx)
	if err != nil {
		return Manifest{}, fmt.Errorf("demo: count users: %w", err)
	}
	if n > 0 {
		return Manifest{}, ErrNotEmpty
	}

	s := &seeder{ctx: ctx, db: db, opts: opts, rng: newRNG(opts.Seed)}
	steps := []func() error{s.users, s.organization, s.nodes, s.services, s.deploys, s.readiness, s.databases, s.audit, s.statusPage, s.teams, s.approvals, s.usage}
	for _, step := range steps {
		if err := step(); err != nil {
			return Manifest{}, err
		}
	}
	if err := db.SaveDemoMarker(ctx, store.DemoMarker{Seed: opts.Seed, Anchor: opts.Anchor, SeededAt: opts.Anchor}); err != nil {
		return Manifest{}, err
	}
	return m, nil
}

func planManifest() Manifest {
	var m Manifest
	for _, a := range appSpecs {
		m.Services = append(m.Services, a.name)
	}
	for _, d := range databaseSpecs {
		m.Databases = append(m.Databases, d.name)
	}
	for _, n := range nodeSpecs {
		m.Nodes = append(m.Nodes, n.id)
	}
	return m
}

type seeder struct {
	ctx  context.Context
	db   *store.DB
	opts Options
	rng  *rng
	// lastOK maps an app to its newest succeeded attempt, set by deploys.
	lastOK map[string]okAttempt
}

func (s *seeder) at(daysAgo int, extra time.Duration) time.Time {
	return s.opts.Anchor.AddDate(0, 0, -daysAgo).Add(extra)
}

func (s *seeder) stamp(t time.Time) string { return t.UTC().Format(time.RFC3339Nano) }

// rng is a tiny splitmix64 generator so seeded output never depends on the
// standard library's generator implementation.
type rng struct{ state uint64 }

func newRNG(seed int64) *rng {
	return &rng{state: uint64(seed) ^ 0x9e3779b97f4a7c15} //nolint:gosec // bit reinterpretation of the seed, not a size conversion
}

func (r *rng) next() uint64 {
	r.state += 0x9e3779b97f4a7c15
	z := r.state
	z = (z ^ (z >> 30)) * 0xbf58476d1ce4e5b9
	z = (z ^ (z >> 27)) * 0x94d049bb133111eb
	return z ^ (z >> 31)
}

// intn returns a value in [0, n).
func (r *rng) intn(n int) int {
	if n <= 0 {
		return 0
	}
	return int(r.next() % uint64(n)) //nolint:gosec // n is positive and small, result is below n
}

func (r *rng) hex(n int) string {
	const digits = "0123456789abcdef"
	b := make([]byte, n)
	for i := range b {
		b[i] = digits[r.intn(len(digits))]
	}
	return string(b)
}
