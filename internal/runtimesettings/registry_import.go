package runtimesettings

// Importer input bound setting keys.
const (
	KeyImportMaxBytes = "import.max_bytes"
	KeyImportMaxItems = "import.max_items"
	importMinBytes    = 64 << 10
	importMaxBytes    = 512 << 20
	importMaxItemsCap = 100000
)

func init() {
	Register(Definition{
		Key: KeyImportMaxBytes, Category: CategoryDeploys, Kind: KindInt, Default: "8388608",
		Env: "APP_IMPORT_MAX_BYTES", Apply: ApplyLive, EnvLoose: true, CheckValue: intRangeCheck(importMinBytes, importMaxBytes),
		Title:       "Largest import source payload",
		Description: "Most bytes read from one platform API response or container scan during an app or Kubernetes import, from 64 KiB to 512 MiB. A bigger source is refused and nothing is imported. A change applies to the next import.",
	})
	Register(Definition{
		Key: KeyImportMaxItems, Category: CategoryDeploys, Kind: KindInt, Default: "5000",
		Env: "APP_IMPORT_MAX_ITEMS", Apply: ApplyLive, EnvLoose: true, CheckValue: intRangeCheck(1, importMaxItemsCap),
		Title:       "Most apps per import",
		Description: "Most apps or containers one import discovery may return, from 1 to 100000. A larger source is refused with a clear error. A change applies to the next import.",
	})
}
