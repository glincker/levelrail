package runtimesettings

import (
	"fmt"
	"strings"
	"time"
)

// Boot recheck keys.
const (
	KeyBootRechecks     = "boot.rechecks"
	KeyBootEventRecheck = "boot.event_recheck"
)

// Floors keep a typo from turning the post-start passes into a busy loop.
const (
	minBootRecheck      = time.Second
	minBootEventRecheck = time.Second
)

func bootRecheckItemCheck(item string) error {
	d, err := time.ParseDuration(strings.TrimSpace(item))
	if err != nil || d < minBootRecheck {
		return fmt.Errorf("%q must be a duration of at least %s", item, minBootRecheck)
	}
	return nil
}

// bootRechecksFromEnv turns the legacy comma separated env form into the list form.
func bootRechecksFromEnv(raw string) (string, error) {
	return strings.ReplaceAll(strings.TrimSpace(raw), ",", ":"), nil
}

func init() {
	Register(Definition{
		Key: KeyBootRechecks, Category: CategoryDeploys, Kind: KindStringList, Default: "3s:10s",
		Env: "APP_BOOT_RECHECKS", Apply: ApplyRestart, CheckItem: bootRecheckItemCheck, EnvConvert: bootRechecksFromEnv,
		TitleKey: "runtimeSettings:items.bootRechecks.title", DescKey: "runtimeSettings:items.bootRechecks.description",
		Title:       "Boot recheck delays",
		Description: "Offsets after start, colon separated (for example 3s:10s), at which the control plane runs extra reconcile passes so routes are fixed when Docker publishes ports late after a host reboot. Each offset is at least 1s. Read at start.",
	})
	Register(Definition{
		Key: KeyBootEventRecheck, Category: CategoryDeploys, Kind: KindDuration, Default: "3s",
		Env: "APP_BOOT_EVENT_RECHECK", Apply: ApplyRestart, CheckValue: durationRangeCheck(minBootEventRecheck, time.Hour),
		TitleKey: "runtimeSettings:items.bootEventRecheck.title", DescKey: "runtimeSettings:items.bootEventRecheck.description",
		Title:       "Boot recheck after first event",
		Description: "How long after the first Docker event the control plane runs one more reconcile pass, from 1s to 1h. Read at start.",
	})
}
