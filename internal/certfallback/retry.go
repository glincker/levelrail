package certfallback

import (
	"context"
	"log/slog"
	"net"
	"strings"
	"sync"
	"time"
)

// Retrier periodically drops the fallback certificate of domains that now
// resolve to this server, so certmagic requests a public certificate again.
type Retrier struct {
	Store      Store
	Candidates func(ctx context.Context) []string // ACME-owned domains only
	Resolves   func(ctx context.Context, domain string) bool
	Interval   func(ctx context.Context) time.Duration
	MaxRetries func(ctx context.Context) int
	Audit      func(ctx context.Context, domain, detail string)
	Nudge      func()
	// Evict clears the live certificate cache after a drop so the new cert is served without a reload.
	Evict  func(ctx context.Context) error
	Logger *slog.Logger

	mu       sync.Mutex
	attempts map[string]int
}

// Pass runs one retry round and returns the domains it dropped a fallback for.
func (r *Retrier) Pass(ctx context.Context) ([]string, error) {
	held, err := Held(ctx, r.Store)
	if err != nil {
		return nil, err
	}
	r.mu.Lock()
	if r.attempts == nil {
		r.attempts = map[string]int{}
	}
	for d := range r.attempts {
		if _, ok := held[d]; !ok {
			delete(r.attempts, d) // issued, or gone: start fresh next time
		}
	}
	r.mu.Unlock()

	limit := r.MaxRetries(ctx)
	var dropped []string
	for _, d := range r.Candidates(ctx) {
		d = strings.ToLower(strings.TrimSpace(d))
		if _, ok := held[d]; !ok {
			continue
		}
		r.mu.Lock()
		n := r.attempts[d]
		r.mu.Unlock()
		if n >= limit || !r.Resolves(ctx, d) {
			continue
		}
		if _, err := Drop(ctx, r.Store, d); err != nil {
			r.log().Warn("certfallback: drop failed", slog.String("domain", d), slog.String("error", err.Error()))
			continue
		}
		r.mu.Lock()
		r.attempts[d] = n + 1
		r.mu.Unlock()
		if r.Audit != nil {
			r.Audit(ctx, d, "scheduled retry dropped the local-issuer fallback certificate")
		}
		dropped = append(dropped, d)
	}
	if len(dropped) > 0 && r.Evict != nil {
		if err := r.Evict(ctx); err != nil {
			r.log().Warn("certfallback: cache evict failed", slog.String("error", err.Error()))
		}
	}
	if len(dropped) > 0 && r.Nudge != nil {
		r.Nudge()
	}
	return dropped, nil
}

// Run loops Pass at the live interval until ctx ends.
func (r *Retrier) Run(ctx context.Context) {
	for {
		wait := r.Interval(ctx)
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}
		if _, err := r.Pass(ctx); err != nil {
			r.log().Warn("certfallback: retry pass failed", slog.String("error", err.Error()))
		}
	}
}

func (r *Retrier) log() *slog.Logger {
	if r.Logger != nil {
		return r.Logger
	}
	return slog.Default()
}

// ResolvesHere reports whether domain resolves to any of serverIPs.
func ResolvesHere(ctx context.Context, domain string, serverIPs []string) bool {
	addrs, err := net.DefaultResolver.LookupIPAddr(ctx, domain)
	if err != nil {
		return false
	}
	for _, a := range addrs {
		for _, ip := range serverIPs {
			if a.IP.String() == ip {
				return true
			}
		}
	}
	return false
}
