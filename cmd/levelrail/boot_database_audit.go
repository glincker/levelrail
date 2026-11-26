package main

import (
	"context"
	"log/slog"
	"time"

	"github.com/GLINCKER/levelrail/internal/api"
	"github.com/GLINCKER/levelrail/internal/dbaudit"
	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	dbAuditSweepInterval = time.Hour
	dbAuditSweepFirst    = 30 * time.Second
	dbAuditFlushEvery    = time.Second
)

// startDatabaseAudit turns on the explorer statement audit and its retention sweeper.
// The returned stop flushes the queue; it is a no-op when settings are unavailable.
func startDatabaseAudit(ctx context.Context, rt *api.Router, db *store.DB, logger *slog.Logger) (stop func()) {
	svc := rt.RuntimeSettings()
	if svc == nil {
		return func() {}
	}
	set := dbaudit.ServiceSettings{Svc: svc}
	w := dbaudit.New(db, set, dbaudit.Options{FlushInterval: dbAuditFlushEvery, Logger: logger})
	w.Start()
	rt.WithDatabaseAudit(w)
	go dbaudit.RunSweeper(ctx, db, set, dbAuditSweepFirst, dbAuditSweepInterval, logger)
	// Close is idempotent, so a caller stop and the context end can both run it.
	context.AfterFunc(ctx, w.Close)
	return w.Close
}
