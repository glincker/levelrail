package api

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/dockertest"
	"github.com/GLINCKER/levelrail/internal/reconcile/application"
	"github.com/GLINCKER/levelrail/internal/store"
)

// TestHandleAppTerminal_Live proves the whole browser-facing path
// against a real daemon: a real WebSocket upgrade, a real PTY in a real
// container, and a resize the shell inside actually observes.
func TestHandleAppTerminal_Live(t *testing.T) {
	dockertest.SkipIfShort(t)
	client, err := docker.NewClient()
	if err != nil {
		t.Skipf("no docker client available: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })

	ctx := context.Background()
	const image = "nginx:alpine"
	svc := store.DesiredService{Name: "web", Image: image, Port: 80}
	containerName := application.ContainerName(svc.Name, svc.Image, svc.RestartNonce)

	state, err := client.InspectByName(ctx, containerName)
	if err != nil {
		t.Skipf("docker daemon not reachable: %v", err)
	}
	if state != nil {
		_ = client.Remove(ctx, state.ID, true)
	}
	id, err := client.Create(ctx, docker.ContainerSpec{Name: containerName, Image: image})
	if err != nil {
		t.Skipf("could not create the test container: %v", err)
	}
	t.Cleanup(func() { _ = client.Remove(context.Background(), id, true) })
	if err := client.Start(ctx, id); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	db := openTestDB(t)
	rt := NewRouter(discardLogger(), testBrand(), db, WithExecRuntime(func(string) (docker.Runtime, error) { return client, nil }))
	cookie := loginTestSession(t, rt, db)
	if err := db.SaveDesiredService(ctx, svc); err != nil {
		t.Fatalf("seed app: %v", err)
	}

	conn := dialTestTerminal(t, rt, cookie, "?rows=30&cols=100")
	readCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	if err := conn.Write(readCtx, websocket.MessageBinary, []byte("tty -s && stty size\n")); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	if got := readTerminalUntil(readCtx, t, conn, "30 100"); !strings.Contains(got, "30 100") {
		t.Fatalf("terminal output = %q, want the PTY to report 30 rows by 100 cols", got)
	}

	resize, err := json.Marshal(terminalControl{Type: "resize", Rows: 44, Cols: 111})
	if err != nil {
		t.Fatalf("marshal resize: %v", err)
	}
	if err := conn.Write(readCtx, websocket.MessageText, resize); err != nil {
		t.Fatalf("Write(resize) error = %v", err)
	}
	if err := conn.Write(readCtx, websocket.MessageBinary, []byte("stty size\n")); err != nil {
		t.Fatalf("Write() after resize error = %v", err)
	}
	if got := readTerminalUntil(readCtx, t, conn, "44 111"); !strings.Contains(got, "44 111") {
		t.Fatalf("terminal output after resize = %q, want 44 rows by 111 cols", got)
	}
}

// readTerminalUntil accumulates binary frames until want appears or ctx
// expires, returning everything seen either way.
func readTerminalUntil(ctx context.Context, t *testing.T, conn *websocket.Conn, want string) string {
	t.Helper()
	var sb strings.Builder
	for {
		kind, data, err := conn.Read(ctx)
		if err != nil {
			return sb.String()
		}
		if kind != websocket.MessageBinary {
			continue
		}
		sb.Write(data)
		if strings.Contains(sb.String(), want) {
			return sb.String()
		}
	}
}
