package api

import (
	"context"
	"time"

	"github.com/GLINCKER/levelrail/internal/janitor"
	"github.com/GLINCKER/levelrail/internal/storageinv"
)

const volumeLockLookupTimeout = 10 * time.Second

// volumeLock is the database volume lock every non-database delete path asks
// first. It fails closed: when ownership cannot be read the volume is kept.
func (rt *Router) volumeLock(ctx context.Context, name string) (bool, string) {
	if prot, why := storageinv.IsProtectedVolumeName(name); prot {
		return true, why
	}
	if rt.databases == nil {
		return false, ""
	}
	databases, err := rt.databases.ListDesiredDatabases(ctx)
	if err != nil {
		return true, "database ownership could not be checked, so the volume is kept"
	}
	owned := make(map[string]bool, len(databases))
	for _, db := range databases {
		owned[db.Name] = true
	}
	return storageinv.IsProtectedVolume(storageinv.Volume{Name: name, Database: storageinv.DatabaseOwner(name, owned)})
}

// installJanitorVolumeLock points the janitor's volume stage at the same lock
// the API delete paths use, since janitor cannot import api.
func (rt *Router) installJanitorVolumeLock() {
	janitor.SetVolumeProtector(func(name string) (bool, string) {
		ctx, cancel := context.WithTimeout(context.Background(), volumeLockLookupTimeout)
		defer cancel()
		return rt.volumeLock(ctx, name)
	})
}
