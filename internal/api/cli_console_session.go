package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/coder/websocket"

	"github.com/GLINCKER/levelrail/internal/cliconsole"
)

// Frozen CLI console protocol message types.
const (
	cliMsgRun         = "run"
	cliMsgCancel      = "cancel"
	cliMsgPing        = "ping"
	cliMsgReady       = "ready"
	cliMsgOutput      = "output"
	cliMsgExit        = "exit"
	cliMsgIdleTimeout = "idle_timeout"
	cliMsgError       = "error"

	cliConsoleWriteTimeout = 10 * time.Second
	cliConsolePingInterval = 30 * time.Second
)

type cliClientMessage struct {
	Type string `json:"type"`
	Line string `json:"line,omitempty"`
}

type cliServerMessage struct {
	Type    string `json:"type"`
	Server  string `json:"server,omitempty"`
	Stream  string `json:"stream,omitempty"`
	Data    string `json:"data,omitempty"`
	Code    *int   `json:"code,omitempty"`
	Message string `json:"message,omitempty"`
}

type cliConsoleSession struct {
	rt      *Router
	conn    *websocket.Conn
	req     *http.Request
	caller  cliConsoleCaller
	token   string
	cfg     cliconsole.Config
	runner  *cliconsole.Runner
	writeMu sync.Mutex
}

type cliRunDone struct{}

type cliActiveRun struct {
	cancel context.CancelFunc
	done   chan cliRunDone
}

