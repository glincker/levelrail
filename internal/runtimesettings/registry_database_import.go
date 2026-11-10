package runtimesettings

import "strconv"

// KeyDatabaseImportMaxBytes caps one dump uploaded to a managed database.
const (
	KeyDatabaseImportMaxBytes = "databases.import_max_bytes"

	databaseImportCeiling = 1 << 40
)

func init() {
	registerLimit(Definition{
		Key: KeyDatabaseImportMaxBytes, Category: CategoryNetworking, Default: strconv.Itoa(1 << 30), Env: "APP_DATABASE_IMPORT_MAX_BYTES", Apply: ApplyLive, Risk: RiskWeakensSecurity, HigherIsWeaker: true,
		Title:       "Database import size limit",
		Description: "Largest dump in bytes accepted by a database import (databases import, POST /api/v1/databases/{name}/import); a bigger upload is refused with HTTP 413 and nothing is restored. Between 1 MiB and 1 TiB; imports are exempt from the API request body limit and obey only this one. A change applies to the next import.",
	}, bodyLimitFloor, databaseImportCeiling)
}
