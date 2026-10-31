package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/GLINCKER/levelrail/internal/plugin/egress"
)

const defaultEgressListen = "0.0.0.0:3128"

// runPluginEgress serves the egress proxy and its policy socket until ctx ends.
// With no policy it denies everything, so a missing cache fails closed.
func runPluginEgress(ctx context.Context, logger *slog.Logger, lookup func(string) (string, bool)) error {
	get := func(k string) string {
		v, _ := lookup(k)
		return strings.TrimSpace(v)
	}
	socket := get(egress.EnvPolicySocket)
	if socket == "" {
		return fmt.Errorf("%s is required", egress.EnvPolicySocket)
	}
	listen := get(egress.EnvListen)
	if listen == "" {
		listen = defaultEgressListen
	}

	store := egress.NewPolicyStore()
	ps := egress.NewPolicyServer(store, get(egress.EnvPolicyCache))
	if err := ps.LoadCache(); err != nil {
		logger.Warn("plugin egress policy cache ignored", slog.String("error", err.Error()))
	}

	policyLn, err := egress.ListenPolicySocket(socket)
	if err != nil {
		return err
	}
	var lc net.ListenConfig
	proxyLn, err := lc.Listen(ctx, "tcp", listen)
	if err != nil {
		_ = policyLn.Close()
		return fmt.Errorf("listen proxy: %w", err)
	}

	proxy := egress.New(egress.Options{
		Credentials: store,
		Allowlist:   store,
		Config:      egress.ConfigFromEnv(lookup),
		OnDeny: func(plugin, host string, port int, reason string) {
			logger.Warn("plugin egress denied",
				slog.String("plugin", plugin), slog.String("host", host),
				slog.Int("port", port), slog.String("reason", reason))
		},
	})
	proxySrv := &http.Server{Handler: proxy, ReadHeaderTimeout: 10 * time.Second}
	policySrv := &http.Server{Handler: ps.Handler(), ReadHeaderTimeout: 10 * time.Second}

	errc := make(chan error, 2)
	go func() { errc <- proxySrv.Serve(proxyLn) }()
	go func() { errc <- policySrv.Serve(policyLn) }()
	logger.Info("plugin egress started", slog.String("listen", listen), slog.Bool("ready", store.Ready()))

	var serveErr error
	select {
	case <-ctx.Done():
	case serveErr = <-errc:
	}
	shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = proxySrv.Shutdown(shutCtx)
	_ = policySrv.Shutdown(shutCtx)
	if serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
		return serveErr
	}
	return nil
}

// runPluginEgressSignals stops the sidecar on SIGTERM or interrupt.
func runPluginEgressSignals(logger *slog.Logger) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return runPluginEgress(ctx, logger, os.LookupEnv)
}
