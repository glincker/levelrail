package dbviewer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// Column is one table column.
type Column struct {
	Name       string `json:"name"`
	Type       string `json:"type"`
	Nullable   bool   `json:"nullable"`
	Default    string `json:"default,omitempty"`
	PrimaryKey bool   `json:"primary_key"`
}

// Index is one table index.
type Index struct {
	Name       string `json:"name"`
	Definition string `json:"definition"`
	Unique     bool   `json:"unique"`
	Primary    bool   `json:"primary"`
}

// Table is a table, view, or other relation.
type Table struct {
	Name        string   `json:"name"`
	Kind        string   `json:"kind"`
	RowEstimate int64    `json:"row_estimate"`
	SizeBytes   int64    `json:"size_bytes"`
	Columns     []Column `json:"columns"`
	Indexes     []Index  `json:"indexes"`
}

// SchemaNode groups tables under one schema (a MySQL database).
type SchemaNode struct {
	Name   string  `json:"name"`
	Tables []Table `json:"tables"`
}

// ErrNotFound reports a table or column the database does not have.
var ErrNotFound = errors.New("not found")

const pgSystemFilter = `n.nspname NOT IN ('pg_catalog','information_schema') AND n.nspname NOT LIKE 'pg_toast%' AND n.nspname NOT LIKE 'pg_temp%'`

const pgEmptyJSONArray = `'[]'::json`

func pgSchemaSQL(limit int, outline bool) string {
	cols, idx := pgSchemaColumnsSQL, pgSchemaIndexesSQL
	if outline {
		cols, idx = pgEmptyJSONArray, pgEmptyJSONArray
	}
	return `SELECT COALESCE(json_agg(t ORDER BY t.schema, t.name), '[]'::json) FROM (
SELECT n.nspname AS schema, c.relname AS name,
  CASE c.relkind WHEN 'r' THEN 'table' WHEN 'p' THEN 'table' WHEN 'v' THEN 'view' WHEN 'm' THEN 'materialized_view' ELSE 'foreign_table' END AS kind,
  GREATEST(c.reltuples, 0)::bigint AS row_estimate,
  CASE WHEN c.relkind IN ('r','p','m') THEN pg_total_relation_size(c.oid) ELSE 0 END::bigint AS size_bytes,
  ` + cols + ` AS columns,
  ` + idx + ` AS indexes
FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace
WHERE c.relkind IN ('r','p','v','m','f') AND ` + pgSystemFilter + `
ORDER BY n.nspname, c.relname LIMIT ` + strconv.Itoa(limit) + `) t`
}

const pgSchemaColumnsSQL = `(SELECT COALESCE(json_agg(json_build_object('name', a.attname, 'type', format_type(a.atttypid, a.atttypmod),
      'nullable', NOT a.attnotnull, 'default', COALESCE(pg_get_expr(d.adbin, d.adrelid), ''),
      'primary_key', EXISTS (SELECT 1 FROM pg_index i WHERE i.indrelid = c.oid AND i.indisprimary AND a.attnum = ANY (i.indkey))) ORDER BY a.attnum), '[]'::json)
   FROM pg_attribute a LEFT JOIN pg_attrdef d ON d.adrelid = a.attrelid AND d.adnum = a.attnum
   WHERE a.attrelid = c.oid AND a.attnum > 0 AND NOT a.attisdropped)`

const pgSchemaIndexesSQL = `(SELECT COALESCE(json_agg(json_build_object('name', ic.relname, 'definition', pg_get_indexdef(i.indexrelid),
      'unique', i.indisunique, 'primary', i.indisprimary) ORDER BY ic.relname), '[]'::json)
   FROM pg_index i JOIN pg_class ic ON ic.oid = i.indexrelid WHERE i.indrelid = c.oid)`

type pgSchemaRow struct {
	Schema      string   `json:"schema"`
	Name        string   `json:"name"`
	Kind        string   `json:"kind"`
	RowEstimate int64    `json:"row_estimate"`
	SizeBytes   int64    `json:"size_bytes"`
	Columns     []Column `json:"columns"`
	Indexes     []Index  `json:"indexes"`
}

// Schema lists every user schema with its tables, columns, and indexes.
func (t Target) Schema(ctx context.Context) ([]SchemaNode, error) {
	if t.Dialect == DialectPostgres {
		return t.postgresSchema(ctx, false)
	}
	return t.mysqlSchema(ctx, false)
}

// SchemaOutline lists every user schema with its tables, row estimates and
// sizes but no columns or indexes, so a database with thousands of tables
// stays a small document. Per-table detail comes from TableStructure.
func (t Target) SchemaOutline(ctx context.Context) ([]SchemaNode, error) {
	if t.Dialect == DialectPostgres {
		return t.postgresSchema(ctx, true)
	}
	return t.mysqlSchema(ctx, true)
}

