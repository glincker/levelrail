package runtimesettings

import "github.com/GLINCKER/levelrail/internal/store"

// legacyKeys maps a retired saved key to its current key. Add an entry in the
// same change that renames a setting so an upgrade keeps the operator's value.
var legacyKeys = map[string]string{}

// applyLegacyKeys returns rows with retired keys moved to their new key. An
// explicit row under the new key always wins over a migrated one.
func applyLegacyKeys(rows map[string]store.RuntimeSettingRow) map[string]store.RuntimeSettingRow {
	for old, cur := range legacyKeys {
		r, ok := rows[old]
		if !ok {
			continue
		}
		if _, has := rows[cur]; !has {
			r.Key = cur
			rows[cur] = r
		}
		delete(rows, old)
	}
	return rows
}
