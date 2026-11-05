package api

import (
	"strings"

	"github.com/GLINCKER/levelrail/internal/placement"
	"github.com/GLINCKER/levelrail/internal/runtimesettings"
)

// Registered here so internal/placement does not depend on the settings package.
func init() {
	runtimesettings.Register(runtimesettings.Definition{
		Key: KeyPlacementScoreMode, Category: runtimesettings.CategoryDeploys, Kind: runtimesettings.KindString,
		Default: placement.ScoreSpread, Env: "APP_PLACEMENT_SCORE_MODE", Apply: runtimesettings.ApplyLive,
		AllowedStrings: []string{placement.ScoreSpread, placement.ScoreBinpack},
		Risk:           runtimesettings.RiskNormal,
		EnvConvert:     func(raw string) (string, error) { return strings.ToLower(strings.TrimSpace(raw)), nil },
		TitleKey:       "runtimeSettings:items.placementScoreMode.title",
		DescKey:        "runtimeSettings:items.placementScoreMode.description",
		Title:          "Placement score mode",
		Description:    "How new workloads pick a node: spread keeps the most headroom free on each node, binpack fills the fullest node that still fits. Read on every placement; the environment variable only seeds the default.",
	})
}
