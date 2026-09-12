package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"

	"github.com/GLINCKER/levelrail/internal/docker"
)

// GET /api/v1/apps/{name}/terminal: an interactive shell on an app's
// running container.
//
// A WebSocket is a narrow, deliberate exception to this project's
// SSE-by-default rule: SSE has no upward channel, and a terminal needs
// keystrokes and resizes going up on the same stream its output comes
// down. Log tailing and deploy progress stay on SSE.
//
// Frames: binary is raw terminal bytes in both directions, text is JSON
// control (client sends resize, server sends the exit status).
// AbilityRoot-gated for the same reason one-shot exec is (exec.go): a
// shell can read the container's injected secrets.

// terminalIdleTimeout ends a session that has carried no traffic in
// either direction for this long. An abandoned tab that never sent a
// close frame (a laptop suspended, a network dropped) would otherwise
// hold a shell open on the node indefinitely, which is exactly the leak
// this endpoint's cleanup discipline exists to prevent. Idle, not a
// hard session cap: a terminal watching a chatty log for an hour is
// being used, and cutting it off would be a bug, not a safeguard.
const terminalIdleTimeout = 30 * time.Minute

// terminalIdleCheckInterval is how often the watchdog re-checks
// idleness, trading a little imprecision at the deadline for a timer
// that fires a handful of times per session rather than per frame.
const terminalIdleCheckInterval = time.Minute

// terminalWriteTimeout bounds one frame's write to a browser that has
// stopped reading, so a stalled client cannot pin the output pump.
const terminalWriteTimeout = 30 * time.Second

// terminalReadLimit caps one inbound frame. Keystrokes and pastes are
// small; anything past this is not a terminal client.
const terminalReadLimit = 1 << 20 // 1 MiB

// defaultTerminalCommand is the shell a session gets when the caller
// does not name one: bash where the image has it, plain sh everywhere
// else, decided inside the container rather than guessed out here.
var defaultTerminalCommand = []string{
	"/bin/sh", "-c",
	"if command -v bash >/dev/null 2>&1; then exec bash; fi; exec sh",
}

// terminalControl is the JSON a client sends as a text frame. Only
// resize exists today; the type tag is what lets a later one be added
// without changing how terminal bytes travel.
type terminalControl struct {
	Type string `json:"type"`
	Rows uint16 `json:"rows,omitempty"`
	Cols uint16 `json:"cols,omitempty"`
}

// terminalEvent is the JSON the server sends as a text frame: how the
// session ended, which a raw byte stream has no way to express.
type terminalEvent struct {
	Type     string `json:"type"`
	ExitCode *int   `json:"exit_code,omitempty"`
	Message  string `json:"message,omitempty"`
}

// handleAppTerminal handles GET /api/v1/apps/{name}/terminal. Every
// failure before the upgrade is a normal HTTP error; after the upgrade
// there is no status code left to send, so failures become a close
// frame with a reason instead.
func (rt *Router) handleAppTerminal(w http.ResponseWriter, r *http.Request) {
	if rt.execRuntime == nil {
		writeError(w, http.StatusNotImplemented, "exec is not configured on this control plane")
		return
	}

	name := r.PathValue("name")
	svc, ok := rt.loadExecApp(w, r, name)
	if !ok {
		return
	}
	nodeRuntime, state, ok := rt.resolveExecContainer(w, r, svc)
	if !ok {
		return
	}
	tty, ok := nodeRuntime.(docker.TTYRuntime)
	if !ok {
		writeError(w, http.StatusNotImplemented, "this app's node does not support interactive terminals")
		return
	}

	size := terminalSizeFromQuery(r)
	cmd := defaultTerminalCommand
	if requested := r.URL.Query()["command"]; len(requested) > 0 {
		cmd = requested
	}

	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{CompressionMode: websocket.CompressionDisabled})
	if err != nil {
		// Accept has already written its own HTTP error.
		rt.logger.Warn("api: terminal: websocket upgrade failed", slog.String("error", err.Error()), slog.String("name", name))
		return
	}
	conn.SetReadLimit(terminalReadLimit)

	// Deliberately not r.Context(): some servers cancel a hijacked
	// request's context on upgrade, and this session outlives the
	// request either way. The idle watchdog and the client's own close
	// are what bound it.
	ctx, cancel := context.WithCancel(context.WithoutCancel(r.Context()))
	defer cancel()

	sess, err := tty.ExecTTY(ctx, state.ID, docker.ExecTTYOptions{
		Cmd:  cmd,
		Env:  []string{"TERM=xterm-256color"},
		Size: size,
	})
	if err != nil {
		rt.logger.Error("api: terminal: open session failed",
			slog.String("error", err.Error()), slog.String("name", name), slog.String("container", state.ID))
		_ = conn.Close(websocket.StatusInternalError, "could not open a terminal in this container")
		return
	}

	rt.runTerminalSession(ctx, cancel, conn, sess, name)
}

