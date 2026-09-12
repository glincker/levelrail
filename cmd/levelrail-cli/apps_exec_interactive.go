package main

// "apps exec --interactive": a real terminal on an app's container,
// over the same WebSocket the dashboard's terminal uses
// (internal/api/terminal.go). Wire shapes are redeclared here rather
// than imported, the same convention client.go's own doc comment
// establishes for every other wire type in this package.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/coder/websocket"
	"golang.org/x/term"
)

// terminalCredentials is the already-resolved API target an
// interactive session dials, kept as one value so this path does not
// grow a run of bare string parameters.
type terminalCredentials struct {
	APIURL string
	Token  string
}

// terminalControlMessage is what the client sends as a text frame.
type terminalControlMessage struct {
	Type string `json:"type"`
	Rows uint16 `json:"rows,omitempty"`
	Cols uint16 `json:"cols,omitempty"`
}

// terminalExitEvent is the server's final text frame: how the session
// ended, which the raw byte stream cannot carry.
type terminalExitEvent struct {
	Type     string `json:"type"`
	ExitCode *int   `json:"exit_code,omitempty"`
	Message  string `json:"message,omitempty"`
}

// terminalStdinChunk bounds one keystroke frame. Terminal input is tiny
// except for pastes, which this splits rather than buffering whole.
const terminalStdinChunk = 4 << 10

// terminalURL turns the CLI's own API base URL into the WebSocket URL
// for one app's terminal, carrying the initial size and, when the caller
// named one, the command to run.
func terminalURL(apiURL, name string, command []string, rows, cols int) (string, error) {
	base, err := url.Parse(strings.TrimRight(apiURL, "/"))
	if err != nil {
		return "", fmt.Errorf("invalid API URL %q: %w", apiURL, err)
	}
	switch base.Scheme {
	case "http":
		base.Scheme = "ws"
	case "https":
		base.Scheme = "wss"
	default:
		return "", fmt.Errorf("invalid API URL %q: expected an http or https scheme", apiURL)
	}

	// Path and RawPath together, not a single escaped string: URL.String
	// re-encodes Path, so an already-escaped name would be escaped twice.
	prefix := strings.TrimRight(base.Path, "/") + "/api/v1/apps/"
	base.Path = prefix + name + "/terminal"
	base.RawPath = prefix + pathEscape(name) + "/terminal"

	query := url.Values{}
	query.Set("rows", strconv.Itoa(rows))
	query.Set("cols", strconv.Itoa(cols))
	for _, arg := range command {
		query.Add("command", arg)
	}
	base.RawQuery = query.Encode()
	return base.String(), nil
}

// runAppsExecInteractiveFromFlags is "apps exec --interactive"'s entry
// point: the app name is required, the command after "--" is optional
// (the server picks a shell when none is given).
func runAppsExecInteractiveFromFlags(args []string, prog string, creds credentialFlags, lookupEnv func(string) (string, bool), stdout, stderr io.Writer) int {
	if len(args) < 1 {
		_, _ = fmt.Fprintf(stderr, "%s: apps exec --interactive requires an app name, e.g. \"%s apps exec web --interactive\"\n", prog, prog)
		return exitUsage
	}
	profile := resolveProfile(creds.Profile, lookupEnv)
	return runAppsExecInteractive(prog, args[0], args[1:], terminalCredentials{
		APIURL: resolveAPIURL(creds.APIURL, lookupEnv, prog, profile),
		Token:  resolveToken(creds.Token, lookupEnv, prog, profile),
	}, stdout, stderr)
}