// send writes one frame; any failure ends the session through cancel.
func (s *cliConsoleSession) send(ctx context.Context, cancel context.CancelFunc, m cliServerMessage) error {
	payload, err := json.Marshal(m)
	if err != nil {
		return fmt.Errorf("cli console: encode %s: %w", m.Type, err)
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	wctx, wcancel := context.WithTimeout(ctx, cliConsoleWriteTimeout)
	defer wcancel()
	if err := s.conn.Write(wctx, websocket.MessageText, payload); err != nil {
		cancel()
		return fmt.Errorf("cli console: write %s: %w", m.Type, err)
	}
	return nil
}

// serve runs the session until the client leaves, idles out, loses access
// or hits the session cap. One command runs at a time.
func (s *cliConsoleSession) serve(ctx context.Context, cancel context.CancelFunc) {
	msgs := make(chan cliClientMessage)
	go s.readLoop(ctx, cancel, msgs)
	go s.pingLoop(ctx, cancel)

	host, _ := os.Hostname()
	if s.send(ctx, cancel, cliServerMessage{Type: cliMsgReady, Server: host}) != nil {
		return
	}

	idle := time.NewTimer(s.cfg.IdleTimeout)
	defer idle.Stop()
	var run *cliActiveRun
	var runDone chan cliRunDone
	defer func() {
		if run != nil {
			run.cancel()
			<-run.done
		}
	}()
	for {
		select {
		case <-ctx.Done():
			_ = s.conn.Close(websocket.StatusNormalClosure, "session ended")
			return
		case <-runDone:
			run, runDone = nil, nil
			idle.Reset(s.cfg.IdleTimeout)
		case <-idle.C:
			if runDone != nil {
				idle.Reset(s.cfg.IdleTimeout)
				continue
			}
			_ = s.send(ctx, cancel, cliServerMessage{Type: cliMsgIdleTimeout})
			_ = s.conn.Close(websocket.StatusNormalClosure, "idle timeout")
			cancel()
			return
		case m := <-msgs:
			switch m.Type {
			case cliMsgRun:
				idle.Reset(s.cfg.IdleTimeout)
				if runDone != nil {
					// A run releases its slot before its exit frame goes out, so a client that
					// saw the exit may send the next line before the loop read runDone.
					select {
					case <-runDone:
						run, runDone = nil, nil
					default:
					}
				}
				if runDone != nil {
					_ = s.send(ctx, cancel, cliServerMessage{Type: cliMsgError, Message: "a command is already running; cancel it first"})
					continue
				}
				argv, path, ok := s.vet(ctx, cancel, m.Line)
				if !ok {
					continue
				}
				rctx, rcancel := context.WithCancel(ctx)
				active := &cliActiveRun{cancel: rcancel, done: make(chan cliRunDone)}
				release := sync.OnceFunc(func() { close(active.done) })
				run, runDone = active, active.done
				if cliconsole.IsHost(argv) {
					go s.executeHost(rctx, cancel, path, release)
				} else {
					go s.execute(rctx, cancel, argv, path, release)
				}
			case cliMsgCancel:
				idle.Reset(s.cfg.IdleTimeout)
				if run != nil {
					run.cancel()
				}
			case cliMsgPing:
			default:
				_ = s.send(ctx, cancel, cliServerMessage{Type: cliMsgError, Message: "unknown message type"})
			}
		}
	}
}

// vet parses and checks a line; refusals are audited and reported, never run.
func (s *cliConsoleSession) vet(ctx context.Context, cancel context.CancelFunc, line string) ([]string, string, bool) {
	argv, err := cliconsole.ParseLine(line)
	path := ""
	switch {
	case err != nil:
	case cliconsole.IsHost(argv):
		path, err = vetHostLine(argv)
	default:
		path, err = cliconsole.Check(argv)
	}
	if err != nil {
		shown := "[unparsable line]"
		if argv != nil {
			shown = cliconsole.Redact(argv, path)
		}
		s.rt.auditCLIConsole(s.req, s.caller, shown+" (refused: "+err.Error()+")", http.StatusForbidden, auditActionCLIConsoleRejected)
		_ = s.send(ctx, cancel, cliServerMessage{Type: cliMsgError, Message: err.Error()})
		return nil, "", false
	}
	return argv, path, true
}

func (s *cliConsoleSession) execute(ctx context.Context, cancel context.CancelFunc, argv []string, path string, done func()) {
	defer done()
	start := time.Now()
	res, err := s.runner.Run(ctx, argv, s.token, func(stream string, data []byte) error {
		return s.send(ctx, cancel, cliServerMessage{Type: cliMsgOutput, Stream: stream, Data: string(data)})
	})
	took := time.Since(start).Round(time.Millisecond)
	shown := cliconsole.Redact(argv, path)
	if err != nil {
		s.rt.logger.Warn("api: cli console: run failed", slog.String("user_id", s.caller.userID), slog.String("error", err.Error()))
		s.rt.auditCLIConsole(s.req, s.caller, fmt.Sprintf("%s (failed to run, %s)", shown, took), http.StatusInternalServerError, auditActionCLIConsoleRun)
		done()
		_ = s.send(ctx, cancel, cliServerMessage{Type: cliMsgError, Message: "the command could not be run"})
		return
	}
	note := ""
	switch {
	case res.TimedOut:
		note = ", timed out"
	case res.Cancelled:
		note = ", cancelled"
	case res.Truncated:
		note = ", output truncated"
	}
	s.rt.auditCLIConsole(s.req, s.caller, fmt.Sprintf("%s (exit %d, %s%s)", shown, res.ExitCode, took, note), http.StatusOK, auditActionCLIConsoleRun)
	if res.TimedOut {
		_ = s.send(ctx, cancel, cliServerMessage{Type: cliMsgOutput, Stream: cliconsole.StreamStderr, Data: fmt.Sprintf("\n[stopped after %s]\n", s.cfg.CommandTimeout)})
	}
	code := res.ExitCode
	done()
	// The session context may be cancelled already; exit then goes nowhere, which is fine.
	_ = s.send(context.WithoutCancel(ctx), cancel, cliServerMessage{Type: cliMsgExit, Code: &code})
}

func (s *cliConsoleSession) readLoop(ctx context.Context, cancel context.CancelFunc, out chan<- cliClientMessage) {
	defer cancel()
	for {
		kind, data, err := s.conn.Read(ctx)
		if err != nil {
			return
		}
		var m cliClientMessage
		if kind != websocket.MessageText || json.Unmarshal(data, &m) != nil {
			m = cliClientMessage{Type: "invalid"}
		}
		select {
		case out <- m:
		case <-ctx.Done():
			return
		}
	}
}

// pingLoop notices a vanished client between commands; it never resets idleness.
func (s *cliConsoleSession) pingLoop(ctx context.Context, cancel context.CancelFunc) {
	t := time.NewTicker(cliConsolePingInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			pctx, pcancel := context.WithTimeout(ctx, cliConsoleWriteTimeout)
			err := s.conn.Ping(pctx)
			pcancel()
			if err != nil {
				cancel()
				return
			}
		}
	}
}
