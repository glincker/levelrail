package runtimesettings

import (
	"fmt"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/kit/cronexpr"
)

// Image retention keys.
const (
	KeyImagesKeepPerApp    = "images.keep_per_app"
	KeyImagesPruneSchedule = "images.prune_schedule"
)

// ImagesPruneOff turns the scheduled prune off, which is the long-standing behavior.
const ImagesPruneOff = "off"

const imagesKeepCeiling = 1000

func pruneScheduleCheck(canonical string) error {
	v := strings.TrimSpace(canonical)
	if v == ImagesPruneOff {
		return nil
	}
	if err := cronexpr.Validate(v, time.UTC, time.Now()); err != nil {
		return fmt.Errorf("must be %q or a valid cron expression such as 0 3 * * 0: %w", ImagesPruneOff, err)
	}
	return nil
}

func init() {
	Register(Definition{
		Key: KeyImagesKeepPerApp, Category: CategoryDeploys, Kind: KindInt, Default: "3",
		Env: "APP_IMAGE_KEEP_PER_APP", Apply: ApplyLive, CheckValue: intRangeCheck(1, imagesKeepCeiling),
		Title:       "Images kept per app",
		Description: "How many recent images of each app the image cleanup keeps for rollback, at least 1. Live images and rollback pins are always kept. Applies to the next cleanup, manual or scheduled.",
	})
	Register(Definition{
		Key: KeyImagesPruneSchedule, Category: CategoryDeploys, Kind: KindString, Default: ImagesPruneOff,
		Apply: ApplyLive, CheckValue: pruneScheduleCheck,
		Title:       "Scheduled image cleanup",
		Description: "A cron expression (for example 0 3 * * 0 for Sundays at 03:00) for removing old images on this node, or off. The cleanup uses the same safety rules as the manual one and the images-kept setting. Off by default.",
	})
}

// KeyImagesRemoveMaxPerRequest caps how many images one explicit removal request may name.
const KeyImagesRemoveMaxPerRequest = "images.remove_max_per_request"

func init() {
	Register(Definition{
		Key: KeyImagesRemoveMaxPerRequest, Category: CategoryRetention, Kind: KindInt, Default: "200",
		Env: "APP_IMAGES_REMOVE_MAX_PER_REQUEST", Apply: ApplyLive, CheckValue: intRangeCheck(1, 2000),
		Title:       "Images: most removed per request",
		Description: "Most images one remove request may name or select, from 1 to 2000. Guards against a mistaken select-all. The next request uses a changed value.",
	})
}
