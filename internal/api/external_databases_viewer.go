package api

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/GLINCKER/levelrail/internal/dbviewer"
	"github.com/GLINCKER/levelrail/internal/extdb"
	"github.com/GLINCKER/levelrail/internal/store"
)

// externalViewerTarget resolves an external database into a console target.
// found is false when name is not an external database. When found, ok says
// whether a target was produced; otherwise the response is already written.
// Writes still go through the guarded write route and its confirmation.
func (rt *Router) externalViewerTarget(w http.ResponseWriter, r *http.Request, name string, wantKV bool) (target dbviewer.Target, desired *store.DesiredDatabase, found, ok bool) {
	rec, err := rt.externalDatabases.GetExternalDatabase(r.Context(), name)
	if errors.Is(err, store.ErrExternalDatabaseNotFound) {
		return dbviewer.Target{}, nil, false, false
	}
	if err != nil {
		rt.internalError(w, "api: database viewer: load external database failed", err, slog.String("name", name))
		return dbviewer.Target{}, nil, true, false
	}
	desired = &store.DesiredDatabase{Name: rec.Name, Engine: rec.Engine, NodeID: rec.NodeID}

	var dialect dbviewer.Dialect
	switch rec.Engine {
	case store.EnginePostgres:
		dialect = dbviewer.DialectPostgres
	case store.EngineMySQL:
		dialect = dbviewer.DialectMySQL
	case store.EngineMariaDB:
		dialect = dbviewer.DialectMariaDB
	default:
		writeError(w, http.StatusBadRequest, "the viewer for external databases supports postgres, mysql and mariadb")
		return dbviewer.Target{}, desired, true, false
	}
	if wantKV {
		writeError(w, http.StatusBadRequest, "key browser is not supported for engine "+rec.Engine)
		return dbviewer.Target{}, desired, true, false
	}
	runtime, rok := rt.externalNodeRuntime(w, rec.NodeID)
	if !rok {
		return dbviewer.Target{}, desired, true, false
	}
	var pw extdb.PasswordSource
	if rt.secrets != nil {
		pw = rt.secrets
	}
	conn, err := extdb.ConnFromRecord(r.Context(), *rec, pw)
	if err != nil {
		rt.internalError(w, "api: database viewer: load external connection failed", err, slog.String("name", name))
		return dbviewer.Target{}, desired, true, false
	}
	return dbviewer.Target{
		Exec:     &extdb.Execer{Helper: &extdb.Helper{Runtime: runtime, Logger: rt.logger}, Name: name, Conn: conn},
		Dialect:  dialect,
		Limits:   dbviewer.LimitsFromEnv(nil),
		External: true,
	}, desired, true, true
}
