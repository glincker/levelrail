package api

import (
	"context"
	"errors"
	"fmt"
	"github.com/GLINCKER/levelrail/internal/spool"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/internal/backup"
	"github.com/GLINCKER/levelrail/internal/datamigrate"
	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/runtimesettings"
	"github.com/GLINCKER/levelrail/internal/store"
)

// AuditActionDatabaseImport marks a dump restored into a managed database.
const AuditActionDatabaseImport = "database.import"

// maxImportProbeOutput bounds what the emptiness probe may print back.
const maxImportProbeOutput = 1 << 20

// databaseImports lets one import per database run at a time.
var databaseImports = struct {
	sync.Mutex
	active map[string]struct{}
}{active: map[string]struct{}{}}

func beginDatabaseImport(name string) (func(), bool) {
	databaseImports.Lock()
	defer databaseImports.Unlock()
	if _, busy := databaseImports.active[name]; busy {
		return nil, false
	}
	databaseImports.active[name] = struct{}{}
	return func() {
		databaseImports.Lock()
		delete(databaseImports.active, name)
		databaseImports.Unlock()
	}, true
}

type databaseImportCheckResource struct {
	Database string `json:"database"`
	Engine   string `json:"engine"`
	Empty    bool   `json:"empty"`
}

type databaseImportResource struct {
	Database string `json:"database"`
	Engine   string `json:"engine"`
	Bytes    int64  `json:"bytes"`
	Replaced bool   `json:"replaced"`
}

// isDatabaseImport is the one route whose dump the handler bounds itself with databases.import_max_bytes, after auth.
func isDatabaseImport(r *http.Request) bool {
	rest, ok := strings.CutPrefix(r.URL.Path, "/api/v1/databases/")
	if !ok || r.Method != http.MethodPost {
		return false
	}
	name, tail, ok := strings.Cut(rest, "/")
	return ok && name != "" && tail == "import"
}

// databaseHoldsData reads the engine's own table or key counts. Anything it
// cannot parse counts as data, so an odd answer never opens the overwrite path.
func databaseHoldsData(engine, probeOutput string) bool {
	for _, line := range strings.Split(probeOutput, "\n") {
		label, count, ok := strings.Cut(strings.TrimSpace(line), "|")
		if !ok || label == "" {
			continue
		}
		if engine != store.EngineRedis {
			return true
		}
		if n, err := strconv.Atoi(strings.TrimSpace(count)); err != nil || n != 0 {
			return true
		}
	}
	return false
}

// importTarget loads the database and its node runtime; it answers the error itself.
func (rt *Router) importTarget(w http.ResponseWriter, r *http.Request) (*store.DesiredDatabase, docker.Runtime, bool) {
	name := r.PathValue("name")
	db, ok := rt.loadDatabaseForRunner(w, r, name, "api: database import: load database failed")
	if !ok {
		return nil, nil, false
	}
	if !datamigrate.Supported(db.Engine) {
		writeError(w, http.StatusBadRequest, "importing a dump is available for postgres, mysql, mariadb, mongodb and redis databases")
		return nil, nil, false
	}
	if rt.execRuntime == nil {
		writeError(w, http.StatusNotImplemented, "importing data is not available on this control plane")
		return nil, nil, false
	}
	runtime, err := rt.execRuntime(db.NodeID)
	if err != nil {
		rt.logger.Error("api: database import: resolve node runtime failed", slog.String("error", err.Error()), slog.String("database", name), slog.String("node_id", db.NodeID))
		writeError(w, http.StatusBadGateway, "the database's node is not currently reachable")
		return nil, nil, false
	}
	return db, runtime, true
}

