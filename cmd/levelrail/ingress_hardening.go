package main

import (
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/api"
	ingressdriver "github.com/GLINCKER/levelrail/internal/ingress"
	ingressreconcile "github.com/GLINCKER/levelrail/internal/reconcile/ingress"
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
func loadIngressEdge(logger *slog.Logger) (ingressEdge, error) {
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
	edge.hardening = &h
	return edge, nil
}

func (e ingressEdge) options() []ingressreconcile.Option {
	opts := []ingressreconcile.Option{ingressreconcile.WithHoldTracker(e.holds)}
	if e.fallbackLifetime > 0 {
		opts = append(opts, ingressreconcile.WithFallbackCertLifetime(e.fallbackLifetime))
	}
	if e.inherited.Active() {
		opts = append(opts, ingressreconcile.WithInheritedSockets(e.inherited))
	}
	if e.hardening != nil {
		opts = append(opts, ingressreconcile.WithHardening(*e.hardening))
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
