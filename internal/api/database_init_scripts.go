package api

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

// DatabaseInitScriptStore is the store surface the init-script handlers
// need. *store.DB satisfies this structurally.
type DatabaseInitScriptStore interface {
	SaveDatabaseInitScript(ctx context.Context, s store.DatabaseInitScript) error
	GetDatabaseInitScript(ctx context.Context, id string) (store.DatabaseInitScript, error)
	ListDatabaseInitScripts(ctx context.Context, databaseName string) ([]store.DatabaseInitScript, error)
	UpdateDatabaseInitScript(ctx context.Context, id, filename, content string, updatedAt time.Time) error
	DeleteDatabaseInitScript(ctx context.Context, id string) error
}

// databaseInitScriptResource is one init script's wire shape.
type databaseInitScriptResource struct {
	ID           string `json:"id"`
	DatabaseName string `json:"database_name"`
	Filename     string `json:"filename"`
	Content      string `json:"content"`
	CreatedAt    string `json:"created_at"`
	UpdatedAt    string `json:"updated_at"`
}

func toDatabaseInitScriptResource(s store.DatabaseInitScript) databaseInitScriptResource {
	return databaseInitScriptResource{
		ID:           s.ID,
		DatabaseName: s.DatabaseName,
		Filename:     s.Filename,
		Content:      s.Content,
		CreatedAt:    s.CreatedAt.UTC().Format(time.RFC3339Nano),
		UpdatedAt:    s.UpdatedAt.UTC().Format(time.RFC3339Nano),
	}
}

type setDatabaseInitScriptRequest struct {
	Filename string `json:"filename"`
	Content  string `json:"content"`
}

// initScriptSupportedExtensions mirrors internal/reconcile/database's
// own initScriptSupportedEngines: which file extensions each engine's
// official image actually executes from /docker-entrypoint-initdb.d.
// postgres/mysql/mariadb run *.sh/*.sql; mongo has no SQL dialect, its
// own entrypoint runs *.sh/*.js instead. Engines absent here (redis,
// keydb, dragonfly, clickhouse) never appear as a key, so any of them
// reaching validateInitScriptFilename falls through to the default
// "engine doesn't support init scripts" rejection.
var initScriptSupportedExtensions = map[string][]string{
	store.EnginePostgres: {".sql", ".sh"},
	store.EngineMySQL:    {".sql", ".sh"},
	store.EngineMariaDB:  {".sql", ".sh"},
	store.EngineMongoDB:  {".js", ".sh"},
}

// validateInitScriptFilename rejects anything that isn't a bare
// filename with an extension engine's own image actually executes.
// This is the one place path-traversal matters: filename ends up in
// filepath.Join(initScriptsDir, databaseName, filename)
// (internal/reconcile/database's own materializeInitScripts), so a
// filename carrying a path separator or ".." could otherwise write
// outside that directory entirely, using this control plane's own
// write access rather than anything Docker or the database container
// itself would do.
func validateInitScriptFilename(engine, filename string) error {
	if filename == "" {
		return errors.New("filename is required")
	}
	if filename != path.Base(filename) || strings.Contains(filename, "..") {
		return errors.New("filename must be a bare name, no path separators or \"..\"")
	}
	exts, ok := initScriptSupportedExtensions[engine]
	if !ok {
		return fmt.Errorf("engine %q does not support init scripts: only postgres, mysql, mariadb, and mongodb do", engine)
	}
	for _, ext := range exts {
		if strings.HasSuffix(filename, ext) {
			return nil
		}
	}
	return fmt.Errorf("filename must end in one of %v for engine %q", exts, engine)
}

