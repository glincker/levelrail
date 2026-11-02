package backup

import (
	"errors"
	"sync"
)

// ErrImportInProgress means a volume import is running for the app.
var ErrImportInProgress = errors.New("backup: a volume import is running for this app, retry when it finishes")

var (
	importMu   sync.Mutex
	importApps = map[string]int{}
)

// MarkImport records an import under way for app; the returned func clears it.
func MarkImport(app string) func() {
	importMu.Lock()
	importApps[app]++
	importMu.Unlock()
	return func() {
		importMu.Lock()
		defer importMu.Unlock()
		if importApps[app]--; importApps[app] <= 0 {
			delete(importApps, app)
		}
	}
}

// CheckNoImport returns ErrImportInProgress while an import runs for app. Every
// path that starts or un-suspends an app calls it, so no container mounts a
// volume mid-import.
func CheckNoImport(app string) error {
	importMu.Lock()
	defer importMu.Unlock()
	if importApps[app] > 0 {
		return ErrImportInProgress
	}
	return nil
}
