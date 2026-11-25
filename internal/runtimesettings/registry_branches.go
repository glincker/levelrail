package runtimesettings

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Database branch settings (ADR 081).
const (
	KeyBranchesEnabled            = "branches.enabled"
	KeyBranchesExpiryInterval     = "branches.expiry_interval"
	KeyBranchesExpiryWarning      = "branches.expiry_warning"
	KeyBranchesDefaultTTL         = "branches.default_ttl"
	KeyBranchesPreviewDefaultTTL  = "branches.preview_default_ttl"
	KeyBranchesFailedRetention    = "branches.failed_retention"
	KeyBranchesMaxPerRoot         = "branches.max_per_root"
	KeyBranchesMaxTotal           = "branches.max_total"
	KeyBranchesMaxTotalBytes      = "branches.max_total_bytes"
	KeyBranchesMaxDepth           = "branches.max_depth"
	KeyBranchesSlowPathMaxBytes   = "branches.slow_path_max_bytes"
	KeyBranchesSlowPathHeadroom   = "branches.slow_path_headroom_factor"
	KeyBranchesFastPath           = "branches.fast_path"
	KeyBranchesActivityInterval   = "branches.activity_interval"
	KeyBranchesSizeInterval       = "branches.size_interval"
	KeyBranchesMaskPreviewRows    = "branches.mask_preview_rows"
	KeyBranchesCloneTimeout       = "branches.clone_timeout"
	branchBytesCeil               = 1 << 50
	branchDefaultMaxTotalBytes    = "53687091200"
	branchDefaultSlowPathMaxBytes = "21474836480"
)

func branchHeadroomCheck(canonical string) error {
	f, err := strconv.ParseFloat(canonical, 64)
	if err != nil || !(f >= 1 && f <= 10) {
		return fmt.Errorf("value must be a number between 1 and 10")
	}
	return nil
}

func branchBytesCheck(canonical string) error {
	n, err := strconv.ParseInt(canonical, 10, 64)
	if err != nil || n < 0 || n > branchBytesCeil {
		return fmt.Errorf("value must be a byte count of at most %d", int64(branchBytesCeil))
	}
	return nil
}

// branchEnvConvert makes an env seeded value pass the same check as a saved one, so 0s or 0 cannot slip in.
func branchEnvConvert(check func(string) error) func(string) (string, error) {
	return func(raw string) (string, error) {
		v := strings.TrimSpace(raw)
		if err := check(v); err != nil {
			return "", err
		}
		return v, nil
	}
}

func branchDurationCheck(lo, hi time.Duration) func(string) error {
	return func(canonical string) error {
		d, err := time.ParseDuration(canonical)
		if err != nil || d < lo || d > hi {
			return fmt.Errorf("duration must be between %s and %s", lo, hi)
		}
		return nil
	}
}

