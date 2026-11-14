package cloudiac

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"sync/atomic"
	"time"
)

// RunOutput is what one sandboxed step printed, each stream capped.
type RunOutput struct {
	Stdout    []byte
	Stderr    []byte
	Truncated bool
}

// Runner executes one sandboxed step. Tests swap in a fake so no container,
// binary or cloud is ever touched.
type Runner interface {
	Run(ctx context.Context, spec SandboxSpec) (RunOutput, error)
}

// maxOutputBytes caps captured output so a runaway step cannot exhaust memory.
func maxOutputBytes() int64 {
	if v, err := strconv.Atoi(os.Getenv("APP_CLOUD_ENGINE_MAX_OUTPUT_KB")); err == nil && v > 0 {
		return int64(v) * 1024
	}
	return 16 * 1024 * 1024
}

// ExecRunner shells to docker or podman with the argv BuildSandboxSpec produced.
type ExecRunner struct {
	// MaxBytes caps each stream; zero uses the env seeded default.
	MaxBytes int64
	// command builds every process this runner starts; tests swap it so no runtime is touched.
	command func(ctx context.Context, argv []string) *exec.Cmd
}

// stepNameRe is the only shape a generated container name takes, so argv never carries user input.
var stepNameRe = regexp.MustCompile(`^lr-cloud-[0-9a-f]{12}$`)

func newContainerName() (string, error) {
	b := make([]byte, 6)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return "lr-cloud-" + hex.EncodeToString(b), nil
}

func envSeconds(key string, def time.Duration) time.Duration {
	if v, err := strconv.Atoi(os.Getenv(key)); err == nil && v > 0 {
		return time.Duration(v) * time.Second
	}
	return def
}

// stepWaitDelay is how long a cancelled step gets after the interrupt before it is killed.
func stepWaitDelay() time.Duration {
	return envSeconds("APP_CLOUD_ENGINE_STOP_GRACE_SECONDS", 5*time.Second)
}

// removeTimeout bounds the best effort container removal after a cancelled step.
func removeTimeout() time.Duration {
	return envSeconds("APP_CLOUD_ENGINE_REMOVE_TIMEOUT_SECONDS", 15*time.Second)
}

// spawn is the single place this runner starts a process: argv form, no shell, PATH only env.
func (r ExecRunner) spawn(ctx context.Context, argv []string) *exec.Cmd {
	var cmd *exec.Cmd
	if r.command != nil {
		cmd = r.command(ctx, argv)
	} else {
		cmd = exec.CommandContext(ctx, argv[0], argv[1:]...) //nolint:gosec // argv is the validated sandbox spec or a fixed rm of a generated name
	}
	cmd.Env = []string{"PATH=" + os.Getenv("PATH")}
	return cmd
}

// Run starts the container and waits; the caller's context carries the timeout. A cancelled
// step is interrupted, killed after the wait delay, and its named container removed.
func (r ExecRunner) Run(ctx context.Context, spec SandboxSpec) (RunOutput, error) {
	if spec.Runtime != "docker" && spec.Runtime != "podman" {
		return RunOutput{}, ErrSandboxSpec
	}
	name, err := newContainerName()
	if err != nil {
		return RunOutput{}, err
	}
	spec.Name = name
	argv := spec.Argv()
	limit := r.MaxBytes
	if limit <= 0 {
		limit = maxOutputBytes()
	}
	stdout, stderr := &cappedBuffer{max: limit}, &cappedBuffer{max: limit}
	cmd := r.spawn(ctx, argv)
	cmd.Stdout, cmd.Stderr = stdout, stderr
	cmd.WaitDelay = stepWaitDelay()
	var cancelled atomic.Bool
	cmd.Cancel = func() error {
		cancelled.Store(true)
		return cmd.Process.Signal(os.Interrupt)
	}
	err = cmd.Run()
	if cancelled.Load() {
		r.removeContainer(spec.Runtime, name)
	}
	out := RunOutput{Stdout: stdout.Bytes(), Stderr: stderr.Bytes(), Truncated: stdout.cut || stderr.cut}
	if ctx.Err() != nil {
		return out, ctx.Err()
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return out, errStepExit
	}
	return out, err
}

// removeContainer runs a fixed `<runtime> rm -f <name>`; a step that ignored the interrupt
// must not keep running after its caller gave up. Best effort under its own timeout.
func (r ExecRunner) removeContainer(runtime, name string) {
	if (runtime != "docker" && runtime != "podman") || !stepNameRe.MatchString(name) {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), removeTimeout())
	defer cancel()
	_ = r.spawn(ctx, []string{runtime, "rm", "-f", name}).Run()
}

var errStepExit = errors.New("cloudiac: step exited with an error")

// cappedBuffer keeps the first max bytes and drops the rest without failing the writer.
type cappedBuffer struct {
	buf bytes.Buffer
	max int64
	cut bool
}

func (c *cappedBuffer) Write(p []byte) (int, error) {
	room := c.max - int64(c.buf.Len())
	if room <= 0 {
		c.cut = c.cut || len(p) > 0
		return len(p), nil
	}
	if int64(len(p)) > room {
		c.buf.Write(p[:room])
		c.cut = true
		return len(p), nil
	}
	c.buf.Write(p)
	return len(p), nil
}

func (c *cappedBuffer) Bytes() []byte { return c.buf.Bytes() }
