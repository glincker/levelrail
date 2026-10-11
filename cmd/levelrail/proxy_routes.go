package main

import (
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/brand"
	"github.com/GLINCKER/levelrail/internal/proxyroutes"
	"github.com/GLINCKER/levelrail/internal/store"
)

const envProxyVerifyTimeout = "APP_PROXY_VERIFY_TIMEOUT"

// newProxySyncer builds the one syncer the reconciler and the API share.
func newProxySyncer(b *brand.Brand, db *store.DB, logger *slog.Logger) *proxyroutes.Syncer {
	return &proxyroutes.Syncer{
		Store:     db,
		NS:        proxyroutes.Namespace(strings.ToLower(b.BinaryName)),
		Ingress:   proxyroutes.Listener{Label: "ingress", Addr: ingressHTTPAddr(), EnvVar: "APP_INGRESS_HTTP_ADDR"},
		Dashboard: proxyroutes.Listener{Label: "dashboard", Addr: httpAddr(), EnvVar: "APP_HTTP_ADDR"},
		Unit:      b.BinaryName + ".service",
		Logger:    logger,
	}
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
