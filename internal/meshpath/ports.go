package meshpath

import "strconv"

// Default range for database mesh ports, overridable with
// APP_MESH_DB_PORT_MIN and APP_MESH_DB_PORT_MAX.
const (
	DefaultDatabasePortMin = 21000
	DefaultDatabasePortMax = 21999
)

// DatabasePortRange reads the database mesh port range, falling back to the
// defaults when either bound is unset or the pair is invalid.
func DatabasePortRange(lookup func(string) (string, bool)) (int, int) {
	lo, hi := DefaultDatabasePortMin, DefaultDatabasePortMax
	if v, ok := lookup("APP_MESH_DB_PORT_MIN"); ok {
		if n, err := strconv.Atoi(v); err == nil {
			lo = n
		}
	}
	if v, ok := lookup("APP_MESH_DB_PORT_MAX"); ok {
		if n, err := strconv.Atoi(v); err == nil {
			hi = n
		}
	}
	if lo < 1024 || hi > 65535 || hi < lo {
		return DefaultDatabasePortMin, DefaultDatabasePortMax
	}
	return lo, hi
}
