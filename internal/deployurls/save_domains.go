package deployurls

import (
	"context"

	"github.com/GLINCKER/levelrail/internal/store"
)

// saveDomains writes the copy's domains with a revision check when the store can re-read,
// so an env edit made since the list snapshot is kept; only Domains is re-applied.
func (s *Syncer) saveDomains(ctx context.Context, svc store.DesiredService) error {
	g, ok := s.Store.(interface {
		GetDesiredService(ctx context.Context, name string) (*store.DesiredService, error)
	})
	if !ok {
		return s.Store.SaveDesiredService(ctx, svc)
	}
	_, err := store.SaveRebased(ctx, svc, g.GetDesiredService, s.Store.SaveDesiredService, func(fresh store.DesiredService) store.DesiredService {
		fresh.Domains = svc.Domains
		return fresh
	})
	return err
}
