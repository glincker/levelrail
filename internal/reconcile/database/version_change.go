package database

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/GLINCKER/levelrail/internal/store"
)

// CheckInPlaceVersionChange decides whether swapping an engine's image tag
// over the same data volume is safe. It is the one rule behind both the
// set-version route and the automatic upgrade runner.
func CheckInPlaceVersionChange(engine, from, to string) error {
	if err := ValidateEngineVersion(engine, to); err != nil {
		return err
	}
	if to == from {
		return nil
	}
	_, fromVariant, _ := ParsePgvectorVersion(from)
	_, toVariant, _ := ParsePgvectorVersion(to)
	if fromVariant != toVariant {
		return errors.New("switching between the plain and pgvector images in place is refused: they are built on different Debian releases, and a glibc collation change can silently corrupt indexes. Use the guarded major upgrade (dump and restore) or restore a backup into a new database")
	}
	fromMajor, fromOK := LeadingMajor(from)
	toMajor, toOK := LeadingMajor(to)
	if !fromOK || !toOK {
		return fmt.Errorf("cannot compare %q with %q: use numeric versions, or restore a backup into a new database", from, to)
	}
	switch engine {
	case store.EngineRedis, store.EngineKeyDB, store.EngineDragonfly:
		if toMajor < fromMajor {
			return fmt.Errorf("downgrading %s from major %d to %d is not supported: the data files are not backward compatible", engine, fromMajor, toMajor)
		}
	default:
		if toMajor != fromMajor {
			return fmt.Errorf("changing %s major version %d to %d in place would corrupt the data directory: %s", engine, fromMajor, toMajor, majorChangeAdvice(engine))
		}
	}
	return nil
}

func majorChangeAdvice(engine string) string {
	if engine == store.EnginePostgres {
		return "use the guarded major upgrade (POST /api/v1/databases/{name}/major-upgrade, CLI: databases major-upgrade), or restore a backup into a new database on the new version"
	}
	return "take a backup and restore it into a new database on the new version"
}

// LeadingMajor parses the first numeric component of a version tag,
// accepting a "v" prefix (Dragonfly tags such as "v1.27.1").
func LeadingMajor(v string) (int, bool) {
	head := strings.TrimPrefix(v, "v")
	if i := strings.IndexAny(head, ".-"); i >= 0 {
		head = head[:i]
	}
	n, err := strconv.Atoi(head)
	return n, err == nil
}
