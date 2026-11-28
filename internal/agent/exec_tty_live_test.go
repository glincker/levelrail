package agent

// TestLive_ExecTTY runs the PTY shell checks through a real enrollment, mTLS session, and exec relay.

import (
	"context"
	"fmt"
	"io"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	dockerclient "github.com/docker/docker/client"

	"github.com/glincker/levelrail/internal/docker"
	"github.com/glincker/levelrail/internal/dockertest"
	"github.com/glincker/levelrail/internal/store"
)

// readUntilTimeout bounds every readUntilRemote in this file.
const readUntilTimeout = 20 * time.Second

// Unique container name suffix so concurrent test processes on one daemon never collide.
func liveRemoteTransport(t *testing.T, containerNamePrefix string) (Transport, string) {
	t.Helper()
	dockertest.SkipIfShort(t)

	containerName := fmt.Sprintf("%s-%d", containerNamePrefix, time.Now().UnixNano())

	rt, err := docker.NewClient()
	if err != nil {
		t.Skipf("no docker client available: %v", err)
	}
	rawCli, err := dockerclient.NewClientWithOpts(dockerclient.FromEnv, dockerclient.WithAPIVersionNegotiation())
	if err != nil {
		t.Skipf("no docker client available: %v", err)
	}
	pingCtx, cancelPing := context.WithTimeout(context.Background(), 3*time.Second)
	_, err = rawCli.Ping(pingCtx)
	cancelPing()
	if err != nil {
		t.Skipf("docker daemon not reachable: %v", err)
	}
	t.Cleanup(func() { _ = rt.Close() })

	ctx := context.Background()
	if err := pullIfMissing(ctx, t, rawCli, "nginx:alpine"); err != nil {
		t.Fatalf("pull nginx:alpine: %v", err)
	}
	cleanupContainer(ctx, t, rt, containerName)
	t.Cleanup(func() { cleanupContainer(context.Background(), t, rt, containerName) })

	id, err := rt.Create(ctx, docker.ContainerSpec{Name: containerName, Image: "nginx:alpine"})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if err := rt.Start(ctx, id); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	db := openLiveTestStore(t)
	ca, err := GenerateCA()
	if err != nil {
		t.Fatalf("GenerateCA() error = %v", err)
	}
	registry := NewRegistry()
	listener, grpcServer := startTestAgentServer(t, ca, db, registry)
	t.Cleanup(grpcServer.GracefulStop)
	addr := listener.Addr().String()

	plaintext := "live-tty-join-token"
	now := time.Now()
	if err := db.SaveNodeJoinToken(ctx, store.NodeJoinToken{
		ID: "njt_live_tty", TokenHash: hashJoinToken(plaintext), CreatedAt: now, ExpiresAt: now.Add(time.Hour),
	}); err != nil {
		t.Fatalf("SaveNodeJoinToken() error = %v", err)
	}

	enrollCtx, cancelEnroll := context.WithTimeout(ctx, 10*time.Second)
	identity, err := DialEnroll(enrollCtx, addr, plaintext, containerName+"-node")
	cancelEnroll()
	if err != nil {
		t.Fatalf("DialEnroll() error = %v", err)
	}

	sessionCtx, cancelSession := context.WithCancel(ctx)
	t.Cleanup(cancelSession)
	go func() { _ = RunSession(sessionCtx, addr, identity, rt, nil) }()

	return waitForTransport(t, registry, identity.NodeID), id
}

// ttyReader owns the single reader of a session so no output is lost or
// raced between waits; the goroutine ends when the session closes.
type ttyReader struct {
	mu     sync.Mutex
	buf    strings.Builder
	cursor int
	wake   chan struct{}
}

func newTTYReader(r io.Reader) *ttyReader {
	tr := &ttyReader{wake: make(chan struct{}, 1)}
	go func() {
		chunk := make([]byte, 4096)
		for {
			n, err := r.Read(chunk)
			if n > 0 {
				tr.mu.Lock()
				tr.buf.Write(chunk[:n])
				tr.mu.Unlock()
				select {
				case tr.wake <- struct{}{}:
				default:
				}
			}
			if err != nil {
				return
			}
		}
	}()
	return tr
}

// unread returns output not yet consumed by a wait, and whether want is in it.
func (tr *ttyReader) unread(want string) (string, bool) {
	tr.mu.Lock()
	defer tr.mu.Unlock()
	out := tr.buf.String()[tr.cursor:]
	if strings.Contains(out, want) {
		tr.cursor += len(out)
		return out, true
	}
	return out, false
}

// readUntilRemote waits until want appears in new output or readUntilTimeout
// passes, returning the output seen in that wait either way.
func readUntilRemote(t *testing.T, tr *ttyReader, want string) string {
	t.Helper()
	deadline := time.After(readUntilTimeout)
	for {
		if out, ok := tr.unread(want); ok {
			return out
		}
		select {
		case <-tr.wake:
		case <-deadline:
			out, _ := tr.unread(want)
			return out
		}
	}
}