// handleDatabaseImportCheck handles GET /api/v1/databases/{name}/import: whether the target holds data yet.
func (rt *Router) handleDatabaseImportCheck(w http.ResponseWriter, r *http.Request) {
	db, runtime, ok := rt.importTarget(w, r)
	if !ok {
		return
	}
	holds, err := databaseHoldsDataNow(r.Context(), db, runtime)
	if err != nil {
		rt.logger.Warn("api: database import: emptiness probe failed", slog.String("database", db.Name), slog.String("error", err.Error()))
		writeError(w, http.StatusBadGateway, "could not read the database to check whether it is empty; is it running?")
		return
	}
	writeJSON(w, http.StatusOK, databaseImportCheckResource{Database: db.Name, Engine: db.Engine, Empty: !holds})
}

func databaseHoldsDataNow(ctx context.Context, db *store.DesiredDatabase, runtime docker.Runtime) (bool, error) {
	cmd, err := datamigrate.TargetEmptinessCommand(db.Engine)
	if err != nil {
		return false, err
	}
	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	rc, err := runtime.Exec(ctx, databaseContainerName(db.Name), cmd)
	if err != nil {
		return false, err
	}
	defer func() { _ = rc.Close() }()
	out, err := io.ReadAll(io.LimitReader(rc, maxImportProbeOutput))
	if err != nil {
		return false, err
	}
	return databaseHoldsData(db.Engine, string(out)), nil
}

// handleDatabaseImport handles POST /api/v1/databases/{name}/import with the
// dump as the body. AbilityRoot like an in-place restore: it can replace live
// data. A target that holds data needs force=true and confirm=<name>.
func (rt *Router) handleDatabaseImport(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	db, runtime, ok := rt.importTarget(w, r)
	if !ok {
		return
	}
	status := http.StatusOK
	var written int64
	replaced := false
	defer func() { rt.auditDatabaseImport(r, name, written, replaced, status) }()
	fail := func(code int, msg string) {
		status = code
		writeError(w, code, msg)
	}

	force := r.URL.Query().Get("force") == "true"
	if force && r.URL.Query().Get("confirm") != name {
		fail(http.StatusBadRequest, "overwriting needs confirm=<database name>")
		return
	}
	maxBytes := int64(rt.runtimeSettings.Limit(r.Context(), runtimesettings.KeyDatabaseImportMaxBytes))
	if r.ContentLength > maxBytes {
		fail(http.StatusRequestEntityTooLarge, fmt.Sprintf("dump exceeds the %d MB import limit", maxBytes>>20))
		return
	}
	release, free := beginDatabaseImport(name)
	if !free {
		fail(http.StatusConflict, "another import into this database is already running")
		return
	}
	defer release()

	holds, err := databaseHoldsDataNow(r.Context(), db, runtime)
	if err != nil {
		rt.logger.Warn("api: database import: emptiness probe failed", slog.String("database", name), slog.String("error", err.Error()))
		fail(http.StatusBadGateway, "could not read the database to check whether it is empty; is it running?")
		return
	}
	if holds && !force {
		fail(http.StatusConflict, "the database already holds data: pass force with confirm to replace it")
		return
	}
	replaced = holds

	ctx, cancel := context.WithTimeout(r.Context(), copyTimeout())
	defer cancel()
	// Spool first: a cut-off or oversized upload must fail before the engine drops anything.
	spool, n, err := spoolImportBody(http.MaxBytesReader(w, r.Body, maxBytes))
	written = n
	if err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			fail(http.StatusRequestEntityTooLarge, fmt.Sprintf("dump exceeds the %d MB import limit", maxBytes>>20))
			return
		}
		rt.logger.Warn("api: database import: upload not received", slog.String("database", name), slog.String("error", err.Error()))
		fail(http.StatusBadRequest, "the upload did not arrive completely; nothing was restored")
		return
	}
	defer spool.discard()
	if n == 0 {
		fail(http.StatusBadRequest, "the dump is empty; nothing was restored")
		return
	}
	if isAge, aerr := isAgeExport(spool.f); aerr == nil && isAge {
		plain, pn, msg := rt.unsealImport(spool, maxBytes)
		if msg != "" {
			fail(http.StatusBadRequest, msg)
			return
		}
		spool, written = plain, pn
		defer spool.discard()
		if pn == 0 {
			fail(http.StatusBadRequest, "the dump is empty; nothing was restored")
			return
		}
	}
	if msg, herr := sealedImportReason(spool.f, db.Engine); herr != nil || msg != "" {
		if herr != nil {
			msg = "the upload could not be read back; nothing was restored"
		}
		fail(http.StatusBadRequest, msg)
		return
	}
	// Writers outside this import (the app itself) may have added data while the upload spooled.
	if !holds && !force {
		again, perr := databaseHoldsDataNow(r.Context(), db, runtime)
		if perr != nil {
			rt.logger.Warn("api: database import: second emptiness probe failed", slog.String("database", name), slog.String("error", perr.Error()))
			fail(http.StatusBadGateway, "could not read the database to check whether it is still empty; nothing was restored")
			return
		}
		if again {
			fail(http.StatusConflict, "the database received data during the upload: pass force with confirm to replace it")
			return
		}
	}
	err = (&backup.ContainerRestorer{Runtime: runtime}).Restore(ctx, db.Engine, databaseContainerName(name), spool.f)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			fail(http.StatusGatewayTimeout, "the import ran past the time limit")
			return
		}
		class, exit := importErrorClass(err)
		rt.logger.Error("api: database import failed", slog.String("engine", db.Engine), slog.String("database", name), slog.Int("exit_status", exit), slog.String("error_class", class))
		fail(http.StatusBadGateway, "the restore failed; check that the dump matches this database engine")
		return
	}
	writeJSON(w, http.StatusOK, databaseImportResource{Database: name, Engine: db.Engine, Bytes: written, Replaced: replaced})
}

