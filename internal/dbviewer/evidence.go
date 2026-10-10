package dbviewer

import (
	"context"
	"strconv"
)

// Evidence is what the catalog says about a database independent of the
// schema listing, so an empty listing can be checked against it.
type Evidence struct {
	DatabaseSizeBytes int64    `json:"database_size_bytes"`
	SchemasScanned    int      `json:"schemas_scanned"`
	UserTables        int64    `json:"user_tables"`
	ExcludedSchemas   []string `json:"excluded_schemas"`
}

var (
	pgExcludedSchemas    = []string{"pg_catalog", "information_schema", "pg_toast", "pg_temp_*"}
	mysqlExcludedSchemas = []string{"mysql", "information_schema", "performance_schema", "sys"}
)

// Evidence counts schemas, tables and bytes straight from the catalog.
func (t Target) Evidence(ctx context.Context) (Evidence, error) {
	var sql string
	ev := Evidence{ExcludedSchemas: mysqlExcludedSchemas}
	if t.Dialect == DialectPostgres {
		ev.ExcludedSchemas = pgExcludedSchemas
		sql = `SELECT pg_database_size(current_database()),
 (SELECT count(*) FROM pg_namespace n WHERE ` + pgSystemFilter + `),
 (SELECT count(*) FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace WHERE c.relkind IN ('r','p','v','m','f') AND ` + pgSystemFilter + `)`
	} else {
		sql = `SELECT COALESCE(SUM(COALESCE(data_length,0)+COALESCE(index_length,0)),0),
 (SELECT COUNT(*) FROM information_schema.schemata WHERE ` + mysqlSchemaFilter + `),
 COUNT(*) FROM information_schema.tables WHERE ` + mysqlSystemFilter
	}
	res, err := t.RunTrusted(ctx, sql)
	if err != nil {
		return ev, err
	}
	if len(res.Rows) == 0 {
		return ev, nil
	}
	row := res.Rows[0]
	ev.DatabaseSizeBytes, _ = strconv.ParseInt(cell(row, 0), 10, 64)
	schemas, _ := strconv.Atoi(cell(row, 1))
	ev.SchemasScanned = schemas
	ev.UserTables, _ = strconv.ParseInt(cell(row, 2), 10, 64)
	return ev, nil
}

const mysqlSchemaFilter = `schema_name NOT IN ('mysql','information_schema','performance_schema','sys')`
