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
		Title:       "Diagnostic page cache",
		Description: "Seconds the 'no app is serving this address' page reuses a host's state before looking it up again. 0 looks up on every request. Read live; the environment variable only seeds the default.",
	})
	runtimesettings.Register(runtimesettings.Definition{
		Key: proxyroutes.KeyUnroutedToLevelrail, Category: runtimesettings.CategoryNetworking, Kind: runtimesettings.KindBool,
		Default: "false", Env: proxyroutes.EnvUnroutedToLevelrail, Apply: runtimesettings.ApplyLive,
		Title:       "Send unrouted hosts to Levelrail",
		Description: "Write a low priority catch-all route so hosts no router claims reach the Levelrail diagnostic page instead of a bare proxy 503. Traefik only. It also catches hosts of other apps on the same front proxy when they have no router at all. Applies on the next route sync.",
	})
}

// DiagnosticCacheTTL is the live cache lifetime for the diagnostic page lookup.
func DiagnosticCacheTTL(svc *runtimesettings.Service) func(context.Context) time.Duration {
	return func(ctx context.Context) time.Duration {
		return time.Duration(svc.Int(ctx, ingress.KeyDiagnosticCacheTTLSeconds)) * time.Second
	}
}

// UnroutedEnabled is the live opt in for the managed catch-all file.
func UnroutedEnabled(svc *runtimesettings.Service) func(context.Context) bool {
	return func(ctx context.Context) bool { return svc.Bool(ctx, proxyroutes.KeyUnroutedToLevelrail) }
}

func checkDiagnosticTTL(canonical string) error {
	if n, err := strconv.Atoi(canonical); err != nil || n < 0 || n > 3600 {
		return fmt.Errorf("value must be between 0 and 3600")
	}
	return nil
}
