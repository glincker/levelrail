package ingress

import (
	"fmt"
	"sync"

	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/certmagic"
)

func init() {
	caddy.RegisterModule(sqliteCertStorageModule{})
}

// activeStorage is the process-wide SQLiteStorage instance the "sqlite"
// Caddy storage module (sqliteCertStorageModule) resolves against.
//
// Caddy's module system instantiates config-driven modules purely from
// JSON (see caddy.Module's New func in ModuleInfo), with no way to pass
// a live Go value like a *store.DB connection through the config
// document itself. Custom storage modules for other backends (Redis,
// MySQL, ...) usually resolve this by putting connection details (a
// DSN, host/port) directly in their own JSON fields and opening a fresh
// connection in Provision. That would work here too, but it would mean
// opening a second, independent *sql.DB against the same SQLite file
// this control plane's own internal/store.DB already has open and
// migrated, on every Caddy config reload (every
// ingress reconcile, per this package's own driver.go). Pointing the
// module at whichever SQLiteStorage was last registered via
// SetActiveCertStorage instead reuses the one connection the rest of
// this process already maintains.
var (
	activeStorageMu sync.RWMutex
	activeStorage   *SQLiteStorage
)

// SetActiveCertStorage registers storage as the backend the "sqlite"
// Caddy storage module resolves against (see NewSQLiteStorageRef).
// internal/reconcile/ingress.Controller's WithCertStore option calls
// this once per Controller, not once per reconcile: the whole point is
// that Caddy's repeated config reloads share the one already-open
// connection rather than each provisioning their own.
func SetActiveCertStorage(storage *SQLiteStorage) {
	activeStorageMu.Lock()
	defer activeStorageMu.Unlock()
	activeStorage = storage
}

func getActiveCertStorage() *SQLiteStorage {
	activeStorageMu.RLock()
	defer activeStorageMu.RUnlock()
	return activeStorage
}

// sqliteCertStorageModule is the Caddy module wrapper for SQLiteStorage.
// Its JSON shape is just {"module": "sqlite"}: it carries no
// configuration of its own, because CertMagicStorage resolves to
// whatever SetActiveCertStorage last registered rather than building a
// new backend from config fields. See activeStorage's doc comment for
// why.
type sqliteCertStorageModule struct{}

// CaddyModule implements caddy.Module.
func (sqliteCertStorageModule) CaddyModule() caddy.ModuleInfo {
	return caddy.ModuleInfo{
		ID:  "caddy.storage.sqlite",
		New: func() caddy.Module { return new(sqliteCertStorageModule) },
	}
}

// CertMagicStorage implements caddy.StorageConverter.
func (sqliteCertStorageModule) CertMagicStorage() (certmagic.Storage, error) {
	storage := getActiveCertStorage()
	if storage == nil {
		return nil, fmt.Errorf("ingress: sqlite storage module: no active *SQLiteStorage registered, call SetActiveCertStorage before applying a Config that references it")
	}
	return storage, nil
}

// SQLiteStorageRef is the JSON shape assigned to Config.Storage (or
// RoutesOptions.CertStorage) to select the "sqlite" storage module. See
// NewSQLiteStorageRef.
type SQLiteStorageRef struct {
	Module string `json:"module"`
}

// NewSQLiteStorageRef returns the JSON shape for the sqlite storage
// module. Pair with SetActiveCertStorage (directly, or via
// internal/reconcile/ingress.WithCertStore) so CertMagicStorage has a
// real backend to resolve to; a Config referencing this without an
// active backend registered fails to apply with a clear error rather
// than silently falling back to Caddy's file storage default.
func NewSQLiteStorageRef() SQLiteStorageRef {
	return SQLiteStorageRef{Module: "sqlite"}
}

var (
	_ caddy.Module           = (*sqliteCertStorageModule)(nil)
	_ caddy.StorageConverter = (*sqliteCertStorageModule)(nil)
)
