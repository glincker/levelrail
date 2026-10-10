package datamigrate

import (
	"fmt"
	"sort"
	"strings"
)

// PgvectorVariantAvailable gates the "<major>-pgvector" managed Postgres
// version, which the database reconciler maps to the pgvector image.
const PgvectorVariantAvailable = true

// PgvectorVersionSuffix is appended to a Postgres major to select the variant.
const PgvectorVersionSuffix = "-pgvector"

// PgvectorMinMajor is the oldest Postgres major the pgvector images exist for.
const PgvectorMinMajor = 13

var stockExtensions = toSet(
	"plpgsql", "adminpack", "amcheck", "autoinc", "bloom", "btree_gin", "btree_gist", "citext", "cube",
	"dblink", "dict_int", "dict_xsyn", "earthdistance", "file_fdw", "fuzzystrmatch", "hstore",
	"insert_username", "intagg", "intarray", "isn", "lo", "ltree", "moddatetime", "old_snapshot",
	"pageinspect", "pg_buffercache", "pg_freespacemap", "pg_prewarm", "pg_stat_statements", "pg_surgery",
	"pg_trgm", "pg_visibility", "pg_walinspect", "pgcrypto", "pgrowlocks", "pgstattuple", "postgres_fdw",
	"refint", "seg", "sslinfo", "tablefunc", "tcn", "tsm_system_rows", "tsm_system_time", "unaccent",
	"uuid-ossp", "xml2",
)

var pgvectorExtensions = toSet("vector")

func toSet(items ...string) map[string]bool {
	m := make(map[string]bool, len(items))
	for _, s := range items {
		m[s] = true
	}
	return m
}

// requiredVariant maps extensions to the version suffix of the managed image
// that provides all of them: "" for the stock image, PgvectorVersionSuffix for
// pgvector. Anything no managed image ships comes back in unsupported, never
// dropped silently.
func requiredVariant(extensions []string) (suffix string, unsupported []string) {
	for _, e := range extensions {
		name := strings.ToLower(strings.TrimSpace(e))
		switch {
		case stockExtensions[name]:
		case pgvectorExtensions[name]:
			suffix = PgvectorVersionSuffix
		default:
			unsupported = append(unsupported, e)
		}
	}
	sort.Strings(unsupported)
	return suffix, unsupported
}

// variantVersion composes the managed database version for a Postgres major.
func variantVersion(major int, suffix string) string {
	return fmt.Sprintf("%d%s", major, suffix)
}
