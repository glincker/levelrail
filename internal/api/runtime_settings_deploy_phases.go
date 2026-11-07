package api

import (
	"context"
	"fmt"
	"os"
	"strconv"

	"github.com/GLINCKER/levelrail/internal/runtimesettings"
)

// KeyDeployPhasesSamples is how many past successful deploys feed the typical-time baseline.
const KeyDeployPhasesSamples = "deploy.phases_samples"

// Registered here, next to the phases handler, so the settings package stays free of api types.
func init() {
	runtimesettings.Register(runtimesettings.Definition{
		Key: KeyDeployPhasesSamples, Category: runtimesettings.CategoryDeploys, Kind: runtimesettings.KindInt,
		Default: "5", Env: "APP_DEPLOY_PHASES_SAMPLES", Apply: runtimesettings.ApplyLive,
		CheckValue:  phasesSamplesCheck,
		Title:       "Deploy phase baseline samples",
		Description: "How many recent successful deploys of an app are averaged into the typical phase times and the time left estimate, from 2 to 50. Read on every request; the environment variable only seeds the default.",
	})
}

// phaseSampleCount reads the live sample count; a missing service or bad value falls back to 5.
func (rt *Router) phaseSampleCount(ctx context.Context) int {
	svc := rt.runtimeSettings
	if svc == nil {
		svc = runtimesettings.New(nil, func(k string) (string, bool) { return os.LookupEnv(k) })
	}
	if n := svc.Int(ctx, KeyDeployPhasesSamples); n >= 2 && n <= 50 {
		return n
	}
	return 5
}

func phasesSamplesCheck(canonical string) error {
	if n, err := strconv.Atoi(canonical); err != nil || n < 2 || n > 50 {
		return fmt.Errorf("value must be between 2 and 50")
	}
	return nil
}
