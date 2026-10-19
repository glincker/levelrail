package main

import (
	"context"
	"log/slog"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/alerting"
	"github.com/GLINCKER/levelrail/internal/api"
	"github.com/GLINCKER/levelrail/internal/docker"
	ingressdriver "github.com/GLINCKER/levelrail/internal/ingress"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/telemetry"
)

// bootstrapAdmin creates the first admin from APP_ADMIN_USERNAME and
// APP_ADMIN_PASSWORD when no user exists yet. Without a password it does
// nothing and the setup-token flow (ensureSetupToken) takes over.
func bootstrapAdmin(ctx context.Context, db *store.DB) error {
	username := os.Getenv("APP_ADMIN_USERNAME")
	if username == "" {
		username = "admin"
	}
	password := os.Getenv("APP_ADMIN_PASSWORD")
	if password == "" {
		return nil
	}
	return api.BootstrapAdmin(ctx, db, username, password)
}

func publicHostSource() string {
	_, source := resolvedPublicHost()
	return source
}

// httpRedirectEnabled reads APP_INGRESS_HTTP_REDIRECT (true, false, or auto,
// the default). Auto enables the redirect only when the HTTP address can be
// bound right now, so a non-root dev run keeps working without port 80.
func httpRedirectEnabled(addr string, inherited *ingressdriver.InheritedSockets, logger *slog.Logger) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("APP_INGRESS_HTTP_REDIRECT"))) {
	case "1", "true", "on", "yes":
		return true
	case "0", "false", "off", "no":
		return false
	}
	if inherited.Active() && inherited.HTTP != 0 {
		return true
	}
	ln, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", addr)
	if err != nil {
		logger.Warn("http to https redirect disabled: cannot bind the HTTP ingress address", slog.String("addr", addr), slog.String("error", err.Error()))
		return false
	}
	_ = ln.Close()
	return true
}

// acmeDefaultDirectory returns the CA directory enable-https uses: Let's
// Encrypt staging when APP_ACME_STAGING is set, APP_ACME_DIRECTORY_URL when
// given, else "" for Let's Encrypt production.
func acmeDefaultDirectory() string {
	if v, err := strconv.ParseBool(strings.TrimSpace(os.Getenv("APP_ACME_STAGING"))); err == nil && v {
		return api.ACMEStagingDirectoryURL
	}
	return strings.TrimSpace(os.Getenv("APP_ACME_DIRECTORY_URL"))
}

// acmePendingTimeout reads APP_ACME_PENDING_TIMEOUT; 0 keeps the default.
func acmePendingTimeout(logger *slog.Logger) time.Duration {
	raw := os.Getenv("APP_ACME_PENDING_TIMEOUT")
	if raw == "" {
		return 0
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		logger.Warn("invalid APP_ACME_PENDING_TIMEOUT, using the default", slog.String("value", raw))
		return 0
	}
	return d
}

// purgeStaleIssuerCerts drops stored certificates from any CA other than the
// configured ACME one, so a leftover staging or internal certificate is never
// reused in place of a real one. No-op while ACME is disabled.
func purgeStaleIssuerCerts(ctx context.Context, db *store.DB, logger *slog.Logger) {
	settings, err := db.GetIngressSettings(ctx)
	if err != nil || !settings.ACMEEnabled {
		return
	}
	n, err := ingressdriver.PurgeCertsFromOtherIssuersOnce(ctx, db, settings.ACMEDirectoryURL)
	if err != nil {
		logger.Warn("purging certificates from other issuers failed", slog.String("error", err.Error()))
		return
	}
	if n > 0 {
		logger.Info("dropped certificates from other issuers so they re-issue", slog.Int("keys", n))
	}
}

// wakeLogCollectorOnContainerEvents makes the log collector re-derive
// targets on container start/die so containers shorter-lived than the
// resync tick still get their output captured.
func wakeLogCollectorOnContainerEvents(ctx context.Context, src alerting.EventSource, lc *telemetry.LogCollector) {
	events, errs := src.Events(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case ev, ok := <-events:
			if !ok {
				return
			}
			if ev.Action == docker.EventStart || ev.Action == docker.EventDie {
				lc.Wake()
			}
		case _, ok := <-errs:
			if !ok {
				errs = nil
			}
		}
	}
}
