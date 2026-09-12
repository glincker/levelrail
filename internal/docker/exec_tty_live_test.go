package docker

import (
	"bufio"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"
)

// readUntilTimeout bounds every readUntil in this file.
const readUntilTimeout = 15 * time.Second

// ttyTestContainer starts a long-lived container to exec into and
// returns its ID.
func ttyTestContainer(t *testing.T, c *Client, name string) string {
	t.Helper()
	ctx := context.Background()
	removeIfExists(ctx, t, c, name)
	t.Cleanup(func() { removeIfExists(context.Background(), t, c, name) })

	id, err := c.Create(ctx, ContainerSpec{Name: name, Image: "nginx:alpine"})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if err := c.Start(ctx, id); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	return id
}

// readUntil reads from r until want appears or readUntilTimeout passes,
// returning everything read so far either way.
func readUntil(t *testing.T, r io.Reader, want string) string {
	t.Helper()
	var sb strings.Builder
	found := make(chan struct{})
	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := r.Read(buf)
			if n > 0 {
				sb.WriteString(string(buf[:n]))
				if strings.Contains(sb.String(), want) {
					close(found)
					return
				}
			}
			if err != nil {
				return
			}
		}
	}()
	select {
	case <-found:
	case <-time.After(readUntilTimeout):
	}
	return sb.String()
}

// TestClient_ExecTTY_Live_InteractiveShell proves the PTY is real end to
// end: a shell stays alive between commands, sees a terminal (tty -s
// succeeds, which it cannot without a PTY), and exits on its own when
// told to.
func TestClient_ExecTTY_Live_InteractiveShell(t *testing.T) {
	c := liveClient(t)
	id := ttyTestContainer(t, c, "levelrail-test-docker-exec-tty")

	sess, err := c.ExecTTY(context.Background(), id, ExecTTYOptions{
		Cmd:  []string{"sh"},
		Env:  []string{"TERM=xterm-256color"},
		Size: TTYSize{Rows: 24, Cols: 80},
	})
	if err != nil {
		t.Fatalf("ExecTTY() error = %v", err)
	}
	defer func() { _ = sess.Close() }()

	if _, err := io.WriteString(sess, "tty -s && echo HAS_TTY\n"); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	if got := readUntil(t, sess, "HAS_TTY"); !strings.Contains(got, "HAS_TTY") {
		t.Fatalf("interactive shell output = %q, want it to contain HAS_TTY (the exec has no PTY otherwise)", got)
	}

	// The same session again, which is the whole point of a persistent
	// shell: the one-shot Exec endpoint would need a second exec here.
	if _, err := io.WriteString(sess, "echo SECOND_$((20+2))\n"); err != nil {
		t.Fatalf("second Write() error = %v", err)
	}
	if got := readUntil(t, sess, "SECOND_22"); !strings.Contains(got, "SECOND_22") {
		t.Fatalf("second command output = %q, want it to contain SECOND_22", got)
	}
}

// TestClient_ExecTTY_Live_Resize proves Resize reaches the real PTY: the
// shell's own $LINES/$COLUMNS, which it reads from the terminal, change
// to the requested size.
func TestClient_ExecTTY_Live_Resize(t *testing.T) {
	c := liveClient(t)
	id := ttyTestContainer(t, c, "levelrail-test-docker-exec-tty-resize")

	sess, err := c.ExecTTY(context.Background(), id, ExecTTYOptions{
		Cmd:  []string{"sh"},
		Env:  []string{"TERM=xterm"},
		Size: TTYSize{Rows: 24, Cols: 80},
	})
	if err != nil {
		t.Fatalf("ExecTTY() error = %v", err)
	}
	defer func() { _ = sess.Close() }()

	if _, err := io.WriteString(sess, "stty size\n"); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	if got := readUntil(t, sess, "24 80"); !strings.Contains(got, "24 80") {
		t.Fatalf("initial stty size output = %q, want it to contain \"24 80\"", got)
	}

	if err := sess.Resize(context.Background(), TTYSize{Rows: 40, Cols: 132}); err != nil {
		t.Fatalf("Resize() error = %v", err)
	}
	if _, err := io.WriteString(sess, "stty size\n"); err != nil {
		t.Fatalf("Write() after resize error = %v", err)
	}
	if got := readUntil(t, sess, "40 132"); !strings.Contains(got, "40 132") {
		t.Fatalf("stty size after Resize = %q, want it to contain \"40 132\"", got)
	}
}

// TestClient_ExecTTY_Live_ExitCode proves a session that ends by the
// command exiting non-zero surfaces that exit code as the stream's
// trailing error, the same contract Exec already has.
func TestClient_ExecTTY_Live_ExitCode(t *testing.T) {
	c := liveClient(t)
	id := ttyTestContainer(t, c, "levelrail-test-docker-exec-tty-exit")

	sess, err := c.ExecTTY(context.Background(), id, ExecTTYOptions{
		Cmd:  []string{"sh", "-c", "exit 7"},
		Size: TTYSize{Rows: 24, Cols: 80},
	})
	if err != nil {
		t.Fatalf("ExecTTY() error = %v", err)
	}
	defer func() { _ = sess.Close() }()

	_, readErr := io.Copy(io.Discard, bufio.NewReader(sess))
	var exitErr *ExecExitError
	if !errors.As(readErr, &exitErr) {
		t.Fatalf("read after exit 7 error = %v, want *ExecExitError", readErr)
	}
	if exitErr.ExitCode != 7 {
		t.Errorf("ExitCode = %d, want 7", exitErr.ExitCode)
	}
}

// TestClient_ExecTTY_Live_CloseStopsProcess proves Close actually ends
// the remote shell rather than leaking it: the shell's own `sh` process
// is gone from the container's process table afterwards.
func TestClient_ExecTTY_Live_CloseStopsProcess(t *testing.T) {
	c := liveClient(t)
	id := ttyTestContainer(t, c, "levelrail-test-docker-exec-tty-close")

	sess, err := c.ExecTTY(context.Background(), id, ExecTTYOptions{
		Cmd:  []string{"sh"},
		Size: TTYSize{Rows: 24, Cols: 80},
	})
	if err != nil {
		t.Fatalf("ExecTTY() error = %v", err)
	}
	if _, err := io.WriteString(sess, "echo READY\n"); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	if got := readUntil(t, sess, "READY"); !strings.Contains(got, "READY") {
		t.Fatalf("shell output = %q, want it to contain READY", got)
	}
	if err := sess.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	deadline := time.Now().Add(15 * time.Second)
	for {
		rc, err := c.Exec(context.Background(), id, []string{"ps", "-o", "args"})
		if err != nil {
			t.Fatalf("Exec(ps) error = %v", err)
		}
		out, _ := io.ReadAll(rc)
		_ = rc.Close()
		if !strings.Contains(string(out), "sh\n") && !strings.HasSuffix(strings.TrimSpace(string(out)), "sh") {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("the exec'd shell is still running after Close(): %q", out)
		}
		time.Sleep(200 * time.Millisecond)
	}
}
