package api

import (
	"context"

	"github.com/glincker/levelrail/internal/store"
)

// saveComposeService writes one compose member from the file, but with a revision check against
// the previous row so an env edit made while the deploy ran is merged in, not overwritten.
func (rt *Router) saveComposeService(ctx context.Context, save func(context.Context, store.DesiredService) error, svc store.DesiredService, previous []store.DesiredService) error {
	for _, p := range previous {
		if p.Name != svc.Name {
			continue
		}
		svc.Rev = p.Rev
		_, err := store.SaveRebased(ctx, svc, rt.apps.GetDesiredService, save, func(fresh store.DesiredService) store.DesiredService {
			return store.RebaseEnvColumns(p, svc, fresh)
		})
		return err
	}
	return save(ctx, svc)
}
