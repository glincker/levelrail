package main

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/plugin/egress"
)

func TestPluginEgressSidecarRunsAndStops(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	if err := runPluginEgress(context.Background(), logger, func(string) (string, bool) { return "", false }); err == nil {
		t.Fatal("missing socket env must fail")
	}

	dir, err := os.MkdirTemp("/tmp", "egr")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	sock := filepath.Join(dir, "p.sock")
	env := map[string]string{
		egress.EnvPolicySocket: sock,
		egress.EnvPolicyCache:  filepath.Join(dir, "cache.json"),
		egress.EnvListen:       "127.0.0.1:0",
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- runPluginEgress(ctx, logger, func(k string) (string, bool) { v, ok := env[k]; return v, ok })
	}()

	p := egress.Policy{Version: 1, Plugins: []egress.PluginPolicy{{Name: "a", SecretHash: egress.HexHash(egress.HashSecret("s"))}}}
	var pushErr error
	for i := 0; i < 100; i++ {
		if pushErr = egress.PushPolicy(ctx, sock, p); pushErr == nil {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if pushErr != nil {
		t.Fatal(pushErr)
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("sidecar did not stop")
	}
}
