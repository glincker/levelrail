package statuspage

import (
	"os"
	"strconv"
)

// Tuning holds every operator-adjustable limit and weight of the page.
// Each field reads one APP_STATUS_* variable; the defaults are documented
// in docs/status-page.md.
type Tuning struct {
	DegradedWeight float64 // APP_STATUS_DEGRADED_WEIGHT: downtime weight of a degraded sample, 0..1
	DayOutageShare float64 // APP_STATUS_DAY_OUTAGE_SHARE: weighted downtime share that paints a day fully red
	AboutMax       int     // APP_STATUS_ABOUT_MAX: about text length cap
	DescriptionMax int     // APP_STATUS_DESCRIPTION_MAX: component description length cap
	GroupNameMax   int     // APP_STATUS_GROUP_NAME_MAX
	GroupsMax      int     // APP_STATUS_GROUPS_MAX
	LogoURLMax     int     // APP_STATUS_LOGO_URL_MAX
	HistoryPage    int     // APP_STATUS_HISTORY_PAGE_SIZE: entries per history page
	HistoryDays    int     // APP_STATUS_HISTORY_DAYS: archive depth
}

// TuningFromEnv reads the knobs, falling back to the defaults for unset or invalid values.
func TuningFromEnv() Tuning {
	return Tuning{
		DegradedWeight: envFloat("APP_STATUS_DEGRADED_WEIGHT", 0.5, 0, 1),
		DayOutageShare: envFloat("APP_STATUS_DAY_OUTAGE_SHARE", 0.05, 0.0001, 1),
		AboutMax:       envInt("APP_STATUS_ABOUT_MAX", 1000),
		DescriptionMax: envInt("APP_STATUS_DESCRIPTION_MAX", 200),
		GroupNameMax:   envInt("APP_STATUS_GROUP_NAME_MAX", 60),
		GroupsMax:      envInt("APP_STATUS_GROUPS_MAX", 30),
		LogoURLMax:     envInt("APP_STATUS_LOGO_URL_MAX", 512),
		HistoryPage:    envInt("APP_STATUS_HISTORY_PAGE_SIZE", 10),
		HistoryDays:    envInt("APP_STATUS_HISTORY_DAYS", UptimeDays),
	}
}

func envInt(name string, def int) int {
	if n, err := strconv.Atoi(os.Getenv(name)); err == nil && n > 0 {
		return n
	}
	return def
}

func envFloat(name string, def, lo, hi float64) float64 {
	if f, err := strconv.ParseFloat(os.Getenv(name), 64); err == nil && f >= lo && f <= hi {
		return f
	}
	return def
}
