package main

import (
	"bytes"
	"context"
	"io"
	"net"
	"testing"
	"time"
)

func TestServeTunnelRelaysBytes(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	dial := func(context.Context) (net.Conn, error) {
		a, b := net.Pipe()
		go func() {
			defer func() { _ = b.Close() }()
			_, _ = io.Copy(b, b)
		}()
		return a, nil
	}
	done := make(chan struct{})
	go func() {
		serveTunnel(ctx, ln, dial, io.Discard)
		close(done)
	}()

	conn, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }()
	if _, err := conn.Write([]byte("ping")); err != nil {
		t.Fatal(err)
	}
	got := make([]byte, 4)
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	if _, err := io.ReadFull(conn, got); err != nil {
		t.Fatalf("read echo: %v", err)
	}
	if !bytes.Equal(got, []byte("ping")) {
		t.Fatalf("echo = %q, want ping", got)
	}
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("serveTunnel did not stop after cancel")
	}
}
