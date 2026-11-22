package runtimesettings

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// Registered keys for the instance wide quota applied to teams and projects with none of their own. 0 is unlimited.
const (
	KeyQuotaDefaultMaxApps      = "quota.default_max_apps"
	KeyQuotaDefaultMaxDatabases = "quota.default_max_databases"
	KeyQuotaDefaultMaxMemoryMB  = "quota.default_max_memory_mb"
	KeyQuotaDefaultMaxCPU       = "quota.default_max_cpu"
	KeyQuotaDefaultMaxPreviews  = "quota.default_max_previews"

	quotaDefaultCountCeiling = 1_000_000
	quotaDefaultMemoryMBMax  = 1 << 30
)

// checkQuotaCPU keeps the default CPU quota a finite, non-negative number of cores; CPU is fractional so it is a string setting.
func checkQuotaCPU(canonical string) error {
	f, err := strconv.ParseFloat(strings.TrimSpace(canonical), 64)
	if err != nil || math.IsNaN(f) || math.IsInf(f, 0) || f < 0 {
		return fmt.Errorf("%q is not a non-negative number of cores", canonical)
	}
	return nil
}

func init() {
	registerLimit(Definition{
		Key: KeyQuotaDefaultMaxApps, Category: CategoryDeploys, Default: "0", Env: "APP_QUOTA_DEFAULT_MAX_APPS", Apply: ApplyLive,
		Title:       "Default quota: apps",
		Description: "Most apps a team or project may own when it has no quota of its own. 0 means unlimited. A team's own quota wins. Applies to the next create.",
	}, 0, quotaDefaultCountCeiling)
	registerLimit(Definition{
		Key: KeyQuotaDefaultMaxDatabases, Category: CategoryDeploys, Default: "0", Env: "APP_QUOTA_DEFAULT_MAX_DATABASES", Apply: ApplyLive,
		Title:       "Default quota: databases",
		Description: "Most databases a team or project may own when it has no quota of its own. 0 means unlimited. A team's own quota wins. Applies to the next create.",
	}, 0, quotaDefaultCountCeiling)
	registerLimit(Definition{
		Key: KeyQuotaDefaultMaxMemoryMB, Category: CategoryDeploys, Default: "0", Env: "APP_QUOTA_DEFAULT_MAX_MEMORY_MB", Apply: ApplyLive,
		Title:       "Default quota: memory (MB)",
		Description: "Total memory reservation in MB a team or project may hold when it has no quota of its own. 0 means unlimited. A team's own quota wins. Applies to the next create or resize.",
	}, 0, quotaDefaultMemoryMBMax)
	registerLimit(Definition{
		Key: KeyQuotaDefaultMaxPreviews, Category: CategoryDeploys, Default: "0", Env: "APP_QUOTA_DEFAULT_MAX_PREVIEWS", Apply: ApplyLive,
		Title:       "Default quota: previews",
		Description: "Most live previews a team or project may hold when it has no quota of its own. 0 means unlimited. A team's own quota wins. Applies to the next preview.",
	}, 0, quotaDefaultCountCeiling)
	Register(Definition{
		Key: KeyQuotaDefaultMaxCPU, Category: CategoryDeploys, Kind: KindString, Default: "0", Env: "APP_QUOTA_DEFAULT_MAX_CPU", Apply: ApplyLive,
		CheckValue:  checkQuotaCPU,
		Title:       "Default quota: CPU (cores)",
		Description: "Total CPU reservation in cores, fractions allowed such as 0.5, a team or project may hold when it has no quota of its own. 0 means unlimited. A team's own quota wins. Applies to the next create or resize.",
	})
}
