package api

import (
	"fmt"
	"strconv"

	"github.com/GLINCKER/levelrail/internal/runtimesettings"
)

// KeyPreviewTTLHours is the runtime setting for the default preview lifetime.
const KeyPreviewTTLHours = "deploy.preview_ttl_hours"

func init() {
	runtimesettings.Register(runtimesettings.Definition{
		Key: KeyPreviewTTLHours, Category: runtimesettings.CategoryDeploys, Kind: runtimesettings.KindInt,
		Default: "0", Apply: runtimesettings.ApplyLive,
		CheckValue:  checkPreviewTTLHours,
		Title:       "Preview lifetime (hours)",
		Description: "Hours a pull request preview may go without an update before the sweep removes it. 0 keeps the startup default. A per-app override still wins. Read on every sweep and listing.",
	})
}

func checkPreviewTTLHours(canonical string) error {
	if n, err := strconv.Atoi(canonical); err != nil || n < 0 || n > maxPreviewTTLHours {
		return fmt.Errorf("value must be between 0 and %d", maxPreviewTTLHours)
	}
	return nil
}
