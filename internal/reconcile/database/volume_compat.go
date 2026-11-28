package database

import (
	"context"
	"fmt"
	"strings"

	"github.com/glincker/levelrail/internal/store"
)

// VolumeFileReader reads one small file from a named volume; the local Docker client has it.
type VolumeFileReader interface {
	ReadVolumeFile(ctx context.Context, volumeName, relPath string) ([]byte, error)
}

// volumeMarkers lists the files that record which server version wrote a data
// directory, and how many version parts make a major. Engines without a
// reliable marker (mongodb, redis family) are absent and never refused.
var volumeMarkers = map[string]struct {
	paths []string
	parts int
}{
	store.EnginePostgres: {[]string{"PG_VERSION"}, 1},
	store.EngineMySQL:    {[]string{"mysql_upgrade_info"}, 2},
	store.EngineMariaDB:  {[]string{"mariadb_upgrade_info", "mysql_upgrade_info"}, 2},
}

// DetectVolumeMajor returns the on-disk major of a data volume, or "" when the
// engine has no marker or the volume has none (a fresh or unreadable one).
func DetectVolumeMajor(ctx context.Context, r VolumeFileReader, engine, volume string) string {
	m, ok := volumeMarkers[engine]
	if !ok || r == nil {
		return ""
	}
	for _, p := range m.paths {
		data, err := r.ReadVolumeFile(ctx, volume, p)
		if err != nil {
			continue
		}
		if major := majorOf(string(data), m.parts); major != "" {
			return major
		}
	}
	return ""
}

func majorOf(raw string, parts int) string {
	raw = strings.TrimSpace(raw)
	if i := strings.IndexAny(raw, "-\n "); i >= 0 {
		raw = raw[:i]
	}
	fields := strings.Split(raw, ".")
	if len(fields) == 0 || fields[0] == "" {
		return ""
	}
	if len(fields) > parts {
		fields = fields[:parts]
	}
	return strings.Join(fields, ".")
}

// VolumeCompatible reports whether a server of requested version can open a
// data directory written by volumeMajor. Unknown on either side is compatible.
func VolumeCompatible(engine, volumeMajor, requested string) bool {
	m, ok := volumeMarkers[engine]
	if !ok || volumeMajor == "" {
		return true
	}
	want := majorOf(requested, m.parts)
	if want == "" {
		return true
	}
	a, b := strings.Split(volumeMajor, "."), strings.Split(want, ".")
	n := min(len(a), len(b))
	return strings.Join(a[:n], ".") == strings.Join(b[:n], ".")
}

// IncompatibleVolumeMessage names both majors and the three safe ways out.
func IncompatibleVolumeMessage(volume, volumeMajor, requested string) string {
	return fmt.Sprintf("Volume %s was initialised by major version %s, but this database asks for %s, which cannot open it. "+
		"Create the database with version %s, use a different name, or discard the leftover volume.",
		volume, volumeMajor, requested, volumeMajor)
}
