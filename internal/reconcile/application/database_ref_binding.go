package application

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/glincker/levelrail/internal/store"
)

// errDatabaseRefRecreated is returned when a reference points at a database that is not the one it was written against.
var errDatabaseRefRecreated = errors.New("database was recreated or removed since this reference was written; re-attach the app to the database")

// errDatabaseRefTeam is returned for an unbound legacy reference whose database belongs to another team.
var errDatabaseRefTeam = errors.New("database belongs to another team; re-attach the app to a database it owns")

type resourceOwnerLookup interface {
	OwnerOfApp(ctx context.Context, name string) (*store.ResourceOwner, error)
	OwnerOfDatabase(ctx context.Context, name string) (*store.ResourceOwner, error)
}

// checkDatabaseRefBinding refuses a reference whose recorded database instance differs from the live one.
// A legacy reference without an instance is accepted only while both sides belong to the same team.
func (c *Controller) checkDatabaseRefBinding(ctx context.Context, dbName, instance string, live *store.DesiredDatabase) error {
	if instance != "" {
		if instance == live.InstanceID {
			return nil
		}
		c.recordDatabaseRefEvent(dbName, "bound to a different instance of database "+dbName)
		return errDatabaseRefRecreated
	}
	owners := c.ownerLookup()
	if owners == nil {
		return nil
	}
	appOwner, err := owners.OwnerOfApp(ctx, c.serviceName)
	if err != nil || appOwner == nil {
		return nil
	}
	dbOwner, err := owners.OwnerOfDatabase(ctx, dbName)
	if err != nil || dbOwner == nil || dbOwner.TeamID == appOwner.TeamID {
		return nil
	}
	c.recordDatabaseRefEvent(dbName, "unbound reference to a database owned by another team")
	return errDatabaseRefTeam
}

func (c *Controller) ownerLookup() resourceOwnerLookup {
	if o, ok := c.databases.(resourceOwnerLookup); ok {
		return o
	}
	if o, ok := c.store.(resourceOwnerLookup); ok {
		return o
	}
	return nil
}

func (c *Controller) recordDatabaseRefEvent(dbName, reason string) {
	slog.Warn("security: refused a database reference", slog.String("service", c.serviceName), slog.String("database", dbName), slog.String("event", "database_ref_refused"))
	adder, ok := c.store.(appEventAdder)
	if !ok {
		return
	}
	now := time.Now().UnixNano()
	last := c.databaseRefEventAt.Load()
	if last != 0 && now-last < int64(reservedOwnerEventEvery) {
		return
	}
	if !c.databaseRefEventAt.CompareAndSwap(last, now) {
		return
	}
	ev := store.AppEvent{
		AppName: c.serviceName,
		Kind:    store.AppEventDatabaseRefRefused,
		Actor:   "reconciler",
		Title:   "Database connection refused: " + reason,
		Detail:  "The database this app references was recreated or belongs to another team. Re-attach the app to the database.",
	}
	if err := adder.AddAppEvent(context.Background(), ev); err != nil {
		slog.Warn("reconcile: record database reference event failed", slog.String("service", c.serviceName), slog.String("error", err.Error()))
	}
}
