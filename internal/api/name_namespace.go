package api

import (
	"context"
	"errors"

	"github.com/GLINCKER/levelrail/internal/store"
)

// msgNameUnavailable never says which kind of resource holds the name, so it is no cross-tenant existence oracle.
const msgNameUnavailable = "name is not available"

// nameTakenByOtherKind reports whether name is held by an app, database or compose stack other
// than own, plus a parked deleted resource of another kind when includeParked is set.
func (rt *Router) nameTakenByOtherKind(ctx context.Context, name string, own store.NameKind, includeParked bool) (bool, error) {
	if own != store.NameKindApp && rt.apps != nil {
		if _, err := rt.apps.GetDesiredService(ctx, name); err == nil {
			return true, nil
		} else if !errors.Is(err, store.ErrServiceNotFound) {
			return false, err
		}
	}
	if own != store.NameKindDatabase && rt.databases != nil {
		if _, err := rt.databases.GetDesiredDatabase(ctx, name); err == nil {
			return true, nil
		} else if !errors.Is(err, store.ErrDatabaseNotFound) {
			return false, err
		}
	}
	if own != store.NameKindStack {
		if _, err := rt.composeMembers(ctx, name); err == nil {
			return true, nil
		} else if !errors.Is(err, store.ErrServiceNotFound) {
			return false, err
		}
	}
	if !includeParked || rt.recentlyDeleted() == nil {
		return false, nil
	}
	parkedKind := store.RecentlyDeletedApp
	if own == store.NameKindApp {
		parkedKind = store.RecentlyDeletedDatabase
	}
	if _, err := rt.liveDeleted(ctx, parkedKind, name); err == nil {
		return true, nil
	} else if !errors.Is(err, store.ErrRecentlyDeletedNotFound) {
		// A store error keeps the name reserved, like deletedNameReserved.
		return true, nil
	}
	return false, nil
}
