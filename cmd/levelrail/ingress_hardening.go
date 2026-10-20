package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/api"
	"github.com/GLINCKER/levelrail/internal/brand"
	ingressdriver "github.com/GLINCKER/levelrail/internal/ingress"
	ingressreconcile "github.com/GLINCKER/levelrail/internal/reconcile/ingress"
	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	envIngressHoldWindow  = "APP_INGRESS_HOLD_WINDOW"
	defaultIngressHold    = 10 * time.Minute
	envIngressHardeningOn = "APP_INGRESS_HARDENING"

	envFallbackCertLifetime     = "APP_INGRESS_FALLBACK_CERT_LIFETIME"
	defaultFallbackCertLifetime = 6 * time.Hour
)

// ingressEdge is the edge policy and hold tracker shared by every per-pass
// ingress controller.
type ingressEdge struct {
	hardening *ingressdriver.Hardening
	holds     *ingressdriver.HoldTracker
	inherited *ingressdriver.InheritedSockets
	// fallbackLifetime is how long a locally signed fallback certificate lives.
	fallbackLifetime time.Duration
}

// loadIngressEdge reads the APP_INGRESS_* tuning variables. Hardening is on by
// default; APP_INGRESS_HARDENING=false restores the bare Caddy config.
func loadIngressEdge(logger *slog.Logger, b *brand.Brand) (ingressEdge, error) {
	window := defaultIngressHold
	if raw := strings.TrimSpace(os.Getenv(envIngressHoldWindow)); raw != "" {
		d, err := time.ParseDuration(raw)
		if err != nil || d < 0 {
			return ingressEdge{}, fmt.Errorf("%s=%q is not a non-negative duration", envIngressHoldWindow, raw)
		}
		window = d
	}
	edge := ingressEdge{holds: ingressdriver.NewHoldTracker(window), fallbackLifetime: defaultFallbackCertLifetime}
	if raw := strings.TrimSpace(os.Getenv(envFallbackCertLifetime)); raw != "" {
		d, err := time.ParseDuration(raw)
		if err != nil || d < time.Minute {
			return ingressEdge{}, fmt.Errorf("%s=%q is not a duration of at least 1m", envFallbackCertLifetime, raw)
		}
		edge.fallbackLifetime = d
	}
	inherited, err := ingressdriver.InheritSockets(os.Getenv, os.Getpid(), os.Getenv(ingressdriver.EnvSocketActivation))
	if err != nil {
		return ingressEdge{}, err
	}
	ingressdriver.ClearListenEnv()
	if inherited.Active() {
		logger.Info("ingress is using systemd socket activation: listeners survive control plane restarts",
			slog.Int("https_port", inherited.HTTPSPort), slog.Int("http_port", inherited.HTTPPort))
		edge.inherited = inherited
	}
	if strings.EqualFold(strings.TrimSpace(os.Getenv(envIngressHardeningOn)), "false") {
		logger.Warn("ingress hardening disabled by " + envIngressHardeningOn)
		return edge, nil
	}
	h, err := ingressdriver.HardeningFromEnv(os.Getenv)
	if err != nil {
		return ingressEdge{}, err
	}
	if inherited.Active() && strings.TrimSpace(os.Getenv(ingressdriver.EnvGracePeriod)) == "" {
		h.GracePeriod = 3 * time.Second
	}
	h.BrandName, h.DocsURL = b.Name, strings.TrimRight(b.DocsURL, "/")+"/domains-and-ingress"
	edge.hardening = &h
	return edge, nil
}

// edgeOverrideStore reads the dashboard-managed edge settings.
type edgeOverrideStore interface {
	GetIngressEdgeSettings(ctx context.Context) (store.IngressEdgeSettings, error)
}

// effectiveHardening layers the current dashboard settings over the boot
// policy each pass, so a saved change applies without a restart.
func (e ingressEdge) effectiveHardening(ctx context.Context, db edgeOverrideStore, logger *slog.Logger) *ingressdriver.Hardening {
	if e.hardening == nil {
		return nil
	}
	h := *e.hardening
	if db == nil {
		return &h
	}
	s, err := db.GetIngressEdgeSettings(ctx)
	if err != nil {
		logger.Warn("ingress edge settings unreadable, using the boot policy", slog.String("error", err.Error()))
		return &h
	}
	merged, err := h.WithOverrides(ingressdriver.EdgeOverrides{
		TrustedProxies: s.TrustedProxies, MaxHeaderBytes: s.MaxHeaderBytes, MaxBodyBytes: s.MaxBodyBytes,
		ReadHeaderTimeoutSecs: s.ReadHeaderTimeoutSecs, IdleTimeoutSecs: s.IdleTimeoutSecs,
	}, os.Getenv)
	if err != nil {
		logger.Warn("ingress edge settings invalid, using the boot policy", slog.String("error", err.Error()))
		return &h
	}
	return &merged
}

func (e ingressEdge) options(ctx context.Context, db edgeOverrideStore, logger *slog.Logger) []ingressreconcile.Option {
	opts := []ingressreconcile.Option{ingressreconcile.WithHoldTracker(e.holds)}
	if e.fallbackLifetime > 0 {
		opts = append(opts, ingressreconcile.WithFallbackCertLifetime(e.fallbackLifetime))
	}
	if e.inherited.Active() {
		opts = append(opts, ingressreconcile.WithInheritedSockets(e.inherited))
	}
	if h := e.effectiveHardening(ctx, db, logger); h != nil {
		opts = append(opts, ingressreconcile.WithHardening(*h))
	}
	return opts
}

func (e ingressEdge) doctorInfo() api.DoctorIngressEdge {
	info := api.DoctorIngressEdge{SocketActivation: e.inherited.Active(), Hardening: e.hardening != nil}
	if e.hardening != nil {
		info.TrustedProxies = len(e.hardening.TrustedProxies)
		info.MaxBodyBytes = e.hardening.MaxBodyBytes
		info.RetryWindow = e.hardening.RetryWindow
	}
	return info
}