func (t Target) postgresSchema(ctx context.Context, outline bool) ([]SchemaNode, error) {
	res, err := t.RunTrusted(ctx, pgSchemaSQL(t.Limits.SchemaRows, outline))
	if err != nil {
		return nil, err
	}
	if len(res.Rows) == 0 || len(res.Rows[0]) == 0 || res.Rows[0][0] == nil {
		return []SchemaNode{}, nil
	}
	var rows []pgSchemaRow
	if err := json.Unmarshal([]byte(*res.Rows[0][0]), &rows); err != nil {
		return nil, fmt.Errorf("dbviewer: decode schema: %w", err)
	}
	nodes := map[string]*SchemaNode{}
	var order []string
	for _, r := range rows {
		n, ok := nodes[r.Schema]
		if !ok {
			n = &SchemaNode{Name: r.Schema, Tables: []Table{}}
			nodes[r.Schema] = n
			order = append(order, r.Schema)
		}
		n.Tables = append(n.Tables, Table{Name: r.Name, Kind: r.Kind, RowEstimate: r.RowEstimate, SizeBytes: r.SizeBytes, Columns: r.Columns, Indexes: r.Indexes})
	}
	out := make([]SchemaNode, 0, len(order))
	for _, k := range order {
		out = append(out, *nodes[k])
	}
	return out, nil
}

const mysqlSystemFilter = `table_schema NOT IN ('mysql','information_schema','performance_schema','sys')`

func (t Target) mysqlSchema(ctx context.Context, outline bool) ([]SchemaNode, error) {
	lim := strconv.Itoa(t.Limits.SchemaRows)
	tables, err := t.RunTrusted(ctx, `SELECT table_schema, table_name, table_type, COALESCE(table_rows,0), COALESCE(data_length,0)+COALESCE(index_length,0) FROM information_schema.tables WHERE `+mysqlSystemFilter+` ORDER BY table_schema, table_name LIMIT `+lim)
	if err != nil {
		return nil, err
	}
	if outline {
		return assembleMySQL(tables, Result{}, Result{}), nil
	}
	cols, err := t.RunTrusted(ctx, `SELECT table_schema, table_name, column_name, column_type, is_nullable, COALESCE(column_default,''), column_key FROM information_schema.columns WHERE `+mysqlSystemFilter+` ORDER BY table_schema, table_name, ordinal_position LIMIT `+strconv.Itoa(t.Limits.SchemaRows*20))
	if err != nil {
		return nil, err
	}
	idx, err := t.RunTrusted(ctx, `SELECT table_schema, table_name, index_name, MIN(non_unique), GROUP_CONCAT(column_name ORDER BY seq_in_index) FROM information_schema.statistics WHERE `+mysqlSystemFilter+` GROUP BY table_schema, table_name, index_name LIMIT `+lim)
	if err != nil {
		return nil, err
	}
	return assembleMySQL(tables, cols, idx), nil
}

func cell(row []*string, i int) string {
	if i < len(row) && row[i] != nil {
		return *row[i]
	}
	return ""
}

func assembleMySQL(tables, cols, idx Result) []SchemaNode {
	byKey := map[string]*Table{}
	schemas := map[string]*SchemaNode{}
	var order []string
	for _, r := range tables.Rows {
		s := cell(r, 0)
		n, ok := schemas[s]
		if !ok {
			n = &SchemaNode{Name: s, Tables: []Table{}}
			schemas[s] = n
			order = append(order, s)
		}
		kind := "table"
		if strings.Contains(strings.ToUpper(cell(r, 2)), "VIEW") {
			kind = "view"
		}
		rows, _ := strconv.ParseInt(cell(r, 3), 10, 64)
		size, _ := strconv.ParseInt(cell(r, 4), 10, 64)
		n.Tables = append(n.Tables, Table{Name: cell(r, 1), Kind: kind, RowEstimate: rows, SizeBytes: size, Columns: []Column{}, Indexes: []Index{}})
	}
	for _, n := range schemas {
		for i := range n.Tables {
			byKey[n.Name+"\x00"+n.Tables[i].Name] = &n.Tables[i]
		}
	}
	for _, r := range cols.Rows {
		if tb := byKey[cell(r, 0)+"\x00"+cell(r, 1)]; tb != nil {
			tb.Columns = append(tb.Columns, Column{
				Name: cell(r, 2), Type: cell(r, 3), Nullable: strings.EqualFold(cell(r, 4), "YES"),
				Default: cell(r, 5), PrimaryKey: cell(r, 6) == "PRI",
			})
		}
	}
	for _, r := range idx.Rows {
		if tb := byKey[cell(r, 0)+"\x00"+cell(r, 1)]; tb != nil {
			name := cell(r, 2)
			tb.Indexes = append(tb.Indexes, Index{
				Name: name, Definition: "(" + cell(r, 4) + ")",
				Unique: cell(r, 3) == "0", Primary: name == "PRIMARY",
			})
		}
	}
	out := make([]SchemaNode, 0, len(order))
	for _, k := range order {
		out = append(out, *schemas[k])
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}
