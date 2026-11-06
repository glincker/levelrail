package api

import (
	"context"
	"fmt"
	"strconv"

	"github.com/GLINCKER/levelrail/internal/deploy"
	"github.com/GLINCKER/levelrail/internal/runtimesettings"
)

func init() {
	runtimesettings.Register(runtimesettings.Definition{
		Key: deploy.KeyNextDeploymentID, Category: runtimesettings.CategoryDeploys, Kind: runtimesettings.KindBool,
		Default: "true", Env: deploy.EnvNextDeploymentID, Apply: runtimesettings.ApplyLive,
		Title:       "Next.js deployment id",
		Description: "Set NEXT_DEPLOYMENT_ID to the deploy id at build time and on the app containers of Next.js apps, so a browser tab on an old build can be matched to it. A value the app sets itself always wins. Read on every deploy; the environment variable only seeds the default.",
	})
	runtimesettings.Register(runtimesettings.Definition{
		Key: deploy.KeyNextAssetGraceSecs, Category: runtimesettings.CategoryDeploys, Kind: runtimesettings.KindInt,
		Default: "1800", Env: deploy.EnvNextAssetGraceSecs, Apply: runtimesettings.ApplyLive, EnvLoose: true,
		CheckValue:  checkNextAssetGraceSecs,
		Title:       "Next.js old asset grace",
		Description: "Seconds after a cutover during which the previous Next.js release keeps serving its old static chunks and data requests. 0 turns it off. Read on every deploy.",
	})
	runtimesettings.Register(runtimesettings.Definition{
		Key: deploy.KeyNextAssetGraceMaxHeld, Category: runtimesettings.CategoryDeploys, Kind: runtimesettings.KindInt,
		Default: "1", Env: deploy.EnvNextAssetGraceMaxHeld, Apply: runtimesettings.ApplyLive, EnvLoose: true,
		CheckValue:  checkNextAssetGraceMaxHeld,
		Title:       "Next.js old asset releases held",
		Description: "How many superseded Next.js releases per app stay running during the old asset grace window. Each one keeps its containers up, so it costs their memory and CPU for the whole window. 1 holds only the previous release. Read on every deploy.",
	})
}

// wireNextSkewSettings feeds the saved values to the deploy package so changes apply live.
func wireNextSkewSettings(svc *runtimesettings.Service) {
	deploy.SetNextSkewSettings(&deploy.NextSkewSettings{
		DeploymentIDOn: func(ctx context.Context) bool { return svc.Bool(ctx, deploy.KeyNextDeploymentID) },
		GraceSeconds:   func(ctx context.Context) int { return svc.Int(ctx, deploy.KeyNextAssetGraceSecs) },
	})
	deploy.SetNextGraceMaxHeld(func(ctx context.Context) int { return svc.Int(ctx, deploy.KeyNextAssetGraceMaxHeld) })
}

func checkNextAssetGraceMaxHeld(canonical string) error {
	if n, err := strconv.Atoi(canonical); err != nil || n < 1 || n > 3 {
		return fmt.Errorf("value must be between 1 and 3")
	}
	return nil
}

func checkNextAssetGraceSecs(canonical string) error {
	if n, err := strconv.Atoi(canonical); err != nil || n < 0 || n > 604800 {
		return fmt.Errorf("value must be between 0 and 604800")
	}
	return nil
}
