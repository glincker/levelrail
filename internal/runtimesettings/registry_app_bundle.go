package runtimesettings

import "strconv"

// App bundle import keys. A bundle is an uploaded file from anywhere, so
// every cap has a floor and a ceiling and a stray seed can never lift it.
const (
	KeyAppBundleMaxBytes      = "apps.bundle_import_max_bytes"
	KeyAppBundleMaxEntries    = "apps.bundle_import_max_entries"
	KeyAppBundleMaxEntryBytes = "apps.bundle_import_max_entry_bytes"
)

func init() {
	registerLimit(Definition{
		Key: KeyAppBundleMaxBytes, Category: CategoryNetworking, Default: strconv.Itoa(4 << 20), Env: "APP_BUNDLE_IMPORT_MAX_BYTES", Apply: ApplyLive,
		Title:       "Apps: bundle import size limit",
		Description: "Largest app bundle in bytes that an import accepts, from 4096 to 33554432. A bigger upload is refused. A change applies to the next import.",
	}, 4096, 32<<20)
	registerLimit(Definition{
		Key: KeyAppBundleMaxEntries, Category: CategoryNetworking, Default: "16", Env: "APP_BUNDLE_IMPORT_MAX_ENTRIES", Apply: ApplyLive,
		Title:       "Apps: bundle import entry limit",
		Description: "Most files one app bundle may hold, from 1 to 64. A bundle with more entries is refused. A change applies to the next import.",
	}, 1, 64)
	registerLimit(Definition{
		Key: KeyAppBundleMaxEntryBytes, Category: CategoryNetworking, Default: strconv.Itoa(1 << 20), Env: "APP_BUNDLE_IMPORT_MAX_ENTRY_BYTES", Apply: ApplyLive,
		Title:       "Apps: bundle import file size limit",
		Description: "Largest single file in bytes inside an app bundle, from 1024 to 8388608. A bundle with a bigger file is refused. A change applies to the next import.",
	}, 1024, 8<<20)
}