// Branch durations are positive with a floor so no loop spins and no lifetime is zero by accident.
func init() {
	dur := func(key, def, env, title, desc string, hi time.Duration) {
		Register(Definition{Key: key, Category: CategoryDeploys, Kind: KindDuration, Default: def, Env: env, Apply: ApplyLive, CheckValue: branchDurationCheck(time.Minute, hi), EnvConvert: branchEnvConvert(branchDurationCheck(time.Minute, hi)), Title: title, Description: desc})
	}
	num := func(key, def, env, title, desc string, check func(string) error) {
		Register(Definition{Key: key, Category: CategoryDeploys, Kind: KindInt, Default: def, Env: env, Apply: ApplyLive, CheckValue: check, EnvConvert: branchEnvConvert(check), Title: title, Description: desc})
	}
	Register(Definition{
		Key: KeyBranchesEnabled, Category: CategoryDeploys, Kind: KindBool, Default: "true", Env: "APP_BRANCHES_ENABLED", Apply: ApplyLive,
		Title: "Database branches", Description: "Allow creating database branches. Off refuses new branches and keeps existing ones.",
	})
	dur(KeyBranchesExpiryInterval, "5m", "APP_BRANCHES_EXPIRY_INTERVAL", "Branch expiry check interval", "How often expired and failed branches are collected.", 24*time.Hour)
	dur(KeyBranchesExpiryWarning, "2h", "APP_BRANCHES_EXPIRY_WARNING", "Branch expiry warning", "How long before expiry a warning is sent.", 30*24*time.Hour)
	dur(KeyBranchesDefaultTTL, "24h", "APP_BRANCHES_DEFAULT_TTL", "Default branch lifetime", "Lifetime of a manually created branch when no expiry is chosen.", 3650*24*time.Hour)
	dur(KeyBranchesPreviewDefaultTTL, "72h", "APP_BRANCHES_PREVIEW_DEFAULT_TTL", "Preview branch lifetime", "Lifetime of a branch created for a pull request preview.", 3650*24*time.Hour)
	dur(KeyBranchesFailedRetention, "24h", "APP_BRANCHES_FAILED_RETENTION", "Failed branch retention", "How long a failed branch is kept before it is deleted.", 30*24*time.Hour)
	num(KeyBranchesMaxPerRoot, "10", "APP_BRANCHES_MAX_PER_ROOT", "Branches per database tree", "Most branches one root database may have.", intRangeCheck(1, 1000))
	num(KeyBranchesMaxTotal, "50", "APP_BRANCHES_MAX_TOTAL", "Branches in total", "Most branches on the whole platform.", intRangeCheck(1, 10000))
	num(KeyBranchesMaxTotalBytes, branchDefaultMaxTotalBytes, "APP_BRANCHES_MAX_TOTAL_BYTES", "Total branch size (bytes)", "Most disk all branches together may use. 0 means no limit.", branchBytesCheck)
	num(KeyBranchesMaxDepth, "3", "APP_BRANCHES_MAX_DEPTH", "Branch depth", "Most levels of branches under a root.", intRangeCheck(1, 10))
	num(KeyBranchesSlowPathMaxBytes, branchDefaultSlowPathMaxBytes, "APP_BRANCHES_SLOW_PATH_MAX_BYTES", "Largest dump branch (bytes)", "Largest parent a dump based branch may copy. 0 means no limit.", branchBytesCheck)
	Register(Definition{
		Key: KeyBranchesSlowPathHeadroom, Category: CategoryDeploys, Kind: KindString, Default: "1.5", Env: "APP_BRANCHES_SLOW_PATH_HEADROOM", Apply: ApplyLive,
		CheckValue: branchHeadroomCheck, EnvConvert: branchEnvConvert(branchHeadroomCheck), Title: "Dump branch free space factor", Description: "Free space needed for a dump based branch, as a multiple of the parent size.",
	})
	Register(Definition{
		Key: KeyBranchesFastPath, Category: CategoryDeploys, Kind: KindString, Default: "auto", Env: "APP_BRANCHES_FAST_PATH", Apply: ApplyLive,
		CheckValue: oneOfCheck("auto", "off"), EnvConvert: branchEnvConvert(oneOfCheck("auto", "off")), Title: "Snapshot branches", Description: "auto uses a filesystem snapshot when the host supports it; off always copies through a dump.",
	})
	dur(KeyBranchesActivityInterval, "5m", "APP_BRANCHES_ACTIVITY_INTERVAL", "Branch activity sampling", "How often branch client activity is sampled.", 24*time.Hour)
	dur(KeyBranchesSizeInterval, "15m", "APP_BRANCHES_SIZE_INTERVAL", "Branch size sampling", "How often branch disk use is measured.", 24*time.Hour)
	num(KeyBranchesMaskPreviewRows, "5", "APP_BRANCHES_MASK_PREVIEW_ROWS", "Masking preview rows", "Sample rows per table in a masking dry run.", intRangeCheck(1, 100))
	dur(KeyBranchesCloneTimeout, "2h", "APP_BRANCHES_CLONE_TIMEOUT", "Branch clone timeout", "Longest a branch may stay in a copying step before it is marked failed.", 48*time.Hour)
}
