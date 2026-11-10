package ingress

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"syscall"
	"time"

	"github.com/GLINCKER/levelrail/internal/ingress"
	"github.com/GLINCKER/levelrail/internal/store"
)

// DomainTuningStore is optional on ServiceStore: a store without it leaves
// every route on the ingress defaults.
type DomainTuningStore interface {
	ListDomainTuning(ctx context.Context) ([]store.DomainTuning, error)
}

// TuningBounds are the live runtime-setting bounds; a zero field means unknown.
type TuningBounds struct {
	MinSeconds       int
	MaxSeconds       int
	MaxStreamSeconds int
}

// WithDomainTuningBounds makes a changed bound reshape tuning already stored.
func WithDomainTuningBounds(f func(ctx context.Context) TuningBounds) Option {
	return func(c *Controller) { c.domainTuningBounds = f }
}

func (c *Controller) applyDomainTuning(ctx context.Context, routes []ingress.ProxyRoute) ([]ingress.ProxyRoute, error) {
	ts, ok := c.store.(DomainTuningStore)
	if !ok {
		return routes, nil
	}
	rows, err := ts.ListDomainTuning(ctx)
	if err != nil || len(rows) == 0 {
		return routes, err
	}
	var b TuningBounds
	if c.domainTuningBounds != nil {
		b = c.domainTuningBounds(ctx)
	}
	return attachDomainTuning(routes, clampDomainTuning(rows, b)), nil
}

func clampSeconds(v, lo, hi int) int {
	if v <= 0 {
		return 0
	}
	if hi > 0 && lo > hi {
		lo = hi // the max wins when settings ever cross
	}
	if lo > 0 && v < lo {
		v = lo
	}
	if hi > 0 && v > hi {
		v = hi
	}
	return v
}

// clampDomainTuning keeps every stored value inside today's bounds, so a
// lowered ceiling also tightens rows saved under an older one.
func clampDomainTuning(rows []store.DomainTuning, b TuningBounds) []store.DomainTuning {
	out := make([]store.DomainTuning, len(rows))
	for i, t := range rows {
		t.DialTimeoutSeconds = clampSeconds(t.DialTimeoutSeconds, b.MinSeconds, b.MaxSeconds)
		t.ResponseHeaderTimeoutSeconds = clampSeconds(t.ResponseHeaderTimeoutSeconds, b.MinSeconds, b.MaxSeconds)
		t.StreamCloseDelaySeconds = clampSeconds(t.StreamCloseDelaySeconds, b.MinSeconds, b.MaxSeconds)
		t.StreamTimeoutSeconds = clampSeconds(t.StreamTimeoutSeconds, b.MinSeconds, b.MaxStreamSeconds)
		out[i] = t
	}
	return out
}

func secs(n int) time.Duration { return time.Duration(n) * time.Second }

// attachDomainTuning splits a tuned host onto its own route, the same way
// domain limits do, so sibling hosts keep their config untouched.
func attachDomainTuning(routes []ingress.ProxyRoute, rows []store.DomainTuning) []ingress.ProxyRoute {
	byDomain := make(map[string]store.DomainTuning, len(rows))
	for _, t := range rows {
		byDomain[t.Domain] = t
	}
	out := make([]ingress.ProxyRoute, 0, len(routes))
	for _, r := range routes {
		var rest []string
		for _, h := range r.Hosts {
			t, ok := byDomain[h]
			if !ok {
				rest = append(rest, h)
				continue
			}
			own := r
			own.Hosts = []string{h}
			own.Tuning = &ingress.DomainTuning{
				Compression: t.Compression, Streaming: t.Streaming,
				DialTimeout: secs(t.DialTimeoutSeconds), ResponseHeaderTimeout: secs(t.ResponseHeaderTimeoutSeconds),
				StreamTimeout: secs(t.StreamTimeoutSeconds), StreamCloseDelay: secs(t.StreamCloseDelaySeconds),
			}
			out = append(out, own)
		}
		if len(rest) > 0 {
			r.Hosts = rest
			out = append(out, r)
		}
	}
	return out
}

// applyWithTuningFallback applies cfg. When Caddy rejects a config that
// carries domain tuning, it applies the same config without any tuning so
// other domains keep their changes, and returns the tuned hosts it dropped.
func (c *Controller) applyWithTuningFallback(ctx context.Context, opts ingress.RoutesOptions, cfg *ingress.Config) (*ingress.Config, []string, error) {
	plain := make([]ingress.ProxyRoute, len(opts.Routes))
	var tuned []string
	for i, r := range opts.Routes {
		if r.Tuning != nil {
			tuned = append(tuned, r.Hosts...)
			r.Tuning = nil
		}
		plain[i] = r
	}
	if len(tuned) == 0 {
		return cfg, nil, c.driver.Apply(ctx, cfg)
	}
	hash := configHash(cfg)
	c.tuningMu.Lock()
	rejected := c.rejectedTuning
	c.tuningMu.Unlock()
	// An identical tuned config that Caddy already rejected is not loaded again until it changes.
	if hash == "" || hash != rejected {
		err := c.driver.Apply(ctx, cfg)
		if err == nil {
			c.setRejectedTuning("")
			return cfg, nil, nil
		}
		if transientApplyError(ctx, err) {
			return cfg, nil, err
		}
		c.setRejectedTuning(hash)
		c.logger.Warn("ingress: config with domain tuning was rejected, applying it without tuning",
			slog.Any("tuned_domains", tuned), slog.String("error", err.Error()))
	}
	opts.Routes = plain
	fallback, err := ingress.BuildRoutesConfig(opts)
	if err != nil {
		return cfg, nil, fmt.Errorf("build config without domain tuning: %w", err)
	}
	if err := c.driver.Apply(ctx, fallback); err != nil {
		return cfg, nil, fmt.Errorf("apply config without domain tuning: %w", err)
	}
	return fallback, tuned, nil
}

func (c *Controller) setRejectedTuning(hash string) {
	c.tuningMu.Lock()
	c.rejectedTuning = hash
	c.tuningMu.Unlock()
}

func configHash(cfg *ingress.Config) string {
	b, err := json.Marshal(cfg)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// transientApplyError is a failure that says nothing about the config itself,
// so dropping tuning would not help and must not happen.
func transientApplyError(ctx context.Context, err error) bool {
	if ctx.Err() != nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	if errors.Is(err, syscall.ECONNREFUSED) || errors.Is(err, syscall.ECONNRESET) {
		return true
	}
	var opErr *net.OpError
	if errors.As(err, &opErr) {
		return true
	}
	var netErr net.Error
	return errors.As(err, &netErr) && netErr.Timeout()
}
