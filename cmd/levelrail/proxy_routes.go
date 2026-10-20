package main

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/brand"
	"github.com/GLINCKER/levelrail/internal/docker"
	ingressdriver "github.com/GLINCKER/levelrail/internal/ingress"
	"github.com/GLINCKER/levelrail/internal/proxyroutes"
	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	envProxyVerifyTimeout  = "APP_PROXY_VERIFY_TIMEOUT"
	envProxyRouterPriority = "APP_PROXY_ROUTER_PRIORITY"
)

// newProxySyncer builds the one syncer the reconciler and the API share.
func newProxySyncer(b *brand.Brand, db *store.DB, logger *slog.Logger, ex proxyroutes.Exec) *proxyroutes.Syncer {
	return &proxyroutes.Syncer{
		Exec:           ex,
		Store:          db,
		NS:             proxyroutes.Namespace(strings.ToLower(b.BinaryName)),
		Ingress:        proxyroutes.Listener{Label: "ingress", Addr: ingressHTTPAddr(), EnvVar: "APP_INGRESS_HTTP_ADDR"},
		Dashboard:      proxyroutes.Listener{Label: "dashboard", Addr: httpAddr(), EnvVar: "APP_HTTP_ADDR"},
		Unit:           b.BinaryName + ".service",
		RouterPriority: proxyRouterPriority(logger),
		Logger:         logger,
	}
}

// proxyRouterPriority reads APP_PROXY_ROUTER_PRIORITY; 0 keeps the default.
func proxyRouterPriority(logger *slog.Logger) int {
	raw := os.Getenv(envProxyRouterPriority)
	if raw == "" {
		return 0
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 1 || n > proxyroutes.MaxRouterPriority {
		logger.Warn("invalid "+envProxyRouterPriority+", using the default", slog.String("value", raw))
		return 0
	}
	return n
}

// newRouteCheck mints this process's route check marker. Without one no
// proxied route can be proven, so every proxied control reports not enforced.
func newRouteCheck(logger *slog.Logger) ingressdriver.RouteCheck {
	rc, err := ingressdriver.NewRouteCheck()
	if err != nil {
		logger.Error("proxy route check unavailable", slog.String("error", err.Error()))
		return ingressdriver.RouteCheck{}
	}
	return rc
}

// proxyVerifyTimeout reads APP_PROXY_VERIFY_TIMEOUT; 0 keeps the API default.
func proxyVerifyTimeout(logger *slog.Logger) time.Duration {
	raw := os.Getenv(envProxyVerifyTimeout)
	if raw == "" {
		return 0
	}
	d, err := time.ParseDuration(raw)
	if err != nil || d <= 0 {
		logger.Warn("invalid "+envProxyVerifyTimeout+", using the default", slog.String("value", raw))
		return 0
	}
	return d
}

// proxyContainerExec runs commands in the proxy container through the Docker
// Engine API.
type proxyContainerExec struct{ rt docker.Runtime }

func (e proxyContainerExec) Exec(ctx context.Context, container string, cmd []string) (string, error) {
	r, err := e.rt.Exec(ctx, container, cmd)
	if err != nil {
		return "", err
	}
	defer func() { _ = r.Close() }()
	out, err := io.ReadAll(r)
	var exit *docker.ExecExitError
	if errors.As(err, &exit) {
		msg := exit.Stderr
		if msg == "" {
			msg = strings.TrimSpace(string(out))
		}
		return string(out), &proxyroutes.ExecError{Output: msg, Err: err}
	}
	return string(out), err
}
