package api

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/internal/alerting"
	"github.com/GLINCKER/levelrail/internal/dnszones"
)

// dnsCredentialCache keeps the last scheduled access check so alert ticks
// reuse it instead of calling every provider each time.
type dnsCredentialCache struct {
	mu      sync.Mutex
	checked time.Time
	results []alerting.DNSCredentialHealth
}

// DNSCredentialHealth re-runs the read-only access check for every connected
// DNS provider, at most once per maxAge. It satisfies alerting.DNSCredentialSource.
func (rt *Router) DNSCredentialHealth(ctx context.Context, maxAge time.Duration) ([]alerting.DNSCredentialHealth, error) {
	c := &rt.dnsCredCache
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.checked.IsZero() && time.Since(c.checked) < maxAge {
		return c.results, nil
	}
	providers, err := rt.zoneProviders(ctx)
	if err != nil {
		return nil, fmt.Errorf("api: resolve dns providers for health check: %w", err)
	}
	out := make([]alerting.DNSCredentialHealth, 0, len(providers))
	for _, name := range providerNames(providers) {
		vctx, cancel := context.WithTimeout(ctx, dnsVerifyTimeout)
		rep := dnszones.VerifyAccess(vctx, providers[name])
		cancel()
		out = append(out, alerting.DNSCredentialHealth{Provider: name, Outcome: rep.Outcome, Missing: rep.Missing})
	}
	c.checked, c.results = time.Now(), out
	return out, nil
}
