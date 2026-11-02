package main

import (
	"bytes"
	"context"
	"log/slog"
	"net"
	"strconv"
	"testing"
)

func TestHTTPRedirectEnabledOwnPortIsNotAWarning(t *testing.T) {
	t.Setenv("APP_INGRESS_HTTP_REDIRECT", "auto")
	ln, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ln.Close() }()
	addr := ln.Addr().String()
	port := ln.Addr().(*net.TCPAddr).Port

	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))

	owns := func(p int) bool { return p == port }
	if !httpRedirectEnabled(addr, nil, owns, logger) {
		t.Fatalf("redirect should stay on when this process holds %s", strconv.Itoa(port))
	}
	if buf.Len() != 0 {
		t.Fatalf("unexpected log: %s", buf.String())
	}

	if httpRedirectEnabled(addr, nil, func(int) bool { return false }, logger) {
		t.Fatal("a port held by someone else must disable the redirect")
	}
	if buf.Len() == 0 {
		t.Fatal("expected a warning for a foreign holder")
	}
}
