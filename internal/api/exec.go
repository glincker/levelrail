package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/reconcile/application"
	"github.com/GLINCKER/levelrail/internal/store"
)

// This file implements POST /api/v1/apps/{name}/exec: a non-interactive,
// run-and-return-output exec into an app's currently running container.
// The shape is deliberately narrow: send a command (plus optional args),
// wait for it to finish, get stdout/stderr/exit code back in one
// response. This is exactly the pattern docker.Runtime.Exec's own
// ReadCloser-plus-trailing-error contract already supports end to end
// (internal/docker/runtime.go's Exec doc comment) via Local (this
// control plane's own node, in-process) today, and via a connected
// remote agent once GRPCTransport.Exec is implemented for real
// (internal/agent/grpc_transport.go: it currently always returns
// "remote Exec not implemented over the agent transport," so this
// endpoint works today for an app placed on the local node and fails
// loudly, not silently, for one placed on a remote node, exactly the
// "fail loudly, never fake it" posture that stub error's own doc
// comment already establishes for internal/backup's Dumper/Restorer).
//
// What this deliberately is not: a real interactive terminal. No PTY,
// no resize events, no WebSocket, no shell kept alive between calls.
// That needs a genuinely different transport (a bidirectional stream,
// a new agentpb op, PTY allocation inside the container), which is
// agent transport protocol work, explicitly reserved (repo-plan.md
// section 8's "do not parallelize: the agent transport protocol").
// Building that is a real follow-up, not something this endpoint
// pretends to already be.
//
// Gated at AbilityRoot (router.go's route registration), not
// AbilityDeploy: see that registration's own comment. Secrets are
// injected as plaintext env vars at container-create time and this
// package otherwise never decrypts one back into a response body
// anywhere, so exec is the one route that can read them anyway (`env`
// inside the container), and must sit behind the tier that boundary
// implies, not the deploy/restart tier.

// defaultExecTimeout bounds how long handleExecApp waits for a command
// to finish before giving up on it, so a hung command can never hold
// this handler's goroutine (or, transitively, an HTTP connection) open
// forever. A caller may ask for a shorter timeout via
// execRequest.TimeoutSeconds; they may never ask for a longer one, this
// is a hard ceiling, not a suggestion.
const defaultExecTimeout = 30 * time.Second

// execMaxOutputBytes caps how much of a command's stdout this handler
// will hold in memory and return in one response, the same "an
// unbounded buffer fed by a command this package doesn't control is
// not acceptable" reasoning internal/docker/client.go's own
// cappedBuffer (Exec's stderr side) already applies, extended here to
// stdout because this endpoint, unlike internal/backup's Dumper, never
// streams its result: the whole thing has to fit in one JSON response
// body.
const execMaxOutputBytes = 1 << 20 // 1 MiB

// execRequest is POST /apps/{name}/exec's body. Command is required and
// is never shell-interpreted: Args are passed to Docker's exec exactly
// as append([]string{Command}, Args...), the same plain argv-array
// shape docker.Runtime.Exec itself takes. A caller who wants shell
// features (pipes, redirection, globbing, env expansion) has to ask for
// them explicitly, e.g. Command: "sh", Args: ["-c", "echo $HOME"],
// rather than this endpoint guessing whether Command should be
// shell-parsed.
type execRequest struct {
	Command string   `json:"command"`
	Args    []string `json:"args,omitempty"`
	// TimeoutSeconds, if positive and shorter than defaultExecTimeout,
	// overrides it. Zero, negative, or longer than the default all fall
	// back to defaultExecTimeout: see that constant's own doc comment
	// for why this is a ceiling a caller can only lower, never raise.
	TimeoutSeconds int `json:"timeout_seconds,omitempty"`
}

// execResponse is handleExecApp's 200 body: everything the command
// produced, once it finished. ExitCode is always the command's real
// exit code (0 on success), never a sentinel value: a nonzero ExitCode
// here means the exec mechanism itself worked fine and the command it
// ran chose to fail, which is a normal, expected outcome this endpoint
// reports as 200, not as an HTTP-level error (see handleExecApp's own
// branch on docker.ExecExitError below). Stderr is only ever populated
// when ExitCode is nonzero: see docker.Runtime.Exec's own doc comment
// for why a well-behaved, exit-0 command's stderr output (e.g.
// informational logging many CLIs send there) is not observable
// through the current docker.Runtime.Exec contract at all, a real,
// named, pre-existing gap this endpoint inherits rather than works
// around.
type execResponse struct {
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr,omitempty"`
	ExitCode int    `json:"exit_code"`
	// Truncated is true when the command's real stdout exceeded
	// execMaxOutputBytes: Stdout is the first execMaxOutputBytes of it,
	// not the whole thing.
	Truncated bool `json:"truncated,omitempty"`
}

