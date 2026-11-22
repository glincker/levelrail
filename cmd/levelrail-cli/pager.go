package main

import (
	"bytes"
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"golang.org/x/term"
)

const (
	defaultPagerCmd   = "less -FRX"
	pagerConfigFile   = "pager"
	envPager          = "PAGER"
	pagerOffValue     = "off"
	fallbackTermLines = 24
)

// pagerSkipVerbs never page: they stream, prompt or hand the terminal to another program.
var pagerSkipVerbs = map[string]bool{
	"exec": true, "shell": true, "ssh": true, "login": true, "up": true,
	"__complete": true, "completion": true, "tui": true,
}

// extractNoPagerFlag strips --no-pager wherever it appears, like --no-color.
func extractNoPagerFlag(args []string) (remaining []string, set bool) {
	remaining = make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		if args[i] == "--" {
			remaining = append(remaining, args[i:]...)
			break
		}
		if args[i] == "--no-pager" {
			set = true
			continue
		}
		remaining = append(remaining, args[i])
	}
	return remaining, set
}

// pagerCommand resolves the pager live on each run: the per-user config file wins,
// then $PAGER, then the default. "off" or an empty command disables paging.
func pagerCommand(prog string, lookup func(string) (string, bool)) []string {
	spec := defaultPagerCmd
	if v, ok := lookup(envPager); ok && strings.TrimSpace(v) != "" {
		spec = v
	}
	if dir, err := configDir(prog); err == nil {
		if b, err := os.ReadFile(filepath.Join(dir, pagerConfigFile)); err == nil && strings.TrimSpace(string(b)) != "" { //nolint:gosec // fixed file under the user's own config dir
			spec = strings.TrimSpace(string(b))
		}
	}
	if strings.EqualFold(spec, pagerOffValue) {
		return nil
	}
	return strings.Fields(spec)
}

// pagerWanted is true only for an interactive, non-streaming run on a real terminal.
func pagerWanted(args []string, noPager bool, stdoutTTY bool, lookup func(string) (string, bool)) bool {
	if noPager || forceCI || ciEnv(lookup) || !stdoutTTY || len(args) == 0 {
		return false
	}
	if pagerSkipVerbs[args[0]] {
		return false
	}
	for _, a := range args {
		switch a {
		case "-f", "--follow", "--watch", "-w", "--json", "--no-input":
			return false
		}
	}
	return true
}

// lazyPager buffers output until it is taller than the terminal, then pipes it
// through the pager. Short output goes straight to the terminal.
type lazyPager struct {
	dst     *os.File
	argv    []string
	maxRows int
	buf     bytes.Buffer
	cmd     *exec.Cmd
	pipe    io.WriteCloser
	dead    bool
}

func (p *lazyPager) Write(b []byte) (int, error) {
	if p.dead {
		return len(b), nil
	}
	if p.pipe != nil {
		return p.toPager(b)
	}
	p.buf.Write(b)
	if bytes.Count(p.buf.Bytes(), []byte{'\n'}) < p.maxRows {
		return len(b), nil
	}
	pending := p.buf.Bytes()
	if !p.start() {
		_, err := p.dst.Write(pending)
		p.buf.Reset()
		p.dead = true
		return len(b), err
	}
	p.buf.Reset()
	_, _ = p.toPager(pending)
	return len(b), nil
}

// toPager swallows write errors once the user quits the pager early.
func (p *lazyPager) toPager(b []byte) (int, error) {
	if _, err := p.pipe.Write(b); err != nil {
		p.dead = true
	}
	return len(b), nil
}

func (p *lazyPager) start() bool {
	path, err := exec.LookPath(p.argv[0])
	if err != nil {
		return false
	}
	cmd := exec.CommandContext(context.Background(), path, p.argv[1:]...) //nolint:gosec // the user's own configured pager
	cmd.Stdout, cmd.Stderr = p.dst, os.Stderr
	in, err := cmd.StdinPipe()
	if err != nil {
		return false
	}
	if err := cmd.Start(); err != nil {
		return false
	}
	p.cmd, p.pipe = cmd, in
	return true
}

// stop flushes short output or closes the pager and waits so the terminal is restored.
func (p *lazyPager) stop() {
	if p.pipe == nil {
		if p.buf.Len() > 0 && !p.dead {
			_, _ = p.dst.Write(p.buf.Bytes())
		}
		return
	}
	_ = p.pipe.Close()
	_ = p.cmd.Wait()
}

// startPager wraps stdout when paging applies and returns the writer plus its cleanup.
func startPager(prog string, args []string, noPager bool, stdout *os.File, lookup func(string) (string, bool)) (io.Writer, func()) {
	if !pagerWanted(args, noPager, isTerminal(stdout), lookup) {
		return stdout, func() {}
	}
	argv := pagerCommand(prog, lookup)
	if len(argv) == 0 {
		return stdout, func() {}
	}
	rows := fallbackTermLines
	if _, h, err := term.GetSize(int(stdout.Fd())); err == nil && h > 1 { //nolint:gosec // a file descriptor always fits an int
		rows = h
	}
	p := &lazyPager{dst: stdout, argv: argv, maxRows: rows - 1}
	return p, p.stop
}
