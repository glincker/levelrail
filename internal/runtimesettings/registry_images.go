package runtimesettings

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
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

// KeyImagesPullConcurrency caps how many distinct image pulls run at once on a node.
const KeyImagesPullConcurrency = "images.pull_concurrency"

func init() {
	Register(Definition{
		Key: KeyImagesPullConcurrency, Category: CategoryDeploys, Kind: KindInt, Default: "3",
		Env: "APP_IMAGES_PULL_CONCURRENCY", Apply: ApplyLive, CheckValue: intRangeCheck(1, 32),
		Title:       "Image pulls at once",
		Description: "Most distinct image pulls a node runs at the same time, from 1 to 32. Deploys that need the same image share one pull.",
	})
}

// Per-app image keep keys. 0 follows images.keep_per_app.
const (
	KeyAppImageKeep          = "app.image_keep"
	KeyAppImageKeepOverrides = "app.image_keep_overrides"
)

// AppImageKeepMin is the smallest per-app keep, so the previous successful release survives.
const AppImageKeepMin = 2

// appImageKeepEntriesMax bounds the override map so one write cannot bloat the settings row.
const appImageKeepEntriesMax = 10000

// CheckAppImageKeep accepts 0 (follow the global count) or AppImageKeepMin up to the ceiling.
func CheckAppImageKeep(n int) error {
	if n == 0 || (n >= AppImageKeepMin && n <= imagesKeepCeiling) {
		return nil
	}
	return fmt.Errorf("must be 0 (follow the global count) or from %d to %d", AppImageKeepMin, imagesKeepCeiling)
}

// CheckAppImageKeepCount refuses a map with more apps than the settings row may hold.
func CheckAppImageKeepCount(n int) error {
	if n > appImageKeepEntriesMax {
		return fmt.Errorf("at most %d apps can have their own image keep count", appImageKeepEntriesMax)
	}
	return nil
}

// ParseAppImageKeepOverrides reads the per-app keep map; an empty value is no overrides.
func ParseAppImageKeepOverrides(raw string) (map[string]int, error) {
	out := map[string]int{}
	if strings.TrimSpace(raw) == "" {
		return out, nil
	}
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil, fmt.Errorf("must be a JSON object of app name to keep count: %w", err)
	}
	if len(out) > appImageKeepEntriesMax {
		return nil, fmt.Errorf("at most %d apps", appImageKeepEntriesMax)
	}
	for app, n := range out {
		if app == "" {
			return nil, errors.New("app names must not be empty")
		}
		if err := CheckAppImageKeep(n); err != nil {
			return nil, fmt.Errorf("app %q: %w", app, err)
		}
	}
	return out, nil
}

func init() {
	Register(Definition{
		Key: KeyAppImageKeep, Category: CategoryDeploys, Kind: KindInt, Default: "0",
		Env: "APP_IMAGE_KEEP", Apply: ApplyLive,
		CheckValue: func(canonical string) error {
			n, err := strconv.Atoi(strings.TrimSpace(canonical))
			if err != nil {
				return errors.New("must be a whole number")
			}
			return CheckAppImageKeep(n)
		},
		Title:       "Default images kept per app",
		Description: "Images each app keeps when it has no keep count of its own: 0 follows the images-kept setting, otherwise 2 to 1000. Live images, rollback pins and images a queued or held deploy needs are always kept.",
	})
	Register(Definition{
		Key: KeyAppImageKeepOverrides, Category: CategoryDeploys, Kind: KindString, Default: "",
		Apply: ApplyLive,
		CheckValue: func(canonical string) error {
			_, err := ParseAppImageKeepOverrides(canonical)
			return err
		},
		Title:       "Images kept, per app",
		Description: "Keep counts set on single apps, as a JSON object of app name to count. Set them from each app's deploy settings or the images retention CLI rather than by hand.",
	})
}
