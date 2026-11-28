package application

import (
	"context"
	"errors"
	"log/slog"
	"time"

	appspec "github.com/GLINCKER/levelrail/internal/spec"
	"github.com/GLINCKER/levelrail/internal/store"
)

// errReservedSecretOwner is returned when an app named like a control plane secret namespace asks for secrets.
var errReservedSecretOwner = errors.New("app name is a reserved control plane secret namespace; rename the app")

// refuseReservedSecretOwner blocks secret resolution for a pre-existing app whose name aliases a control plane namespace.
func (c *Controller) refuseReservedSecretOwner() error {
	if !appspec.IsReservedName(c.serviceName) {
		return nil
	}
	c.recordReservedOwnerEvent()
	slog.Warn("security: refused to resolve secrets for an app named like a control plane namespace", slog.String("service", c.serviceName), slog.String("event", "reserved_secret_owner"))
	return errReservedSecretOwner
}

type databaseNameLookup interface {
	GetDesiredDatabase(ctx context.Context, name string) (*store.DesiredDatabase, error)
}

// errDatabaseSecretOwner is returned when an app shares its name with a database, whose credentials live under that name.
var errDatabaseSecretOwner = errors.New("app name is also a database name, so its secrets would expose database credentials; rename the app")

// refuseDatabaseSecretOwner blocks secret resolution for an app named like a database.
func (c *Controller) refuseDatabaseSecretOwner(ctx context.Context) error {
	lookup, ok := c.store.(databaseNameLookup)
	if !ok {
		return nil
	}
	if _, err := lookup.GetDesiredDatabase(ctx, c.serviceName); err != nil {
		return nil
	}
	c.recordReservedOwnerEvent()
	slog.Warn("security: refused to resolve secrets for an app named like a database", slog.String("service", c.serviceName), slog.String("event", "database_secret_owner"))
	return errDatabaseSecretOwner
}

// reservedOwnerEventEvery bounds the stored event to one per app per window, so a retry loop cannot flood the timeline.
const reservedOwnerEventEvery = time.Hour

type appEventAdder interface {
	AddAppEvent(ctx context.Context, e store.AppEvent) error
}

func (c *Controller) recordReservedOwnerEvent() {
	adder, ok := c.store.(appEventAdder)
	if !ok {
		return
	}
	now := time.Now().UnixNano()
	last := c.reservedOwnerEventAt.Load()
	if last != 0 && now-last < int64(reservedOwnerEventEvery) {
		return
	}
	if !c.reservedOwnerEventAt.CompareAndSwap(last, now) {
		return
	}
	ev := store.AppEvent{
		AppName: c.serviceName,
		Kind:    store.AppEventReservedSecretOwner,
		Actor:   "reconciler",
		Title:   "Secrets refused: app name is a reserved control plane namespace",
		Detail:  "Rename the app so its secrets cannot alias a control plane secret namespace.",
	}
	if err := adder.AddAppEvent(context.Background(), ev); err != nil {
		slog.Warn("reconcile: record reserved secret owner event failed", slog.String("service", c.serviceName), slog.String("error", err.Error()))
	}
}
