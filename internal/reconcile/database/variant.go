package database

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/GLINCKER/levelrail/internal/store"
)

// Suffixes that mark a Postgres version running an extension image, for
// example "17-pgvector" runs pgvector/pgvector:pg17. The engine id stays
// "postgres" so every Postgres code path (dump, restore, health) applies
// unchanged.
const (
	PgvectorSuffix    = "-pgvector"
	TimescaleDBSuffix = "-timescaledb"
	PostGISSuffix     = "-postgis"
)

// PostgresVariant describes one allowlisted Postgres extension image.
type PostgresVariant struct {
	Name     string
	Suffix   string
	Image    string
	MinMajor int
	tag      func(major int) string
}

var postgresVariants = []PostgresVariant{
	{Name: "pgvector", Suffix: PgvectorSuffix, Image: "pgvector/pgvector", MinMajor: 13,
		tag: func(m int) string { return "pg" + strconv.Itoa(m) }},
	{Name: "timescaledb", Suffix: TimescaleDBSuffix, Image: "timescale/timescaledb", MinMajor: 15,
		tag: func(m int) string { return "latest-pg" + strconv.Itoa(m) }},
	{Name: "postgis", Suffix: PostGISSuffix, Image: "postgis/postgis", MinMajor: 13,
		tag: postgisTag},
}

// postgisTag pins the PostGIS release published for each Postgres major:
// 18 only has 3.6, older majors are published with 3.5.
func postgisTag(major int) string {
	if major >= 18 {
		return strconv.Itoa(major) + "-3.6"
	}
	return strconv.Itoa(major) + "-3.5"
}

// PostgresVariants lists the allowlisted extension images, in picker order.
func PostgresVariants() []PostgresVariant {
	return append([]PostgresVariant(nil), postgresVariants...)
}

var variantVersionPattern = regexp.MustCompile(`^([0-9]{2})-([a-z]+)$`)

// ParsePostgresVariant reports whether version selects an extension image
// and returns its Postgres major and variant name. A string that mentions a
// variant but is not exactly "<major>-<variant>" is an error; a plain
// version returns (0, "", nil).
func ParsePostgresVariant(version string) (major int, variant string, err error) {
	lower := strings.ToLower(version)
	mentioned := ""
	for _, v := range postgresVariants {
		if strings.Contains(lower, v.Name) {
			mentioned = v.Name
			break
		}
	}
	if mentioned == "" {
		return 0, "", nil
	}
	if m := variantVersionPattern.FindStringSubmatch(version); m != nil {
		for _, v := range postgresVariants {
			if v.Name != m[2] {
				continue
			}
			n, convErr := strconv.Atoi(m[1])
			if convErr != nil || n < v.MinMajor {
				return 0, v.Name, fmt.Errorf("version %q: %s images exist for Postgres %d and newer", version, v.Name, v.MinMajor)
			}
			return n, v.Name, nil
		}
	}
	return 0, lower, fmt.Errorf("version %q is not a valid %s variant: use \"<major>-%s\", for example \"17-%s\"", version, mentioned, mentioned, mentioned)
}

// ParsePgvectorVersion reports whether version selects the pgvector variant
// and its Postgres major. Other variants return (0, false, nil).
func ParsePgvectorVersion(version string) (major int, isVariant bool, err error) {
	major, variant, err := ParsePostgresVariant(version)
	if err != nil && strings.Contains(strings.ToLower(version), "pgvector") {
		return 0, true, err
	}
	if variant != "pgvector" {
		return 0, false, nil
	}
	return major, true, nil
}

// ValidateEngineVersion rejects variant strings that cannot be mapped to an
// image. Versions without a variant marker are not constrained here.
func ValidateEngineVersion(engine, version string) error {
	_, variant, err := ParsePostgresVariant(version)
	if err != nil {
		return err
	}
	if variant != "" && engine != store.EnginePostgres {
		return fmt.Errorf("the %s variant is only available for the postgres engine, not %q", variant, engine)
	}
	return validatePinnedVersion(engine, version)
}

func lookupVariant(engine, version string) (PostgresVariant, int, bool) {
	if engine != store.EnginePostgres {
		return PostgresVariant{}, 0, false
	}
	major, name, err := ParsePostgresVariant(version)
	if err != nil || name == "" {
		return PostgresVariant{}, 0, false
	}
	for _, v := range postgresVariants {
		if v.Name == name {
			return v, major, true
		}
	}
	return PostgresVariant{}, 0, false
}

// ImageTag returns the tag part of the image a database of engine and
// version runs.
func ImageTag(engine, version string) string {
	if v, major, ok := lookupVariant(engine, version); ok {
		return v.tag(major)
	}
	return versionOrDefault(version)
}

func imageRepo(engine, version string) string {
	if v, _, ok := lookupVariant(engine, version); ok {
		return v.Image
	}
	return dockerImageFor(engine)
}
