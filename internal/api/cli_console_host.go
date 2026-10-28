package api

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/cliconsole"
	"github.com/GLINCKER/levelrail/internal/hostdiag"
	"github.com/GLINCKER/levelrail/internal/version"
)

var errCLIConsoleHostDisabled = errors.New("host diagnostics are turned off on this server (" + hostdiag.EnvSwitch + "=off)")

// vetHostLine returns the audit path "host <sub>" for an accepted host line.
func vetHostLine(argv []string) (string, error) {
	if !hostdiag.Enabled() {
		return hostdiag.Command, errCLIConsoleHostDisabled
	}
	sub, err := hostdiag.Check(argv)
	if err != nil {
		return hostdiag.Command, err
	}
	return hostdiag.Command + " " + sub, nil
}

func (rt *Router) hostDiagInfo() hostdiag.Info {
	rt.restart.mu.Lock()
	supervised := rt.restart.r.Supervised
	rt.restart.mu.Unlock()
	return hostdiag.Info{DataDir: rt.dataDir, Version: version.Version, Supervised: supervised}
}

type hostDiagResult struct {
	text string
	err  error
}

// executeHost answers a host line in process, through the same output cap,
// audit row and exit frame as a CLI run.
func (s *cliConsoleSession) executeHost(ctx context.Context, cancel context.CancelFunc, path string, done func()) {
	defer done()
	start := time.Now()
	sub := strings.TrimPrefix(path, hostdiag.Command+" ")
	info := s.rt.hostDiagInfo()

	// A hung statfs cannot be interrupted, so wait on it with the command timeout.
	results := make(chan hostDiagResult, 1)
	go func() {
		text, err := hostdiag.Run(sub, info)
		results <- hostDiagResult{text: text, err: err}
	}()
	tctx, tcancel := context.WithTimeout(ctx, s.cfg.CommandTimeout)
	defer tcancel()
	var res hostDiagResult
	note := ""
	select {
	case res = <-results:
	case <-tctx.Done():
		note = ", cancelled"
		res.err = errors.New("host " + sub + " did not finish")
		if ctx.Err() == nil {
			note = ", timed out"
		}
	}

	code, stream, text := 0, cliconsole.StreamStdout, res.text
	if res.err != nil {
		code, stream, text = 1, cliconsole.StreamStderr, res.err.Error()+"\n"
		var usage *hostdiag.UsageError
		if !errors.As(res.err, &usage) && !errors.Is(res.err, hostdiag.ErrUnsupported) {
			s.rt.logger.Warn("api: cli console: host diagnostic failed", slog.String("user_id", s.caller.userID), slog.String("command", path), slog.String("error", res.err.Error()))
		}
	}
	text = hostdiag.Sanitize(text)
	if limit := s.cfg.MaxOutputBytes; int64(len(text)) > limit {
		text = strings.ToValidUTF8(text[:max(limit, 0)], "") + fmt.Sprintf("\n[output truncated at %d bytes]\n", limit)
		note += ", output truncated"
	}
	took := time.Since(start).Round(time.Millisecond)
	s.rt.auditCLIConsole(s.req, s.caller, fmt.Sprintf("%s (exit %d, %s%s)", path, code, took, note), http.StatusOK, auditActionCLIConsoleRun)
	if text != "" && ctx.Err() == nil {
		if s.send(ctx, cancel, cliServerMessage{Type: cliMsgOutput, Stream: stream, Data: text}) != nil {
			return
		}
	}
	done()
	_ = s.send(context.WithoutCancel(ctx), cancel, cliServerMessage{Type: cliMsgExit, Code: &code})
}