// cappedWriter accumulates up to limit bytes of whatever is written to
// it and silently discards the rest, while still reporting every write
// as fully successful, the same contract internal/docker/client.go's
// own cappedBuffer establishes for Exec's stderr side. Write is used
// here (via io.Copy) instead of io.LimitReader specifically so this
// handler keeps draining a command's stdout to completion even past
// the cap: stopping the read early would mean never reaching
// docker.Runtime.Exec's trailing error (the exit code), so a command
// whose stdout happens to exceed the cap would silently look like it
// exited 0 even when it didn't. total tracks real bytes seen
// (independent of buf's own capped length) so truncated() is exact.
type cappedWriter struct {
	buf   bytes.Buffer
	limit int
	total int64
}

func (c *cappedWriter) Write(p []byte) (int, error) {
	n := len(p)
	c.total += int64(n)
	if remaining := c.limit - c.buf.Len(); remaining > 0 {
		if n > remaining {
			p = p[:remaining]
		}
		c.buf.Write(p)
	}
	return n, nil
}

func (c *cappedWriter) truncated() bool {
	return c.total > int64(c.limit)
}

// handleExecApp handles POST /api/v1/apps/{name}/exec. 501 if no
// NodeRuntimeResolver is configured (WithExecRuntime), the same
// "not configured" shape every other optional-dependency route in this
// package already uses.
func (rt *Router) handleExecApp(w http.ResponseWriter, r *http.Request) {
	if rt.execRuntime == nil {
		writeError(w, http.StatusNotImplemented, "exec is not configured on this control plane")
		return
	}

	name := r.PathValue("name")

	svc, err := rt.apps.GetDesiredService(r.Context(), name)
	if errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return
	}
	if err != nil {
		rt.logger.Error("api: exec app: load app failed", slog.String("error", err.Error()), slog.String("name", name))
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

	rt2, err := rt.execRuntime(svc.NodeID)
	if err != nil {
		rt.logger.Error("api: exec app: resolve node runtime failed",
			slog.String("error", err.Error()), slog.String("name", name), slog.String("node_id", svc.NodeID))
		writeError(w, http.StatusBadGateway, "app's node is not currently reachable")
		return
	}

	// Same target-container derivation internal/reconcile/ingress and
	// internal/api/system_prune.go already use to find a service's
	// currently active container from its own desired state, without
	// this package needing to track container IDs anywhere itself: see
	// application.ContainerName's own doc comment.
	target := application.ContainerName(svc.Name, svc.Image, svc.RestartNonce)
	state, err := rt2.InspectByName(r.Context(), target)
	if err != nil {
		rt.logger.Error("api: exec app: inspect container failed",
			slog.String("error", err.Error()), slog.String("name", name), slog.String("container", target))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if state == nil || !state.Running {
		writeError(w, http.StatusConflict, "app has no running container")
		return
	}

	cmd := append([]string{req.Command}, req.Args...)

	execCtx, cancel := context.WithTimeout(r.Context(), timeout)
	defer cancel()

	rc, err := rt2.Exec(execCtx, state.ID, cmd)
	if err != nil {
		rt.logger.Error("api: exec app: start exec failed",
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
		// Best-effort cleanup, not a guarantee: closing rc unblocks a
		// command that is still actively producing output (the next
		// write on docker.Client's own side of the pipe fails once this
		// side is closed, ending its streamExecOutput goroutine
		// promptly). It does not guarantee a genuinely hung command
		// (blocked on its own container-side read, producing nothing at
		// all) stops immediately: docker.Client.Exec does not wire ctx
		// cancellation into that underlying blocking read today
		// (internal/docker/client.go's streamExecOutput only uses ctx
		// for the initial ContainerExecCreate/ContainerExecAttach
		// calls), a real, pre-existing gap in that package this
		// endpoint inherits rather than silently working around. What
		// this guarantees is narrower and still the thing that matters
		// here: this HTTP handler's own goroutine never blocks past
		// timeout, so the client always gets a timely response either
		// way.
		_ = rc.Close()
		rt.logger.Warn("api: exec app: command exceeded timeout",
			slog.String("name", name), slog.String("container", state.ID), slog.Duration("timeout", timeout))
		writeError(w, http.StatusGatewayTimeout, fmt.Sprintf("command timed out after %s", timeout))
	}
}

// writeExecOutcome turns the result of draining Exec's stream into
// handleExecApp's HTTP response: a nil err or a *docker.ExecExitError
// both mean the exec mechanism itself worked (see execResponse's own
// doc comment on why a nonzero exit code is still a 200), anything else
// is a genuine exec-transport failure reported as a 500.
func (rt *Router) writeExecOutcome(w http.ResponseWriter, out *cappedWriter, err error) {
	stdout := out.buf.String()
	truncated := out.truncated()

	if err == nil {
		writeJSON(w, http.StatusOK, execResponse{Stdout: stdout, ExitCode: 0, Truncated: truncated})
		return
	}

	var execErr *docker.ExecExitError
	if errors.As(err, &execErr) {
		writeJSON(w, http.StatusOK, execResponse{
			Stdout:    stdout,
			Stderr:    execErr.Stderr,
			ExitCode:  execErr.ExitCode,
			Truncated: truncated,
		})
		return
	}

	rt.logger.Error("api: exec app: reading command output failed", slog.String("error", err.Error()))
	writeError(w, http.StatusInternalServerError, "internal error")
}
