package api

import "strings"

const (
	existingVolumeReuse   = "reuse"
	existingVolumeDiscard = "discard"
)

// databaseVolumeNames mirrors internal/reconcile/database's volume naming,
// the same duplication desiredVolumeNames documents.
func databaseVolumeNames(dbName string) []string {
	return []string{"db-" + dbName + "-data", "db-" + dbName + "-certs", "db-" + dbName + "-wal-archive"}
}

// validExistingVolume accepts the stored leftover-volume choice. The volume
// itself is inspected by the reconciler, never by the create request.
func validExistingVolume(choice string) (string, bool) {
	c := strings.TrimSpace(choice)
	switch c {
	case "", existingVolumeReuse, existingVolumeDiscard:
		return c, true
	}
	return c, false
}
