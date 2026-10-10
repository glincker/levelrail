package database

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/GLINCKER/levelrail/internal/store"
)

// PgvectorSuffix marks a Postgres version that runs the pgvector image,
// e.g. "17-pgvector" runs pgvector/pgvector:pg17. The engine id stays
// "postgres" so every Postgres code path applies unchanged.
const PgvectorSuffix = "-pgvector"

const (
	pgvectorImage    = "pgvector/pgvector"
	pgvectorMinMajor = 13
)

var pgvectorVersionPattern = regexp.MustCompile(`^([0-9]{2})-pgvector$`)

// ParsePgvectorVersion reports whether version selects the pgvector variant
// and its Postgres major. A string that mentions pgvector but is not exactly
// "<major>-pgvector" is an error; a plain version returns (0, false, nil).
func ParsePgvectorVersion(version string) (major int, isVariant bool, err error) {
	if !strings.Contains(strings.ToLower(version), "pgvector") {
		return 0, false, nil
	}
	m := pgvectorVersionPattern.FindStringSubmatch(version)
	if m == nil {
		return 0, true, fmt.Errorf("version %q is not a valid pgvector variant: use \"<major>-pgvector\", for example \"17-pgvector\"", version)
	}
	major, err = strconv.Atoi(m[1])
	if err != nil || major < pgvectorMinMajor {
		return 0, true, fmt.Errorf("version %q: pgvector images exist for Postgres %d and newer", version, pgvectorMinMajor)
	}
	return major, true, nil
}

// ValidateEngineVersion rejects variant strings that cannot be mapped to an
// image. Versions without the variant marker are not constrained here.
func ValidateEngineVersion(engine, version string) error {
	_, isVariant, err := ParsePgvectorVersion(version)
	if err != nil {
		return err
	}
	if isVariant && engine != store.EnginePostgres {
		return fmt.Errorf("the pgvector variant is only available for the postgres engine, not %q", engine)
	}
	return nil
}

// ImageTag returns the tag part of the image a database of engine and
// version runs.
func ImageTag(engine, version string) string {
	if engine == store.EnginePostgres {
		if major, ok, err := ParsePgvectorVersion(version); ok && err == nil {
			return "pg" + strconv.Itoa(major)
		}
	}
	return versionOrDefault(version)
}

func imageRepo(engine, version string) string {
	if engine == store.EnginePostgres {
		if _, ok, err := ParsePgvectorVersion(version); ok && err == nil {
			return pgvectorImage
		}
	}
	return dockerImageFor(engine)
}
