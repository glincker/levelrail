package dbschema

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"

	"github.com/GLINCKER/levelrail/internal/dbviewer"
)

// ErrUnsupported reports an engine the reader has no catalog queries for.
var ErrUnsupported = errors.New("schema diff supports Postgres only")

const systemFilter = `n.nspname NOT IN ('pg_catalog','information_schema') AND n.nspname NOT LIKE 'pg_toast%' AND n.nspname NOT LIKE 'pg_temp%'`

// chunk keeps every result cell small however many tables there are.
const chunk = 25

// Both queries are fixed text over the catalogs; only integer caps are interpolated.
func relationsSQL(limit int) string {
	return `WITH sp AS MATERIALIZED (SELECT set_config('search_path', 'pg_catalog', true))
SELECT json_agg(t ORDER BY t.schema, t.name) FROM (
SELECT (row_number() OVER (ORDER BY n.nspname, c.relname) - 1) / ` + strconv.Itoa(chunk) + ` AS chunk,
  n.nspname AS schema, c.relname AS name, c.relkind::text AS kind,
  CASE WHEN c.relkind IN ('r','p') THEN (SELECT COALESCE(json_agg(json_build_object('name', a.attname, 'type', format_type(a.atttypid, a.atttypmod),
      'nullable', NOT a.attnotnull, 'default', COALESCE(pg_get_expr(d.adbin, d.adrelid), '')) ORDER BY a.attnum), '[]'::json)
    FROM pg_attribute a LEFT JOIN pg_attrdef d ON d.adrelid = a.attrelid AND d.adnum = a.attnum
    WHERE a.attrelid = c.oid AND a.attnum > 0 AND NOT a.attisdropped) ELSE '[]'::json END AS columns,
  CASE WHEN c.relkind IN ('r','p') THEN (SELECT COALESCE(json_agg(json_build_object('name', ic.relname, 'definition', pg_get_indexdef(i.indexrelid)) ORDER BY ic.relname), '[]'::json)
    FROM pg_index i JOIN pg_class ic ON ic.oid = i.indexrelid
    WHERE i.indrelid = c.oid AND NOT EXISTS (SELECT 1 FROM pg_constraint x WHERE x.conindid = i.indexrelid)) ELSE '[]'::json END AS indexes,
  CASE WHEN c.relkind IN ('r','p') THEN (SELECT COALESCE(json_agg(json_build_object('name', con.conname, 'type', con.contype::text,
      'definition', pg_get_constraintdef(con.oid)) ORDER BY con.conname), '[]'::json)
    FROM pg_constraint con WHERE con.conrelid = c.oid AND con.contype IN ('p','u','c','f')) ELSE '[]'::json END AS constraints,
  CASE WHEN c.relkind IN ('v','m') THEN pg_get_viewdef(c.oid, true) ELSE '' END AS view_definition
FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace
WHERE EXISTS (SELECT 1 FROM sp) AND c.relkind IN ('r','p','v','m') AND ` + systemFilter + `
ORDER BY n.nspname, c.relname LIMIT ` + strconv.Itoa(limit) + `) t GROUP BY t.chunk ORDER BY t.chunk`
}

func sequencesSQL(limit int) string {
	return `SELECT COALESCE(json_agg(json_build_object('schema', n.nspname, 'name', c.relname, 'type', s.seqtypid::regtype::text,
  'start', s.seqstart, 'increment', s.seqincrement, 'min', s.seqmin, 'max', s.seqmax, 'cycle', s.seqcycle) ORDER BY n.nspname, c.relname), '[]'::json)
FROM (SELECT * FROM pg_sequence LIMIT ` + strconv.Itoa(limit) + `) s JOIN pg_class c ON c.oid = s.seqrelid JOIN pg_namespace n ON n.oid = c.relnamespace
WHERE ` + systemFilter + ` AND NOT EXISTS (SELECT 1 FROM pg_depend d WHERE d.objid = c.oid AND d.deptype = 'i')`
}

func schemasSQL(limit int) string {
	return `SELECT COALESCE(json_agg(n.nspname ORDER BY n.nspname), '[]'::json) FROM (SELECT * FROM pg_namespace n WHERE ` + systemFilter + ` ORDER BY n.nspname LIMIT ` + strconv.Itoa(limit) + `) n`
}

type relationRow struct {
	Schema         string       `json:"schema"`
	Name           string       `json:"name"`
	Kind           string       `json:"kind"`
	Columns        []Column     `json:"columns"`
	Indexes        []Index      `json:"indexes"`
	Constraints    []Constraint `json:"constraints"`
	ViewDefinition string       `json:"view_definition"`
}

// Read takes a snapshot of one Postgres database through the viewer's read-only exec path. At most maxObjects
// tables and views are read; the snapshot says when it was cut. Credentials stay inside the container.
func Read(ctx context.Context, t dbviewer.Target, maxObjects int) (Snapshot, error) {
	if t.Dialect != dbviewer.DialectPostgres {
		return Snapshot{}, ErrUnsupported
	}
	if maxObjects < 1 {
		maxObjects = 1
	}
	res, err := t.RunTrusted(ctx, relationsSQL(maxObjects+1))
	if err != nil {
		return Snapshot{}, err
	}
	var rows []relationRow
	for _, r := range res.Rows {
		if len(r) == 0 || r[0] == nil {
			continue
		}
		var part []relationRow
		if err := json.Unmarshal([]byte(*r[0]), &part); err != nil {
			return Snapshot{}, fmt.Errorf("dbschema: decode relations: %w", err)
		}
		rows = append(rows, part...)
	}
	snap := Snapshot{Tables: []Table{}, Views: []View{}, Sequences: []Sequence{}}
	if len(rows) > maxObjects {
		rows = rows[:maxObjects]
		snap.Truncated = true
	}
	for _, r := range rows {
		if r.Kind == "v" || r.Kind == "m" {
			snap.Views = append(snap.Views, View{Schema: r.Schema, Name: r.Name, Materialized: r.Kind == "m", Definition: r.ViewDefinition})
			continue
		}
		snap.Tables = append(snap.Tables, Table{Schema: r.Schema, Name: r.Name, Columns: r.Columns, Indexes: r.Indexes, Constraints: r.Constraints})
	}
	seq, err := t.RunTrusted(ctx, sequencesSQL(maxObjects))
	if err != nil {
		return Snapshot{}, err
	}
	if len(seq.Rows) > 0 && len(seq.Rows[0]) > 0 && seq.Rows[0][0] != nil {
		if err := json.Unmarshal([]byte(*seq.Rows[0][0]), &snap.Sequences); err != nil {
			return Snapshot{}, fmt.Errorf("dbschema: decode sequences: %w", err)
		}
	}
	sch, err := t.RunTrusted(ctx, schemasSQL(maxObjects))
	if err != nil {
		return Snapshot{}, err
	}
	snap.Schemas = []string{}
	if len(sch.Rows) > 0 && len(sch.Rows[0]) > 0 && sch.Rows[0][0] != nil {
		if err := json.Unmarshal([]byte(*sch.Rows[0][0]), &snap.Schemas); err != nil {
			return Snapshot{}, fmt.Errorf("dbschema: decode schemas: %w", err)
		}
	}
	return snap, nil
}
