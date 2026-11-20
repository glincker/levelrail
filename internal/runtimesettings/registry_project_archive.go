package runtimesettings

import "strconv"

// Project archive (.lrx) import and export limits. The env vars only seed the
// defaults; a saved value applies to the next request without a restart.
const (
	KeyProjectsArchiveMaxTotalBytes = "projects.archive_max_total_bytes"
	KeyProjectsArchiveMaxEntries    = "projects.archive_max_entries"
	KeyProjectsArchiveMaxSpecBytes  = "projects.archive_max_spec_bytes"
	KeyProjectsImportMaxBytes       = "projects.import_max_bytes"
	KeyProjectsImportMaxConcurrent  = "projects.import_max_concurrent"
	KeyProjectsArchiveMinPassphrase = "projects.archive_min_passphrase"
	KeyProjectsImportMaxGlobal      = "projects.import_max_concurrent_global"
)

func init() {
	registerLimit(Definition{
		Key: KeyProjectsArchiveMaxTotalBytes, Category: CategoryDeploys, Default: strconv.Itoa(8 << 30), Env: "APP_PROJECT_ARCHIVE_MAX_TOTAL_BYTES", Apply: ApplyLive,
		Title:       "Project archive expanded size limit",
		Description: "Largest size, in bytes, a project archive may expand to once decompressed. A bigger archive is refused. Between 1 MiB and 4 TiB. A change applies to the next import.",
	}, 1<<20, 1<<42)
	registerLimit(Definition{
		Key: KeyProjectsArchiveMaxEntries, Category: CategoryDeploys, Default: "4096", Env: "APP_PROJECT_ARCHIVE_MAX_ENTRIES", Apply: ApplyLive,
		Title:       "Project archive entry limit",
		Description: "Most files one project archive may hold. Between 16 and 100000. A change applies to the next import.",
	}, 16, 100000)
	registerLimit(Definition{
		Key: KeyProjectsArchiveMaxSpecBytes, Category: CategoryDeploys, Default: strconv.Itoa(32 << 20), Env: "APP_PROJECT_ARCHIVE_MAX_SPEC_BYTES", Apply: ApplyLive,
		Title:       "Project archive spec size limit",
		Description: "Most bytes of spec and secret files an import holds in memory. Between 1 MiB and 1 GiB. A change applies to the next import.",
	}, 1<<20, 1<<30)
	registerLimit(Definition{
		Key: KeyProjectsImportMaxBytes, Category: CategoryDeploys, Default: strconv.Itoa(256 << 20), Env: "APP_PROJECT_IMPORT_MAX_BYTES", Apply: ApplyLive,
		Title:       "Project import upload size limit",
		Description: "Largest import request body in bytes, which holds the base64 archive. Between 1 MiB and 64 GiB. A change applies to the next import.",
	}, 1<<20, 1<<36)
	registerLimit(Definition{
		Key: KeyProjectsImportMaxConcurrent, Category: CategoryDeploys, Default: "2", Env: "APP_PROJECT_IMPORT_MAX_CONCURRENT", Apply: ApplyLive,
		Title:       "Concurrent project imports per caller",
		Description: "How many project imports one caller may run at once, from 1 to 16. Further requests get HTTP 429 until one finishes. A change applies at once.",
	}, 1, 16)
	registerLimit(Definition{
		Key: KeyProjectsImportMaxGlobal, Category: CategoryDeploys, Default: "8", Env: "APP_PROJECT_IMPORT_MAX_CONCURRENT_GLOBAL", Apply: ApplyLive,
		Title:       "Concurrent project imports, instance wide",
		Description: "How many project imports may run at once across every caller, from 1 to 256. Further requests get HTTP 429 until one finishes. A change applies at once.",
	}, 1, 256)
	registerLimit(Definition{
		Key: KeyProjectsArchiveMinPassphrase, Category: CategorySecurity, Default: "12", Env: "APP_PROJECT_ARCHIVE_MIN_PASSPHRASE", Apply: ApplyLive, Risk: RiskWeakensSecurity,
		Title:       "Project archive passphrase length",
		Description: "Shortest passphrase, in characters, accepted to seal archive secrets. Never below 12; raise it for stricter policy. A change applies to the next export.",
	}, 12, 256)
}