// importSpool is the fully received dump, held in a private temp file.
type importSpool struct{ f *os.File }

func (s importSpool) discard() {
	name := s.f.Name()
	_ = s.f.Close()
	_ = os.Remove(name)
}

// spoolImportBody copies src to an unlinked-on-discard 0600 temp file and rewinds it.
func spoolImportBody(src io.Reader) (importSpool, int64, error) {
	f, err := spool.Create("dbimport")
	if err != nil {
		return importSpool{}, 0, err
	}
	sp := importSpool{f: f}
	n, err := io.Copy(f, src)
	if err == nil {
		_, err = f.Seek(0, io.SeekStart)
	}
	if err != nil {
		sp.discard()
		return importSpool{}, n, err
	}
	return sp, n, nil
}

// auditDatabaseImport records the name, byte count and outcome, never the dump or any connection string.
func (rt *Router) auditDatabaseImport(r *http.Request, name string, n int64, replaced bool, status int) {
	if rt.auditLog == nil {
		return
	}
	a := rt.accessActor(r)
	id, err := store.NewAuditEntryID()
	if err != nil {
		rt.logger.Warn("api: database import audit id failed", slog.String("error", err.Error()))
		return
	}
	entry := store.AuditEntry{
		ID: id, ActorType: a.kind, ActorID: a.id, ActorName: a.name,
		Ability: AbilityRoot, Method: r.Method, Path: "/api/v1/databases/" + name + "/import", StatusCode: status,
		RemoteAddr: clientIP(r), CreatedAt: store.FormatAuditTime(time.Now()),
		ClientKind: clientKindFromUserAgent(r.Header.Get("User-Agent")),
		Action:     AuditActionDatabaseImport, Detail: fmt.Sprintf(`{"database":%q,"bytes":%d,"replaced":%t}`, name, n, replaced),
	}
	if err := rt.auditLog.SaveAuditEntry(context.WithoutCancel(r.Context()), entry); err != nil {
		rt.logger.Warn("api: database import audit save failed", slog.String("error", err.Error()), slog.String("database", name))
	}
}
