package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"syscall"
)

// runDatabasesTunnel opens a local port that relays to a managed database through the control plane.
func runDatabasesTunnel(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	var port int
	var bind string
	c, code, ok := parseDBAccess(prog, "databases tunnel", "databases tunnel <database> [--port N] [--bind ADDR] [flags]", args, 1, "a database name", stdout, stderr, lookupEnv, func(fs *flag.FlagSet) {
		fs.IntVar(&port, "port", 0, "local port to listen on (default: a free port)")
		fs.StringVar(&bind, "bind", "127.0.0.1", "local address to listen on")
	})
	if !ok {
		return code
	}
	ln, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", net.JoinHostPort(bind, strconv.Itoa(port)))
	if err != nil {
		return c.fail(fmt.Errorf("listen on %s:%d: %w", bind, port, err))
	}
	defer func() { _ = ln.Close() }()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	name := c.rest[0]
	_, _ = fmt.Fprintf(stdout, "tunnel to database %q listening on %s (Ctrl+C to stop)\n", name, ln.Addr())
	serveTunnel(ctx, ln, func(dctx context.Context) (net.Conn, error) {
		return c.client.DialDatabaseTunnel(dctx, name)
	}, stderr)
	return exitOK
}

// serveTunnel accepts local connections until ctx ends and relays each one to a fresh upstream.
func serveTunnel(ctx context.Context, ln net.Listener, dial func(context.Context) (net.Conn, error), stderr io.Writer) {
	go func() {
		<-ctx.Done()
		_ = ln.Close()
	}()
	var wg sync.WaitGroup
	defer wg.Wait()
	for {
		local, err := ln.Accept()
		if err != nil {
			if !errors.Is(err, net.ErrClosed) && ctx.Err() == nil {
				_, _ = fmt.Fprintf(stderr, "tunnel: accept failed: %v\n", err)
			}
			return
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() { _ = local.Close() }()
			upstream, err := dial(ctx)
			if err != nil {
				_, _ = fmt.Fprintf(stderr, "tunnel: %v\n", err)
				return
			}
			defer func() { _ = upstream.Close() }()
			done := make(chan struct{}, 2)
			go func() { _, _ = io.Copy(upstream, local); done <- struct{}{} }()
			go func() { _, _ = io.Copy(local, upstream); done <- struct{}{} }()
			select {
			case <-done:
			case <-ctx.Done():
			}
		}()
	}
}
