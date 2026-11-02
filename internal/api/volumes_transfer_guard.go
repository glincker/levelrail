package api

import (
	"errors"
	"sync"

	"github.com/GLINCKER/levelrail/internal/backup"
)

var (
	errImportVolumeBusy    = errors.New("api: another import is already running on this volume")
	errImportCapacity      = errors.New("api: too many volume imports are running")
	errImportAppNotStopped = errors.New("api: the app is not stopped")
)

// volumeTransferGuard serializes imports per volume, counts running imports
// per control plane, and tells the start paths an app has an import under way.
// The zero value is ready to use.
type volumeTransferGuard struct {
	mu      sync.Mutex
	volumes map[string]bool
	apps    map[string]int
	active  int
	swept   map[string]bool
}

// beginImport reserves the volume and one import slot; the returned func frees both.
// spoolDir is swept once per process while no import is staging into it.
func (g *volumeTransferGuard) beginImport(app, volume string, maxConcurrent int, spoolDir string) (func(), error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.volumes[volume] {
		return nil, errImportVolumeBusy
	}
	if g.active >= maxConcurrent {
		return nil, errImportCapacity
	}
	if g.volumes == nil {
		g.volumes, g.apps, g.swept = map[string]bool{}, map[string]int{}, map[string]bool{}
	}
	if g.active == 0 && spoolDir != "" && !g.swept[spoolDir] {
		g.swept[spoolDir] = true
		backup.CleanSpoolDir(spoolDir)
	}
	g.volumes[volume] = true
	g.apps[app]++
	g.active++
	return func() {
		g.mu.Lock()
		defer g.mu.Unlock()
		delete(g.volumes, volume)
		if g.apps[app]--; g.apps[app] <= 0 {
			delete(g.apps, app)
		}
		g.active--
	}, nil
}

// importing reports whether an import into any of the app's volumes is running.
func (g *volumeTransferGuard) importing(app string) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.apps[app] > 0
}
