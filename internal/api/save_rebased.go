package api

import (
	"context"

	"github.com/GLINCKER/levelrail/internal/store"
)

// saveOwned saves next with a revision check and, when a concurrent write wins,
// re-applies only the caller's own fields onto the fresh row, so env is never rolled back.
func (rt *Router) saveOwned(ctx context.Context, next store.DesiredService, rebase func(fresh store.DesiredService) store.DesiredService) (store.DesiredService, error) {
	return store.SaveRebased(ctx, next, rt.apps.GetDesiredService, rt.apps.SaveDesiredService, rebase)
}
