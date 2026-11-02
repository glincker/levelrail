package dbaccess

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/docker"
)

type recordingExec struct {
	cmd   []string
	stdin string
	err   error
}

func (r *recordingExec) ExecWithInput(_ context.Context, _ string, cmd []string, stdin io.Reader) (io.ReadCloser, error) {
	r.cmd = cmd
	b, _ := io.ReadAll(stdin)
	r.stdin = string(b)
	if r.err != nil {
		return nil, r.err
	}
	return io.NopCloser(strings.NewReader("main|main|0|0|0|0|0|0|0|0\n")), nil
}

func TestReadPoolerPools_PasswordOnStdinOnly(t *testing.T) {
	ex := &recordingExec{}
	if _, err := ReadPoolerPools(context.Background(), ex, "c1", 6432, "s3cret"); err != nil {
		t.Fatalf("ReadPoolerPools() error = %v", err)
	}
	if ex.stdin != "s3cret\n" {
		t.Errorf("stdin = %q, want the password line", ex.stdin)
	}
	if strings.Contains(strings.Join(ex.cmd, " "), "s3cret") {
		t.Error("password appeared in argv")
	}
}

func TestReadPoolerPools_AuthFailureIsUnavailable(t *testing.T) {
	ex := &recordingExec{err: &docker.ExecExitError{ExitCode: 2, Stderr: "psql: error: FATAL:  password authentication failed for user \"main\""}}
	_, err := ReadPoolerPools(context.Background(), ex, "c1", 6432, "stale")
	if !errors.Is(err, ErrPoolerStatsUnavailable) {
		t.Errorf("error = %v, want ErrPoolerStatsUnavailable", err)
	}
	if _, err := ReadPoolerPools(context.Background(), ex, "c1", 6432, ""); !errors.Is(err, ErrPoolerStatsUnavailable) {
		t.Errorf("empty password error = %v, want ErrPoolerStatsUnavailable", err)
	}
}
