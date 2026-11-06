package main

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/internal/deploy"
	"github.com/GLINCKER/levelrail/internal/reconcile/application"
	ingressreconcile "github.com/GLINCKER/levelrail/internal/reconcile/ingress"
	"github.com/GLINCKER/levelrail/internal/store"
)

var nextSkewOnce sync.Once

// nextSkewOptions attaches the durable grace book once and returns the
// reconciler and ingress options that read it.
func nextSkewOptions(ctx context.Context, db *store.DB, logger *slog.Logger) (application.Option, ingressreconcile.Option) {
	book := deploy.DefaultNextGraceBook()
	nextSkewOnce.Do(func() {
		if db == nil {
			return
		}
		if err := book.WithStore(db).Load(ctx, time.Now()); err != nil && logger != nil {
			logger.Warn("next grace: restore failed, windows start empty", slog.String("error", err.Error()))
		}
	})
	hold := application.WithNextGraceHold(book.HeldUntil)
	ids := ingressreconcile.WithNextGraceIDs(func(service string, now time.Time) []string {
		var out []string
		for _, g := range book.Active(service, now) {
			out = append(out, g.DeploymentID)
		}
		return out
	})
	return hold, ids
}
