package api

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/GLINCKER/levelrail/internal/ingress"
	"github.com/GLINCKER/levelrail/internal/proxyroutes"
	"github.com/GLINCKER/levelrail/internal/runtimesettings"
)

func init() {
	runtimesettings.Register(runtimesettings.Definition{
		Key: ingress.KeyDiagnosticCacheTTLSeconds, Category: runtimesettings.CategoryNetworking, Kind: runtimesettings.KindInt,
		Default: "5", Env: ingress.EnvDiagnosticCacheTTLSeconds, Apply: runtimesettings.ApplyLive, EnvLoose: true,
		CheckValue:  checkDiagnosticTTL,
		Title:       "Diagnostic state refresh",
		Description: "How often the host state snapshot refreshes, in seconds, minimum 1. Requests read the snapshot and never query the database. Idle while the show state setting is off. Read live; the environment variable only seeds the default.",
	})
	runtimesettings.Register(runtimesettings.Definition{
		Key: ingress.KeyDiagnosticShowState, Category: runtimesettings.CategoryNetworking, Kind: runtimesettings.KindBool,
		Default: "false", Env: ingress.EnvDiagnosticShowState, Apply: runtimesettings.ApplyLive,
		Title:       "Show app state on the diagnostic page",
		Description: "When on, the 'no app is serving this address' page tells anyone who knows a hostname whether it is attached and its deploy state (building, stopped, starting, unhealthy). Off by default: every host then gets the same generic 404 page. Enable only on hosts where that disclosure is acceptable. Applies live.",
	})
	runtimesettings.Register(runtimesettings.Definition{
		Key: proxyroutes.KeyUnroutedToLevelrail, Category: runtimesettings.CategoryNetworking, Kind: runtimesettings.KindBool,
		Default: "false", Env: proxyroutes.EnvUnroutedToLevelrail, Apply: runtimesettings.ApplyLive,
		Title:       "Send unrouted hosts to Levelrail",
		Description: "Write a low priority catch-all route so hosts no router claims reach the Levelrail diagnostic page instead of a bare proxy 503. Traefik only. Hosts with no Traefik router are then served by the Levelrail ingress over plain HTTP, with no HTTPS redirect. It also catches hosts of other apps on the same front proxy when they have no router at all. Applies on the next route sync.",
	})
}

// DiagnosticCacheTTL is the live snapshot refresh interval, never under 1 s.
func DiagnosticCacheTTL(svc *runtimesettings.Service) func(context.Context) time.Duration {
	return func(ctx context.Context) time.Duration {
		return max(time.Duration(svc.Int(ctx, ingress.KeyDiagnosticCacheTTLSeconds))*time.Second, time.Second)
	}
}

// DiagnosticShowState is the live opt in for revealing host state publicly.
func DiagnosticShowState(svc *runtimesettings.Service) func(context.Context) bool {
	return func(ctx context.Context) bool { return svc.Bool(ctx, ingress.KeyDiagnosticShowState) }
}

// UnroutedEnabled is the live opt in for the managed catch-all file.
func UnroutedEnabled(svc *runtimesettings.Service) func(context.Context) bool {
	return func(ctx context.Context) bool { return svc.Bool(ctx, proxyroutes.KeyUnroutedToLevelrail) }
}

func checkDiagnosticTTL(canonical string) error {
	if n, err := strconv.Atoi(canonical); err != nil || n < 1 || n > 3600 {
		return fmt.Errorf("value must be between 1 and 3600")
	}
	return nil
}