// runAppsExecInteractive opens the terminal, puts the local terminal in
// raw mode so keystrokes reach the container unbuffered, and returns the
// remote shell's own exit code, the same contract the non-interactive
// path already has.
func runAppsExecInteractive(prog, name string, command []string, creds terminalCredentials, stdout, stderr io.Writer) int {
	fd := int(os.Stdin.Fd())
	if !term.IsTerminal(fd) {
		_, _ = fmt.Fprintf(stderr, "%s: --interactive needs a terminal on stdin; drop it to run a single command and read its output\n", prog)
		return exitUsage
	}
	cols, rows, err := term.GetSize(fd)
	if err != nil || cols <= 0 || rows <= 0 {
		cols, rows = 80, 24
	}

	target, err := terminalURL(creds.APIURL, name, command, rows, cols)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "%s: %v\n", prog, err)
		return exitValidation
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	header := http.Header{}
	if creds.Token != "" {
		header.Set("Authorization", "Bearer "+creds.Token)
	}
	conn, resp, err := websocket.Dial(ctx, target, &websocket.DialOptions{HTTPHeader: header})
	if err != nil {
		return reportTerminalDialError(prog, stderr, resp, err)
	}
	defer func() { _ = conn.CloseNow() }()
	conn.SetReadLimit(terminalReadLimitBytes)

	restore, err := term.MakeRaw(fd)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "%s: could not put this terminal in raw mode: %v\n", prog, err)
		return exitValidation
	}
	defer func() { _ = term.Restore(fd, restore) }()

	go pumpTerminalStdin(ctx, conn)
	go pumpTerminalResizes(ctx, conn, fd)

	return drainTerminal(ctx, conn, prog, stdout, stderr)
}

// terminalReadLimitBytes mirrors the server's own per-frame cap
// (internal/api/terminal.go), applied here so a misbehaving server
// cannot make this process allocate without bound either.
const terminalReadLimitBytes = 1 << 20

func reportTerminalDialError(prog string, stderr io.Writer, resp *http.Response, err error) int {
	if resp == nil {
		_, _ = fmt.Fprintf(stderr, "%s: could not open a terminal: %v\n", prog, err)
		return exitNetwork
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, terminalReadLimitBytes))
	message := extractErrorMessage(body)
	if message == "" {
		message = resp.Status
	}
	_, _ = fmt.Fprintf(stderr, "%s: could not open a terminal: %s\n", prog, message)
	return exitAPIError
}

// pumpTerminalStdin forwards local keystrokes as binary frames.
func pumpTerminalStdin(ctx context.Context, conn *websocket.Conn) {
	buf := make([]byte, terminalStdinChunk)
	for {
		n, err := os.Stdin.Read(buf)
		if n > 0 {
			if writeErr := conn.Write(ctx, websocket.MessageBinary, buf[:n]); writeErr != nil {
				return
			}
		}
		if err != nil {
			return
		}
	}
}

// pumpTerminalResizes tells the remote PTY whenever this terminal
// changes size, so full-screen programs redraw at the right dimensions.
func pumpTerminalResizes(ctx context.Context, conn *websocket.Conn, fd int) {
	resized, stop := notifyResize()
	defer stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-resized:
			cols, rows, err := term.GetSize(fd)
			if err != nil || cols <= 0 || rows <= 0 {
				continue
			}
			payload, err := json.Marshal(terminalControlMessage{
				Type: "resize",
				Rows: uint16(rows), //nolint:gosec // term.GetSize never reports a dimension outside a terminal's real range
				Cols: uint16(cols), //nolint:gosec // same
			})
			if err != nil {
				continue
			}
			if err := conn.Write(ctx, websocket.MessageText, payload); err != nil {
				return
			}
		}
	}
}

// drainTerminal writes the remote terminal's bytes straight to stdout
// and returns the exit code its final control frame reports.
func drainTerminal(ctx context.Context, conn *websocket.Conn, prog string, stdout, stderr io.Writer) int {
	for {
		kind, data, err := conn.Read(ctx)
		if err != nil {
			var closeErr websocket.CloseError
			if errors.As(err, &closeErr) && closeErr.Code == websocket.StatusNormalClosure {
				return exitOK
			}
			_, _ = fmt.Fprintf(stderr, "\r\n%s: terminal session ended: %v\r\n", prog, err)
			return exitNetwork
		}
		if kind == websocket.MessageBinary {
			_, _ = stdout.Write(data)
			continue
		}

		var event terminalExitEvent
		if err := json.Unmarshal(data, &event); err != nil || event.Type != "exit" {
			continue
		}
		if event.Message != "" {
			_, _ = fmt.Fprintf(stderr, "\r\n%s: %s\r\n", prog, event.Message)
			return exitAPIError
		}
		if event.ExitCode != nil {
			return *event.ExitCode
		}
		return exitOK
	}
}
