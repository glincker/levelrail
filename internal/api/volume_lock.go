package api

import (
	"github.com/GLINCKER/levelrail/internal/janitor"
	"github.com/GLINCKER/levelrail/internal/storageinv"
)

// volumeLock is the database volume lock every non-database delete path asks
// first. The db- name rule is the only signal available from a bare name, and
// it needs no lookup, so there is nothing to fail open.
func (rt *Router) volumeLock(name string) (bool, string) {
	return storageinv.IsProtectedVolumeName(name)
}

// installJanitorVolumeLock points the janitor's volume stage at the same lock
// the API delete paths use, since janitor cannot import api.
func (rt *Router) installJanitorVolumeLock() {
	janitor.SetVolumeProtector(func(name string) (bool, string) {
		return rt.volumeLock(name)
	})
}
