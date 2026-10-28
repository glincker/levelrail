package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"os/signal"
	"strconv"
	"syscall"
)

// runAppsTunnel opens a loopback-only local port that relays to a declared port of a private app container.
func runAppsTunnel(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	var service string
	var port, localPort int
	c, code, ok := parseDBAccess(prog, "apps tunnel", "apps tunnel <app> [--service s] [--port p] [--local-port n] [flags]", args, 1, "an app name", stdout, stderr, lookupEnv, func(fs *flag.FlagSet) {
		fs.StringVar(&service, "service", "", "service of a multi-service app (default: the app itself)")
		fs.IntVar(&port, "port", 0, "container port to reach (default: the app's routed port)")
		fs.IntVar(&localPort, "local-port", 0, "local port to listen on (default: a free port)")
	})
	if !ok {
		return code
	}
	ln, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(localPort)))
	if err != nil {
		return c.fail(fmt.Errorf("listen on 127.0.0.1:%d: %w", localPort, err))
	}
	defer func() { _ = ln.Close() }()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	name := c.rest[0]
	_, _ = fmt.Fprintf(stdout, "tunnel to app %q listening on %s (Ctrl+C to stop)\n", name, ln.Addr())
	serveTunnel(ctx, ln, func(dctx context.Context) (net.Conn, error) {
		return c.client.DialAppTunnel(dctx, name, service, port)
	}, stderr)
	return exitOK
}
