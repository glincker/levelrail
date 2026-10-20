package ingress

import (
	"context"
	"log/slog"
	"sort"
	"sync"

	"github.com/GLINCKER/levelrail/internal/store"
)

const reasonDomainsDropped = "DomainsDropped"

// domainIssues collects the per-domain problems one reconcile pass finds.
type domainIssues struct {
	mu    sync.Mutex
	items map[string]store.DomainCondition
}

func (d *domainIssues) reset() {
	d.mu.Lock()
	d.items = map[string]store.DomainCondition{}
	d.mu.Unlock()
}

func (d *domainIssues) note(domain, code, severity, message string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.items == nil {
		d.items = map[string]store.DomainCondition{}
	}
	key := domain + "\x00" + code
	d.items[key] = store.DomainCondition{Domain: domain, Code: code, Severity: severity, Message: message}
}

func (d *domainIssues) has(domain, severity string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, it := range d.items {
		if it.Domain == domain && it.Severity == severity {
			return true
		}
	}
	return false
}

func (d *domainIssues) snapshot() []store.DomainCondition {
	d.mu.Lock()
	out := make([]store.DomainCondition, 0, len(d.items))
	for _, it := range d.items {
		out = append(out, it)
	}
	d.mu.Unlock()
	sort.Slice(out, func(i, j int) bool {
		if out[i].Domain != out[j].Domain {
			return out[i].Domain < out[j].Domain
		}
		return out[i].Code < out[j].Code
	})
	return out
}

func (d *domainIssues) droppedHosts() []string {
	seen := map[string]bool{}
	var hosts []string
	for _, it := range d.snapshot() {
		if it.Severity == store.DomainConditionDropped && !seen[it.Domain] {
			seen[it.Domain] = true
			hosts = append(hosts, it.Domain)
		}
	}
	return hosts
}

// recordDomainConditions persists this pass's issues, an empty set included so
// a fixed domain clears.
func (c *Controller) recordDomainConditions(ctx context.Context) {
	if c.domainConds == nil {
		return
	}
	if err := c.domainConds.ReplaceDomainConditions(ctx, c.issues.snapshot()); err != nil {
		c.logger.WarnContext(ctx, "ingress: record domain conditions failed", slog.String("error", err.Error()))
	}
}
