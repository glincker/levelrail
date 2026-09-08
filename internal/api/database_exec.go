package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

// handleExecDatabase handles POST /api/v1/databases/{name}/exec: the
// database counterpart to handleExecApp (exec.go), same non-
// interactive run-and-return-output shape, same execRequest/
// execResponse/cappedWriter/writeExecOutcome plumbing reused directly
// rather than duplicated. The one real difference is container
// resolution: a database has a single, stably-named container
// (databaseContainerName, "db-" + name, see internal/reconcile/
// database's own containerName doc comment for why that name never
// changes), not an image/restart-nonce-derived one, so there is no
// application.ContainerName-equivalent call here.
//
// Gated at AbilityRoot (routes_platform.go's route registration), the
// same tier handleExecApp uses and for the identical reason: exec can
// read whatever the container's own env holds, which for a database
// includes its root credentials (internal/reconcile/database sets
// POSTGRES_PASSWORD/MYSQL_ROOT_PASSWORD/etc. as plain container env),
// so this is strictly more sensitive than an app's own exec, never
// less.
func (rt *Router) handleExecDatabase(w http.ResponseWriter, r *http.Request) {
	if rt.execRuntime == nil {
		writeError(w, http.StatusNotImplemented, "exec is not configured on this control plane")
		return
	}

	name := r.PathValue("name")

	desired, err := rt.databases.GetDesiredDatabase(r.Context(), name)
	if errors.Is(err, store.ErrDatabaseNotFound) {
		writeError(w, http.StatusNotFound, "database not found")
		return
	}
	if err != nil {
		rt.logger.Error("api: exec database: load database failed", slog.String("error", err.Error()), slog.String("name", name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	var req execRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if strings.TrimSpace(req.Command) == "" {
		writeError(w, http.StatusBadRequest, "command is required")
		return
	}

	timeout := defaultExecTimeout
	if req.TimeoutSeconds > 0 {
		if requested := time.Duration(req.TimeoutSeconds) * time.Second; requested < timeout {
			timeout = requested
		}
	}

	runtime, err := rt.execRuntime(desired.NodeID)
	if err != nil {
		rt.logger.Error("api: exec database: resolve node runtime failed",
			slog.String("error", err.Error()), slog.String("name", name), slog.String("node_id", desired.NodeID))
		writeError(w, http.StatusBadGateway, "database's node is not currently reachable")
		return
	}

	target := databaseContainerName(name)
	state, err := runtime.InspectByName(r.Context(), target)
	if err != nil {
		rt.logger.Error("api: exec database: inspect container failed",
			slog.String("error", err.Error()), slog.String("name", name), slog.String("container", target))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if state == nil || !state.Running {
		writeError(w, http.StatusConflict, "database has no running container")
		return
	}

	cmd := append([]string{req.Command}, req.Args...)

	execCtx, cancel := context.WithTimeout(r.Context(), timeout)
	defer cancel()

	rc, err := runtime.Exec(execCtx, state.ID, cmd)
	if err != nil {
		rt.logger.Error("api: exec database: start exec failed",
			slog.String("error", err.Error()), slog.String("name", name), slog.String("container", state.ID))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	type execOutcome struct {
		out *cappedWriter
		err error
	}
	outcome := make(chan execOutcome, 1)
	go func() {
		capped := &cappedWriter{limit: execMaxOutputBytes}
		_, readErr := io.Copy(capped, rc)
		outcome <- execOutcome{out: capped, err: readErr}
	}()

	select {
	case res := <-outcome:
		_ = rc.Close()
		rt.writeExecOutcome(w, res.out, res.err)
	case <-execCtx.Done():
		// Same best-effort-cleanup posture handleExecApp's own doc
		// comment on its identical select explains: closing rc unblocks
		// a command still producing output, but does not guarantee a
		// genuinely hung one stops. What is guaranteed is that this
		// handler's own goroutine never blocks past timeout.
		_ = rc.Close()
		rt.logger.Warn("api: exec database: command exceeded timeout",
			slog.String("name", name), slog.String("container", state.ID), slog.Duration("timeout", timeout))
		writeError(w, http.StatusGatewayTimeout, fmt.Sprintf("command timed out after %s", timeout))
	}
}
