package main

import (
	"context"
	"log/slog"
	"os"
	"strconv"
	"time"

	"github.com/GLINCKER/levelrail/internal/api"
	ingressdriver "github.com/GLINCKER/levelrail/internal/ingress"
	"github.com/GLINCKER/levelrail/internal/proxyroutes"
)

// Seconds a running container counts as still starting on the diagnostic page.
const (
	envDiagnosticStartingSeconds = "LEVELRAIL_INGRESS_DIAGNOSTIC_STARTING_SECONDS"
	defaultDiagnosticStarting    = 60
)

func diagnosticStartingWindow() time.Duration {
	if n, err := strconv.Atoi(os.Getenv(envDiagnosticStartingSeconds)); err == nil && n > 0 {
		return time.Duration(n) * time.Second
	}
	return defaultDiagnosticStarting * time.Second
}

// wireDiagnostic starts the host state snapshot, points the ingress catch-all
// at it with a per-process token, and feeds the unrouted setting to the syncer.
func wireDiagnostic(ctx context.Context, rt *api.Router, hardening *ingressdriver.Hardening, syncer *proxyroutes.Syncer, logger *slog.Logger) {
	if rs := rt.RuntimeSettings(); rs != nil {
		syncer.Unrouted = api.UnroutedEnabled(rs)
		syncer.UnroutedPriority = proxyroutes.DefaultUnroutedPriority
	}
	dial := dashboardDialAddr(httpAddr())
	if hardening == nil || dial == "" {
		return
	}
	rc, err := ingressdriver.NewRouteCheck()
	if err != nil {
		logger.Warn("diagnostic page keeps the static fallback", slog.String("error", err.Error()))
		return
	}
	if rt.StartDiagnostic(ctx, rc.Token, hardening.BrandName, hardening.DocsURL, diagnosticStartingWindow()) == nil {
		return
	}
	hardening.DiagnosticDial, hardening.DiagnosticToken = dial, rc.Token
}