// runTerminalSession pumps both directions until either end stops, then
// tears down both. Closing sess is what actually ends the shell on the
// node, so it happens on every exit path, including the browser tab
// simply disappearing.
func (rt *Router) runTerminalSession(ctx context.Context, cancel context.CancelFunc, conn *websocket.Conn, sess docker.ExecSession, name string) {
	defer func() {
		_ = sess.Close()
		_ = conn.CloseNow()
	}()

	activity := newTerminalActivity()
	go watchTerminalIdle(ctx, activity, cancel)

	outcome := make(chan error, 1)
	go func() { outcome <- rt.pumpTerminalOutput(ctx, conn, sess, activity) }()

	go func() {
		rt.pumpTerminalInput(ctx, conn, sess, activity, name)
		// The client hung up: ending the session here is what stops the
		// remote shell instead of leaving it attached to nobody.
		cancel()
	}()

	select {
	case err := <-outcome:
		rt.closeTerminal(ctx, conn, err)
	case <-ctx.Done():
	}
}

// terminalActivity is the last moment either direction carried a frame,
// shared by both pumps and the idle watchdog.
type terminalActivity struct {
	lastUnixNano atomic.Int64
}

func newTerminalActivity() *terminalActivity {
	a := &terminalActivity{}
	a.touch()
	return a
}

func (a *terminalActivity) touch() { a.lastUnixNano.Store(time.Now().UnixNano()) }

func (a *terminalActivity) idleFor() time.Duration {
	return time.Since(time.Unix(0, a.lastUnixNano.Load()))
}

// watchTerminalIdle ends a session nobody has used in terminalIdleTimeout.
func watchTerminalIdle(ctx context.Context, activity *terminalActivity, cancel context.CancelFunc) {
	ticker := time.NewTicker(terminalIdleCheckInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if activity.idleFor() >= terminalIdleTimeout {
				cancel()
				return
			}
		}
	}
}

// pumpTerminalOutput forwards the PTY's bytes to the browser as binary
// frames, returning the error that ended the stream.
func (rt *Router) pumpTerminalOutput(ctx context.Context, conn *websocket.Conn, sess docker.ExecSession, activity *terminalActivity) error {
	buf := make([]byte, 32<<10)
	for {
		n, readErr := sess.Read(buf)
		if n > 0 {
			activity.touch()
			writeCtx, cancelWrite := context.WithTimeout(ctx, terminalWriteTimeout)
			err := conn.Write(writeCtx, websocket.MessageBinary, buf[:n])
			cancelWrite()
			if err != nil {
				return err
			}
		}
		if readErr != nil {
			return readErr
		}
	}
}

// pumpTerminalInput forwards the browser's frames to the PTY: binary
// frames are keystrokes, text frames are control messages.
func (rt *Router) pumpTerminalInput(ctx context.Context, conn *websocket.Conn, sess docker.ExecSession, activity *terminalActivity, name string) {
	for {
		kind, data, err := conn.Read(ctx)
		if err != nil {
			return
		}
		activity.touch()
		if kind == websocket.MessageText {
			rt.applyTerminalControl(ctx, sess, data, name)
			continue
		}
		if _, err := sess.Write(data); err != nil {
			return
		}
	}
}

func (rt *Router) applyTerminalControl(ctx context.Context, sess docker.ExecSession, data []byte, name string) {
	var msg terminalControl
	if err := json.Unmarshal(data, &msg); err != nil {
		rt.logger.Warn("api: terminal: unreadable control message", slog.String("error", err.Error()), slog.String("name", name))
		return
	}
	if msg.Type != "resize" || msg.Rows == 0 || msg.Cols == 0 {
		return
	}
	if err := sess.Resize(ctx, docker.TTYSize{Rows: msg.Rows, Cols: msg.Cols}); err != nil {
		rt.logger.Warn("api: terminal: resize failed", slog.String("error", err.Error()), slog.String("name", name))
	}
}

// closeTerminal reports how the session ended as one last text frame,
// then closes. A shell exiting normally is not an error here, it is the
// expected way a terminal ends.
func (rt *Router) closeTerminal(ctx context.Context, conn *websocket.Conn, err error) {
	event := terminalEvent{Type: "exit"}
	status := websocket.StatusNormalClosure
	reason := "session ended"

	var exitErr *docker.ExecExitError
	switch {
	case err == nil || errors.Is(err, io.EOF):
		code := 0
		event.ExitCode = &code
	case errors.As(err, &exitErr):
		code := exitErr.ExitCode
		event.ExitCode = &code
	default:
		event.Message = err.Error()
		status = websocket.StatusInternalError
		reason = "session failed"
	}

	writeCtx, cancel := context.WithTimeout(ctx, terminalWriteTimeout)
	defer cancel()
	if payload, marshalErr := json.Marshal(event); marshalErr == nil {
		_ = conn.Write(writeCtx, websocket.MessageText, payload)
	}
	_ = conn.Close(status, reason)
}

// terminalSizeFromQuery reads the terminal's initial size off the
// upgrade request, falling back to a conventional 80x24 so a client
// that forgot to send one still gets a sanely sized PTY.
func terminalSizeFromQuery(r *http.Request) docker.TTYSize {
	size := docker.TTYSize{Rows: 24, Cols: 80}
	if rows, err := strconv.ParseUint(r.URL.Query().Get("rows"), 10, 16); err == nil && rows > 0 {
		size.Rows = uint16(rows)
	}
	if cols, err := strconv.ParseUint(r.URL.Query().Get("cols"), 10, 16); err == nil && cols > 0 {
		size.Cols = uint16(cols)
	}
	return size
}
