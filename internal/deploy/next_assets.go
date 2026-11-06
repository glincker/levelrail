package deploy

import (
	"context"
	"sync"
	"time"
)

// deployIDFor is the stable per deploy id: the attempt id, else the commit.
func deployIDFor(req Request) string {
	if req.AttemptID != "" {
		return req.AttemptID
	}
	return req.CommitSHA
}

// NextAssetGrace records that a superseded Next.js release must keep serving
// its /_next/static and /_next/data requests until Until.
type NextAssetGrace struct {
	Service      string
	DeploymentID string
	Image        string
	Until        time.Time
}

// NextGraceBook is the deploy side bookkeeping. It is in memory on purpose:
// a durable table needs a migration, left to the ingress slice that consumes it.
type NextGraceBook struct {
	mu    sync.Mutex
	items map[string][]NextAssetGrace
}

// NewNextGraceBook returns an empty book.
func NewNextGraceBook() *NextGraceBook {
	return &NextGraceBook{items: map[string][]NextAssetGrace{}}
}

// Begin opens a grace window for the superseded release. It returns false when
// grace is disabled, the old release carried no deployment id, or the same
// release is already recorded.
func (b *NextGraceBook) Begin(ctx context.Context, service, oldDeploymentID, oldImage string, now time.Time) bool {
	window := nextAssetGrace(ctx)
	if window <= 0 || oldDeploymentID == "" {
		return false
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.prune(service, now)
	for _, g := range b.items[service] {
		if g.DeploymentID == oldDeploymentID {
			return false
		}
	}
	b.items[service] = append(b.items[service], NextAssetGrace{
		Service: service, DeploymentID: oldDeploymentID, Image: oldImage, Until: now.Add(window),
	})
	return true
}

// Active lists releases still in grace at now.
func (b *NextGraceBook) Active(service string, now time.Time) []NextAssetGrace {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.prune(service, now)
	return append([]NextAssetGrace(nil), b.items[service]...)
}

func (b *NextGraceBook) prune(service string, now time.Time) {
	kept := b.items[service][:0]
	for _, g := range b.items[service] {
		if g.Until.After(now) {
			kept = append(kept, g)
		}
	}
	b.items[service] = kept
}
