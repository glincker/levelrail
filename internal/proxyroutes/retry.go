package proxyroutes

import (
	"context"
	"errors"
	"fmt"
	"time"
)

const defaultRetryGap = 2 * time.Second

// ErrRetryUnsupported is returned for providers where a file change does not
// make the proxy re-attempt a certificate request.
var ErrRetryUnsupported = errors.New("retrying a certificate request is only supported for Traefik file routes")

// Retry makes Traefik re-attempt a failed certificate request for domain. It
// rewrites the route file with a router priority one higher, waits RetryGap,
// then restores the normal file. Traefik ignores a reload whose parsed
// configuration is unchanged, so a comment would not work; a priority change
// does, and the restore leaves the stored route state untouched.
func (s *Syncer) Retry(ctx context.Context, domain string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, err := s.Plan(ctx)
	if err != nil {
		return err
	}
	if p.Provider.Kind() != KindTraefik || !p.Settings.Enabled() {
		return ErrRetryUnsupported
	}
	if p.Dir == nil {
		return p.DirErr
	}
	path := PathFromSettings(p.Settings.UpstreamHost, s.Unit)
	for _, it := range p.Items {
		if it.Domain != domain {
			continue
		}
		if it.Err != nil {
			return it.Err
		}
		prio := s.RouterPriority
		if prio == 0 {
			prio = DefaultRouterPriority
		}
		if prio < MaxRouterPriority {
			prio++
		} else {
			prio--
		}
		nudge := s.renderPrio(p.Provider, p.Settings, path, it.Target, prio)
		if nudge.Err != nil {
			return nudge.Err
		}
		if err := p.Dir.Write(it.File, nudge.Content); err != nil {
			return fmt.Errorf("proxyroutes: retry %s: %w", domain, err)
		}
		gap := s.RetryGap
		if gap <= 0 {
			gap = defaultRetryGap
		}
		RecordRetry(domain, time.Now())
		select {
		case <-ctx.Done():
		case <-time.After(gap):
		}
		// Restore even when the caller gave up, or the odd priority would stay.
		if err := p.Dir.Write(it.File, it.Content); err != nil {
			return fmt.Errorf("proxyroutes: restore %s: %w", domain, err)
		}
		return nil
	}
	return fmt.Errorf("proxyroutes: %s is not a routed domain", domain)
}
