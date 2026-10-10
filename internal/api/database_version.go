package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/GLINCKER/levelrail/internal/reconcile/database"
	"github.com/GLINCKER/levelrail/internal/store"
)

var databaseVersionPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)

type setDatabaseVersionRequest struct {
	Version string `json:"version"`
}

// handleSetDatabaseVersion handles PUT /api/v1/databases/{name}/version:
// a minor or patch image change, applied by the reconciler as a stop,
// remove and recreate over the same data volume. Major upgrades are
// refused because the on-disk format is not portable; use backup plus
// restore-as-new for those.
func (rt *Router) handleSetDatabaseVersion(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	var req setDatabaseVersionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	existing, err := rt.databases.GetDesiredDatabase(r.Context(), name)
	if errors.Is(err, store.ErrDatabaseNotFound) {
		writeError(w, http.StatusNotFound, "database not found")
		return
	}
	if err != nil {
		rt.internalError(w, "api: set database version: load failed", err, slog.String("name", name))
		return
	}

	if err := checkVersionChange(existing.Engine, existing.Version, req.Version); err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}

	desired := *existing
	desired.Version = req.Version
	if err := rt.databases.SaveDesiredDatabase(r.Context(), desired); err != nil {
		rt.internalError(w, "api: set database version failed", err, slog.String("name", name))
		return
	}
	rt.reloadAndWriteDatabase(w, r, name, "set database version")
}

// checkVersionChange decides whether swapping an engine's image tag in
// place is safe. Postgres, MySQL, MariaDB, MongoDB and ClickHouse must keep
// the same major; Redis-family engines may move up but never down, since an
// older server cannot read a newer RDB file.
func checkVersionChange(engine, from, to string) error {
	if !databaseVersionPattern.MatchString(to) {
		return errors.New("version must be an image tag such as \"16.4\"")
	}
	if err := database.ValidateEngineVersion(engine, to); err != nil {
		return err
	}
	if to == from {
		return nil
	}
	_, fromVariant, _ := database.ParsePgvectorVersion(from)
	_, toVariant, _ := database.ParsePgvectorVersion(to)
	if fromVariant != toVariant {
		return errors.New("switching between the plain and pgvector images in place is refused: they are built on different Debian releases, and a glibc collation change can silently corrupt indexes. Use the guarded major upgrade (dump and restore) or restore a backup into a new database")
	}
	fromMajor, fromOK := majorVersion(from)
	toMajor, toOK := majorVersion(to)
	if !fromOK || !toOK {
		return fmt.Errorf("cannot compare %q with %q: use numeric versions, or restore a backup into a new database", from, to)
	}
	switch engine {
	case store.EngineRedis, store.EngineKeyDB, store.EngineDragonfly:
		if toMajor < fromMajor {
			return fmt.Errorf("downgrading %s from major %d to %d is not supported: the data files are not backward compatible", engine, fromMajor, toMajor)
		}
	default:
		if toMajor != fromMajor {
			return fmt.Errorf("changing %s major version %d to %d in place would corrupt the data directory: %s", engine, fromMajor, toMajor, majorChangeAdvice(engine))
		}
	}
	return nil
}

func majorChangeAdvice(engine string) string {
	if engine == store.EnginePostgres {
		return "use the guarded major upgrade (POST /api/v1/databases/{name}/major-upgrade, CLI: databases major-upgrade), or restore a backup into a new database on the new version"
	}
	return "take a backup and restore it into a new database on the new version"
}

func majorVersion(v string) (int, bool) {
	head := v
	if i := strings.IndexAny(v, ".-"); i >= 0 {
		head = v[:i]
	}
	n, err := strconv.Atoi(head)
	return n, err == nil
}
