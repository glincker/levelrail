package cloudiac

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"strconv"
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
}

// Run starts the container and waits; the caller's context carries the timeout.
func (r ExecRunner) Run(ctx context.Context, spec SandboxSpec) (RunOutput, error) {
	argv := spec.Argv()
	if spec.Runtime != "docker" && spec.Runtime != "podman" {
		return RunOutput{}, ErrSandboxSpec
	}
	limit := r.MaxBytes
	if limit <= 0 {
		limit = maxOutputBytes()
	}
	stdout, stderr := &cappedBuffer{max: limit}, &cappedBuffer{max: limit}
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...) //nolint:gosec // argv is the validated sandbox spec
	cmd.Stdout, cmd.Stderr = stdout, stderr
	cmd.Env = []string{"PATH=" + os.Getenv("PATH")}
	err := cmd.Run()
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