func TestLive_ExecTTY_RemoteInteractiveShell(t *testing.T) {
	transport, containerID := liveRemoteTransport(t, "levelrail-test-agent-live-tty")

	tty, ok := transport.(docker.TTYRuntime)
	if !ok {
		t.Fatal("the remote transport does not implement docker.TTYRuntime")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	sess, err := tty.ExecTTY(ctx, containerID, docker.ExecTTYOptions{
		Cmd:  []string{"sh"},
		Env:  []string{"TERM=xterm-256color"},
		Size: docker.TTYSize{Rows: 24, Cols: 80},
	})
	if err != nil {
		t.Fatalf("ExecTTY() error = %v", err)
	}
	defer func() { _ = sess.Close() }()
	tr := newTTYReader(sess)

	if _, err := io.WriteString(sess, "tty -s && echo REMOTE_HAS_TTY\n"); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	if got := readUntilRemote(t, tr, "REMOTE_HAS_TTY"); !strings.Contains(got, "REMOTE_HAS_TTY") {
		t.Fatalf("remote terminal output = %q, want it to contain REMOTE_HAS_TTY", got)
	}

	// The initial size crossed the wire in the exec request itself.
	if _, err := io.WriteString(sess, "stty size\n"); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	if got := readUntilRemote(t, tr, "24 80"); !strings.Contains(got, "24 80") {
		t.Fatalf("remote stty size = %q, want it to contain \"24 80\"", got)
	}

	// The node applies a resize asynchronously to the input stream, so ask until the new size shows.
	if err := sess.Resize(ctx, docker.TTYSize{Rows: 50, Cols: 120}); err != nil {
		t.Fatalf("Resize() error = %v", err)
	}
	var got string
	resizeDeadline := time.Now().Add(readUntilTimeout)
	for {
		if _, err := io.WriteString(sess, "echo SZ:$(stty size):Z\"\"S\n"); err != nil {
			t.Fatalf("Write() after resize error = %v", err)
		}
		got = readUntilRemote(t, tr, ":ZS")
		if strings.Contains(got, "SZ:50 120:") || time.Now().After(resizeDeadline) {
			break
		}
	}
	if !strings.Contains(got, "SZ:50 120:") {
		t.Fatalf("remote stty size after Resize = %q, want it to contain \"50 120\"", got)
	}
}

// TestLive_ExecTTY_RemoteCloseStopsProcess checks a closed remote terminal ends the node's shell.
// shellLine matches only the exec'd shell, not nginx's docker-entrypoint.sh.
var shellLine = regexp.MustCompile(`(?m)^sh$`)

func TestLive_ExecTTY_RemoteCloseStopsProcess(t *testing.T) {
	transport, containerID := liveRemoteTransport(t, "levelrail-test-agent-live-tty-close")

	tty, ok := transport.(docker.TTYRuntime)
	if !ok {
		t.Fatal("the remote transport does not implement docker.TTYRuntime")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	sess, err := tty.ExecTTY(ctx, containerID, docker.ExecTTYOptions{
		Cmd:  []string{"sh"},
		Size: docker.TTYSize{Rows: 24, Cols: 80},
	})
	if err != nil {
		t.Fatalf("ExecTTY() error = %v", err)
	}
	tr := newTTYReader(sess)
	if _, err := io.WriteString(sess, "echo REMOTE_READY\n"); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	// Close sends Ctrl-C, Ctrl-D: wait for the prompt so the shell is reading, or the terminal drops the EOF.
	const readyPrompt = "REMOTE_READY\r\n/ # "
	if got := readUntilRemote(t, tr, readyPrompt); !strings.Contains(got, readyPrompt) {
		t.Fatalf("remote terminal output = %q, want it to contain REMOTE_READY and a prompt", got)
	}
	if err := sess.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	deadline := time.Now().Add(20 * time.Second)
	for {
		rc, err := transport.Exec(ctx, containerID, []string{"ps", "-o", "args"})
		if err != nil {
			t.Fatalf("Exec(ps) error = %v", err)
		}
		out, _ := io.ReadAll(rc)
		_ = rc.Close()
		if !shellLine.MatchString(string(out)) {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("the remote shell is still running after Close(): %q\n%s", out, closeDiagnostics(ctx, transport, containerID))
		}
		time.Sleep(250 * time.Millisecond)
	}
}

// closeDiagnostics reports the full process table and the state of every
// exec in the container, so a CI failure shows why the shell outlived Close.
func closeDiagnostics(ctx context.Context, transport Transport, containerID string) string {
	var sb strings.Builder
	if rc, err := transport.Exec(ctx, containerID, []string{"ps", "-o", "pid,stat,args"}); err != nil {
		fmt.Fprintf(&sb, "ps pid,stat,args: error %v\n", err)
	} else {
		out, _ := io.ReadAll(rc)
		_ = rc.Close()
		fmt.Fprintf(&sb, "ps pid,stat,args:\n%s\n", out)
	}
	cli, err := dockerclient.NewClientWithOpts(dockerclient.FromEnv, dockerclient.WithAPIVersionNegotiation())
	if err != nil {
		fmt.Fprintf(&sb, "exec inspect: no docker client: %v\n", err)
		return sb.String()
	}
	defer func() { _ = cli.Close() }()
	info, err := cli.ContainerInspect(ctx, containerID)
	if err != nil {
		fmt.Fprintf(&sb, "container inspect: %v\n", err)
		return sb.String()
	}
	for _, id := range info.ExecIDs {
		ex, err := cli.ContainerExecInspect(ctx, id)
		if err != nil {
			fmt.Fprintf(&sb, "exec %s inspect: %v\n", id, err)
			continue
		}
		fmt.Fprintf(&sb, "exec %s running=%v pid=%d exit=%d\n", id, ex.Running, ex.Pid, ex.ExitCode)
	}
	return sb.String()
}
