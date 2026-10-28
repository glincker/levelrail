package cliconsole

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"time"
)

// Output streams.
const (
	StreamStdout = "stdout"
	StreamStderr = "stderr"
)

// Runner executes one vetted argv as the CLI binary, with no shell.
type Runner struct {
	Binary         string
	APIURL         string
	Experimental   string
	Timeout        time.Duration
	MaxOutputBytes int64
	MaxLineBytes   int
	RunAsUser      string
}

// Result is how one command ended.
type Result struct {
	ExitCode  int
	TimedOut  bool
	Truncated bool
	Cancelled bool
}

// Env builds the child's whole environment: nothing inherited from the
// control plane, only the loopback API address and the session token.
func Env(apiURL, token, home, experimental string) []string {
	env := []string{
		"APP_API_URL=" + apiURL,
		"APP_API_TOKEN=" + token,
		"HOME=" + home,
		"TERM=dumb",
		"NO_COLOR=1",
		"LANG=C.UTF-8",
	}
	if experimental != "" {
		env = append(env, "APP_EXPERIMENTAL="+experimental)
	}
	return env
}

// Run executes argv and streams output through emit, which is never called
// concurrently. ctx cancellation kills the whole process group.
func (r *Runner) Run(ctx context.Context, argv []string, token string, emit func(stream string, data []byte) error) (Result, error) {
	cred, err := dropCredential(r.RunAsUser)
	if err != nil {
		return Result{}, err
	}
	home, err := os.MkdirTemp("", "cli-console-")
	if err != nil {
		return Result{}, fmt.Errorf("cli console: temp dir: %w", err)
	}
	defer func() { _ = os.RemoveAll(home) }()
	if err := chownForChild(home, cred); err != nil {
		return Result{}, err
	}

	runCtx, cancel := context.WithTimeout(ctx, r.Timeout)
	defer cancel()
	cmd := exec.CommandContext(runCtx, r.Binary, argv...) //nolint:gosec // argv passed Check; binary is vetted by LocateBinary
	cmd.Dir = home
	cmd.Env = Env(r.APIURL, token, home, r.Experimental)
	cmd.Stdin = nil
	// Only stdout and stderr reach the child; Go marks every other fd close-on-exec.
	cmd.ExtraFiles = nil
	configureProcess(cmd, cred)
	cmd.WaitDelay = 2 * time.Second

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return Result{}, fmt.Errorf("cli console: stdout pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return Result{}, fmt.Errorf("cli console: stderr pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return Result{}, fmt.Errorf("cli console: start CLI: %w", err)
	}

	oc := &outputCap{limit: r.MaxOutputBytes, emit: emit, kill: cancel}
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		oc.pump(StreamStdout, stdout, &lineFilter{maxLine: r.MaxLineBytes, secrets: []string{token}})
	}()
	go func() {
		defer wg.Done()
		oc.pump(StreamStderr, stderr, &lineFilter{maxLine: r.MaxLineBytes, secrets: []string{token}})
	}()
	wg.Wait()
	waitErr := cmd.Wait()

	res := Result{Truncated: oc.truncated}
	switch {
	case errors.Is(runCtx.Err(), context.DeadlineExceeded) && ctx.Err() == nil:
		res.TimedOut = true
	case ctx.Err() != nil:
		res.Cancelled = true
	}
	var exitErr *exec.ExitError
	switch {
	case waitErr == nil:
		res.ExitCode = 0
	case errors.As(waitErr, &exitErr):
		res.ExitCode = exitErr.ExitCode()
		if res.ExitCode < 0 {
			res.ExitCode = 137
		}
	default:
		return res, fmt.Errorf("cli console: wait CLI: %w", waitErr)
	}
	if oc.emitErr != nil {
		return res, fmt.Errorf("cli console: send output: %w", oc.emitErr)
	}
	return res, nil
}

// outputCap serialises both pipes into emit and kills the child past limit.
type outputCap struct {
	mu        sync.Mutex
	limit     int64
	sent      int64
	truncated bool
	emitErr   error
	emit      func(string, []byte) error
	kill      context.CancelFunc
}

// pump sanitizes before counting, so the cap and the browser see the same bytes.
func (c *outputCap) pump(stream string, rd io.Reader, f *lineFilter) {
	buf := make([]byte, 16<<10)
	for {
		n, err := rd.Read(buf)
		if n > 0 {
			if out := f.write(buf[:n]); len(out) > 0 {
				c.write(stream, out)
			}
		}
		if err != nil {
			if out := f.flush(); len(out) > 0 {
				c.write(stream, out)
			}
			return
		}
	}
}

func (c *outputCap) write(stream string, p []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.truncated || c.emitErr != nil {
		return
	}
	if room := c.limit - c.sent; int64(len(p)) > room {
		p = p[:max(room, 0)]
		c.truncated = true
	}
	if len(p) > 0 {
		c.sent += int64(len(p))
		c.emitErr = c.emit(stream, append([]byte(nil), p...))
	}
	if c.truncated && c.emitErr == nil {
		c.emitErr = c.emit(StreamStderr, []byte(fmt.Sprintf("\n[output truncated at %d bytes; the command was stopped]\n", c.limit)))
	}
	if c.truncated || c.emitErr != nil {
		c.kill()
	}
}
