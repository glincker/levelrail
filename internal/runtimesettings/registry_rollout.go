package runtimesettings

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/canarypolicy"
	"github.com/GLINCKER/levelrail/internal/rolloutpolicy"
)

// Platform defaults for rolling deploys and staged canaries. An app's own
// rollout or canary block still wins; the env names only seed these.
const (
	KeyRolloutMaxSurge       = "rollout.max_surge"
	KeyRolloutMaxUnavailable = "rollout.max_unavailable"
	KeyRolloutMinAvailable   = "rollout.min_available"
	KeyCanaryStages          = "canary.stages"
	KeyCanaryInterval        = "canary.interval"
	KeyCanaryMaxErrorRate    = "canary.max_error_rate"
	SettingCanaryMaxP95Ratio = "canary.max_p95_ratio"
	KeyCanaryMinRequests     = "canary.min_requests"
	SettingCanaryP95FloorMS  = "canary.p95_floor_ms"
	KeyCanaryTick            = "canary.tick"
	KeyCanarySticky          = "canary.sticky"
)

// rolloutEnvKeys maps each env name the policy packages read to its setting.
var rolloutEnvKeys = map[string]string{
	rolloutpolicy.EnvMaxSurge:       KeyRolloutMaxSurge,
	rolloutpolicy.EnvMaxUnavailable: KeyRolloutMaxUnavailable,
	rolloutpolicy.EnvMinAvailable:   KeyRolloutMinAvailable,
	canarypolicy.EnvStages:          KeyCanaryStages,
	canarypolicy.EnvInterval:        KeyCanaryInterval,
	canarypolicy.EnvMaxErrorRate:    KeyCanaryMaxErrorRate,
	canarypolicy.EnvMaxP95Ratio:     SettingCanaryMaxP95Ratio,
	canarypolicy.EnvMinRequests:     KeyCanaryMinRequests,
	canarypolicy.EnvP95FloorMS:      SettingCanaryP95FloorMS,
	canarypolicy.EnvTick:            KeyCanaryTick,
	canarypolicy.EnvSticky:          KeyCanarySticky,
}

func rolloutValueCheck(canonical string) error {
	_, _, err := rolloutpolicy.ParseValue(canonical)
	return err
}

func floatRangeCheck(floor, ceil float64, floorExclusive bool) func(string) error {
	return func(canonical string) error {
		f, err := strconv.ParseFloat(strings.TrimSpace(canonical), 64)
		if err != nil || f > ceil || f < floor || (floorExclusive && f == floor) {
			return fmt.Errorf("value must be a number above %g and at most %g", floor, ceil)
		}
		return nil
	}
}

func canaryStagesCheck(canonical string) error {
	stages, err := canarypolicy.ParseStages(canonical)
	if err != nil {
		return err
	}
	return canarypolicy.ValidateStages(stages)
}

