package agent

// TestLive_ExecTTY is the remote half of internal/docker's own PTY live
// tests: the same interactive shell, resize, and teardown behavior, but
// reached through a real enrollment, a real mTLS gRPC session, and the
// real exec relay, so a terminal on a remote node is proven identical to
// a local one rather than assumed to be.

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"

	dockerclient "github.com/docker/docker/client"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/store"
)

// readUntilTimeout bounds every readUntilRemote in this file.
const readUntilTimeout = 20 * time.Second

// liveRemoteTransport stands up the whole agent stack against a real
// Docker daemon and returns a Transport reaching it, plus the ID of a
// running container to exec into.
func liveRemoteTransport(t *testing.T, containerName string) (Transport, string) {
	t.Helper()

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

// readUntilRemote reads from r until want appears or readUntilTimeout
// passes, returning everything read either way.
func readUntilRemote(t *testing.T, r io.Reader, want string) string {
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

	if _, err := io.WriteString(sess, "tty -s && echo REMOTE_HAS_TTY\n"); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	if got := readUntilRemote(t, sess, "REMOTE_HAS_TTY"); !strings.Contains(got, "REMOTE_HAS_TTY") {
		t.Fatalf("remote terminal output = %q, want it to contain REMOTE_HAS_TTY", got)
	}

	// The initial size crossed the wire in the exec request itself.
	if _, err := io.WriteString(sess, "stty size\n"); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	if got := readUntilRemote(t, sess, "24 80"); !strings.Contains(got, "24 80") {
		t.Fatalf("remote stty size = %q, want it to contain \"24 80\"", got)
	}

	// And a resize frame changes the real PTY on the far side.
	if err := sess.Resize(ctx, docker.TTYSize{Rows: 50, Cols: 120}); err != nil {
		t.Fatalf("Resize() error = %v", err)
	}
	if _, err := io.WriteString(sess, "stty size\n"); err != nil {
		t.Fatalf("Write() after resize error = %v", err)
	}
	if got := readUntilRemote(t, sess, "50 120"); !strings.Contains(got, "50 120") {
		t.Fatalf("remote stty size after Resize = %q, want it to contain \"50 120\"", got)
	}
}

// TestLive_ExecTTY_RemoteCloseStopsProcess proves closing a remote
// terminal ends the shell on the node rather than leaking it there,
// which is the failure mode a browser tab closing would otherwise cause
// once per session.
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
	if _, err := io.WriteString(sess, "echo REMOTE_READY\n"); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	if got := readUntilRemote(t, sess, "REMOTE_READY"); !strings.Contains(got, "REMOTE_READY") {
		t.Fatalf("remote terminal output = %q, want it to contain REMOTE_READY", got)
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
		if !strings.Contains(string(out), "sh\n") {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("the remote shell is still running after Close(): %q", out)
		}
		time.Sleep(250 * time.Millisecond)
	}
}