// handleListDatabaseInitScripts handles
// GET /api/v1/databases/{name}/init-scripts. AbilityRead, the same
// passive-view tier every other database sub-resource listing uses.
func (rt *Router) handleListDatabaseInitScripts(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if _, err := rt.databases.GetDesiredDatabase(r.Context(), name); errors.Is(err, store.ErrDatabaseNotFound) {
		writeError(w, http.StatusNotFound, "database not found")
		return
	} else if err != nil {
		rt.logger.Error("api: list database init scripts: load database failed", slog.String("error", err.Error()), slog.String("name", name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	scripts, err := rt.databaseInitScripts.ListDatabaseInitScripts(r.Context(), name)
	if err != nil {
		rt.logger.Error("api: list database init scripts failed", slog.String("error", err.Error()), slog.String("name", name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	out := make([]databaseInitScriptResource, len(scripts))
	for i, s := range scripts {
		out[i] = toDatabaseInitScriptResource(s)
	}
	writeJSON(w, http.StatusOK, out)
}

// handleCreateDatabaseInitScript handles
// POST /api/v1/databases/{name}/init-scripts: runs once, the next time
// name's container is (re)created against an empty data volume, never
// retroactively against an already-initialized database (Docker's own
// documented behavior for /docker-entrypoint-initdb.d, not something
// this platform can override). AbilityWriteSensitive: this is code that
// executes automatically and unattended inside the database's own
// container, a materially different sensitivity than an ordinary
// config field like resource limits (AbilityWrite).
func (rt *Router) handleCreateDatabaseInitScript(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	db, err := rt.databases.GetDesiredDatabase(r.Context(), name)
	if errors.Is(err, store.ErrDatabaseNotFound) {
		writeError(w, http.StatusNotFound, "database not found")
		return
	}
	if err != nil {
		rt.logger.Error("api: create database init script: load database failed", slog.String("error", err.Error()), slog.String("name", name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	var req setDatabaseInitScriptRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := validateInitScriptFilename(db.Engine, req.Filename); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.Content == "" {
		writeError(w, http.StatusBadRequest, "content is required")
		return
	}

	if taken, err := rt.databaseInitScriptFilenameTaken(r.Context(), name, req.Filename, ""); err != nil {
		rt.logger.Error("api: create database init script: check existing failed", slog.String("error", err.Error()), slog.String("name", name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	} else if taken {
		writeError(w, http.StatusConflict, "a script with this filename already exists for this database")
		return
	}

	id, err := randomDatabaseInitScriptID()
	if err != nil {
		rt.logger.Error("api: create database init script: generate id failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	now := time.Now().UTC()
	script := store.DatabaseInitScript{ID: id, DatabaseName: name, Filename: req.Filename, Content: req.Content, CreatedAt: now, UpdatedAt: now}
	if err := rt.databaseInitScripts.SaveDatabaseInitScript(r.Context(), script); err != nil {
		rt.logger.Error("api: create database init script failed", slog.String("error", err.Error()), slog.String("name", name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusCreated, toDatabaseInitScriptResource(script))
}

// databaseInitScriptFilenameTaken reports whether databaseName already
// has an init script named filename, other than excludeID (itself, for
// an update that isn't actually renaming). A pre-check query rather
// than relying on the store's own unique index to reject the write,
// the same "check, then act" shape createClonedDatabaseShell
// (database_clone_now.go) already uses for an identical name-collision
// case: this accepts a narrow, low-stakes race (two concurrent creates
// of the same filename) in exchange for never needing this package to
// parse a raw SQL driver error.
func (rt *Router) databaseInitScriptFilenameTaken(ctx context.Context, databaseName, filename, excludeID string) (bool, error) {
	existing, err := rt.databaseInitScripts.ListDatabaseInitScripts(ctx, databaseName)
	if err != nil {
		return false, err
	}
	for _, s := range existing {
		if s.Filename == filename && s.ID != excludeID {
			return true, nil
		}
	}
	return false, nil
}

// handleUpdateDatabaseInitScript handles
// PUT /api/v1/databases/{name}/init-scripts/{id}. Same validation and
// sensitivity tier as create.
func (rt *Router) handleUpdateDatabaseInitScript(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	id := r.PathValue("id")

	db, err := rt.databases.GetDesiredDatabase(r.Context(), name)
	if errors.Is(err, store.ErrDatabaseNotFound) {
		writeError(w, http.StatusNotFound, "database not found")
		return
	}
	if err != nil {
		rt.logger.Error("api: update database init script: load database failed", slog.String("error", err.Error()), slog.String("name", name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	existing, err := rt.databaseInitScripts.GetDatabaseInitScript(r.Context(), id)
	if errors.Is(err, store.ErrDatabaseInitScriptNotFound) {
		writeError(w, http.StatusNotFound, "init script not found")
		return
	}
	if err != nil {
		rt.logger.Error("api: update database init script: load script failed", slog.String("error", err.Error()), slog.String("id", id))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if existing.DatabaseName != name {
		writeError(w, http.StatusNotFound, "init script not found")
		return
	}

	var req setDatabaseInitScriptRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := validateInitScriptFilename(db.Engine, req.Filename); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.Content == "" {
		writeError(w, http.StatusBadRequest, "content is required")
		return
	}

	if taken, err := rt.databaseInitScriptFilenameTaken(r.Context(), name, req.Filename, id); err != nil {
		rt.logger.Error("api: update database init script: check existing failed", slog.String("error", err.Error()), slog.String("id", id))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	} else if taken {
		writeError(w, http.StatusConflict, "a script with this filename already exists for this database")
		return
	}

	if err := rt.databaseInitScripts.UpdateDatabaseInitScript(r.Context(), id, req.Filename, req.Content, time.Now().UTC()); err != nil {
		if errors.Is(err, store.ErrDatabaseInitScriptNotFound) {
			writeError(w, http.StatusNotFound, "init script not found")
			return
		}
		rt.logger.Error("api: update database init script failed", slog.String("error", err.Error()), slog.String("id", id))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	updated, err := rt.databaseInitScripts.GetDatabaseInitScript(r.Context(), id)
	if err != nil {
		rt.logger.Error("api: update database init script: reload failed", slog.String("error", err.Error()), slog.String("id", id))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, toDatabaseInitScriptResource(updated))
}

// handleDeleteDatabaseInitScript handles
// DELETE /api/v1/databases/{name}/init-scripts/{id}.
func (rt *Router) handleDeleteDatabaseInitScript(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	id := r.PathValue("id")

	existing, err := rt.databaseInitScripts.GetDatabaseInitScript(r.Context(), id)
	if errors.Is(err, store.ErrDatabaseInitScriptNotFound) {
		writeError(w, http.StatusNotFound, "init script not found")
		return
	}
	if err != nil {
		rt.logger.Error("api: delete database init script: load script failed", slog.String("error", err.Error()), slog.String("id", id))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if existing.DatabaseName != name {
		writeError(w, http.StatusNotFound, "init script not found")
		return
	}

	if err := rt.databaseInitScripts.DeleteDatabaseInitScript(r.Context(), id); err != nil {
		rt.logger.Error("api: delete database init script failed", slog.String("error", err.Error()), slog.String("id", id))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func randomDatabaseInitScriptID() (string, error) {
	buf := make([]byte, 9)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate database init script id: %w", err)
	}
	return "dis_" + base64.RawURLEncoding.EncodeToString(buf), nil
}