func init() {
	Register(Definition{
		Key: KeyRolloutMaxSurge, Category: CategoryDeploys, Kind: KindString, Default: "1",
		Env: rolloutpolicy.EnvMaxSurge, Apply: ApplyLive, CheckValue: rolloutValueCheck,
		Title:       "Rolling deploy default max surge",
		Description: "Extra replicas a rolling deploy may start above the desired count, as a whole number or a percent such as 25%. An app's own rollout block wins. The next rollout uses a changed value.",
	})
	Register(Definition{
		Key: KeyRolloutMaxUnavailable, Category: CategoryDeploys, Kind: KindString, Default: "0",
		Env: rolloutpolicy.EnvMaxUnavailable, Apply: ApplyLive, CheckValue: rolloutValueCheck,
		Title:       "Rolling deploy default max unavailable",
		Description: "Replicas a rolling deploy may take out of service at once, as a whole number or a percent. An app's own rollout block wins. With max surge at 0 this must stay above 0. The next rollout uses a changed value.",
	})
	Register(Definition{
		Key: KeyRolloutMinAvailable, Category: CategoryDeploys, Kind: KindString, Default: "0",
		Env: rolloutpolicy.EnvMinAvailable, Apply: ApplyLive, CheckValue: rolloutValueCheck,
		Title:       "Default min available replicas",
		Description: "Replicas that must keep serving during a rollout or a restart, as a whole number or a percent. 0 sets no floor. An app's own rollout block wins. The next rollout uses a changed value.",
	})
	Register(Definition{
		Key: KeyCanaryStages, Category: CategoryDeploys, Kind: KindString, Default: "10,25,50,100",
		Env: canarypolicy.EnvStages, Apply: ApplyLive, CheckValue: canaryStagesCheck,
		Title:       "Canary default stages",
		Description: "Traffic percents a staged canary steps through, ascending and ending at 100, such as 10,25,50,100. A canary started after the change uses it; running canaries keep their saved schedule.",
	})
	Register(Definition{
		Key: KeyCanaryInterval, Category: CategoryDeploys, Kind: KindDuration, Default: "5m",
		Env: canarypolicy.EnvInterval, Apply: ApplyLive, CheckValue: durationRangeCheck(time.Second, 24*time.Hour),
		Title:       "Canary default stage interval",
		Description: "How long each canary stage observes traffic before it may advance, from 1s to 24h. A canary started after the change uses it.",
	})
	Register(Definition{
		Key: KeyCanaryMaxErrorRate, Category: CategoryDeploys, Kind: KindString, Default: "0.02",
		Env: canarypolicy.EnvMaxErrorRate, Apply: ApplyLive, CheckValue: floatRangeCheck(0, 1, true),
		Title:       "Canary default max error rate rise",
		Description: "How far the canary's 5xx rate may exceed the stable release's before it rolls back, as a fraction above 0 and at most 1 (0.02 is two points). A canary started after the change uses it.",
	})
	Register(Definition{
		Key: SettingCanaryMaxP95Ratio, Category: CategoryDeploys, Kind: KindString, Default: "1.5",
		Env: canarypolicy.EnvMaxP95Ratio, Apply: ApplyLive, CheckValue: floatRangeCheck(1, 1000, true),
		Title:       "Canary default max p95 ratio",
		Description: "How many times the stable p95 latency the canary's p95 may reach before it rolls back, above 1. A canary started after the change uses it.",
	})
	Register(Definition{
		Key: KeyCanaryMinRequests, Category: CategoryDeploys, Kind: KindInt, Default: "30",
		Env: canarypolicy.EnvMinRequests, Apply: ApplyLive, CheckValue: intRangeCheck(1, 1000000),
		Title:       "Canary requests needed per stage",
		Description: "Canary requests a stage needs before it is judged, from 1 to 1000000. The next reconcile tick uses a changed value.",
	})
	Register(Definition{
		Key: SettingCanaryP95FloorMS, Category: CategoryDeploys, Kind: KindString, Default: "50",
		Env: canarypolicy.EnvP95FloorMS, Apply: ApplyLive, CheckValue: floatRangeCheck(0, 600000, false),
		Title:       "Canary p95 latency floor",
		Description: "Milliseconds below which the canary's p95 is never judged against the stable release, so tiny latencies do not trip a rollback. The next reconcile tick uses a changed value.",
	})
	Register(Definition{
		Key: KeyCanaryTick, Category: CategoryDeploys, Kind: KindDuration, Default: "15s",
		Env: canarypolicy.EnvTick, Apply: ApplyLive, CheckValue: durationRangeCheck(time.Second, time.Hour),
		Title:       "Canary reconcile interval",
		Description: "How often the canary reconciler checks running canaries, from 1s to 1h. The next tick uses a changed value.",
	})
}

func init() {
	Register(Definition{
		Key: KeyCanarySticky, Category: CategoryDeploys, Kind: KindBool, Default: "true",
		Env: canarypolicy.EnvSticky, Apply: ApplyLive,
		Title:       "Canary sticky visitors",
		Description: "When on, a visitor keeps the release (stable or canary) they were first sent to, through a cookie scoped to the app and canary. Raising the weight then moves new visitors only. When off, every request is split by weight. Applied on the next ingress reconcile.",
	})
}

// BindRolloutDefaults makes the rollout and canary env defaults read these
// settings. Only the service that handles settings writes may call it.
func BindRolloutDefaults(s *Service) {
	if s == nil || s.store == nil {
		return
	}
	lookup := func(env string) string {
		key, ok := rolloutEnvKeys[env]
		if !ok {
			return ""
		}
		return s.String(context.Background(), key)
	}
	rolloutpolicy.SetLiveEnv(lookup)
	canarypolicy.SetLiveEnv(lookup)
}
