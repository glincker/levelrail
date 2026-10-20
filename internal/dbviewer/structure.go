package dbviewer

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// ForeignKey is one foreign key constraint. Schema and Table name the table
// that owns it; RefSchema and RefTable the table it points at.
type ForeignKey struct {
	Name       string   `json:"name"`
	Schema     string   `json:"schema"`
	Table      string   `json:"table"`
	Columns    []string `json:"columns"`
	RefSchema  string   `json:"ref_schema"`
	RefTable   string   `json:"ref_table"`
	RefColumns []string `json:"ref_columns"`
}

// Structure is the full detail of one table, what the schema outline leaves
// out. ForeignKeys point away from the table, ReferencedBy point at it.
type Structure struct {
	Schema       string       `json:"schema"`
	Name         string       `json:"name"`
	Kind         string       `json:"kind"`
	RowEstimate  int64        `json:"row_estimate"`
	SizeBytes    int64        `json:"size_bytes"`
	Columns      []Column     `json:"columns"`
	Indexes      []Index      `json:"indexes"`
	ForeignKeys  []ForeignKey `json:"foreign_keys"`
	ReferencedBy []ForeignKey `json:"referenced_by"`
	DDL          string       `json:"ddl"`
}

type pgConstraint struct {
	Name       string `json:"name"`
	Type       string `json:"type"`
	Definition string `json:"definition"`
}

type pgIndexDetail struct {
	Index
	Constraint bool `json:"constraint"`
}

type pgStructureRow struct {
	Kind           string          `json:"kind"`
	RowEstimate    int64           `json:"row_estimate"`
	SizeBytes      int64           `json:"size_bytes"`
	Columns        []Column        `json:"columns"`
	Indexes        []pgIndexDetail `json:"indexes"`
	ForeignKeys    []ForeignKey    `json:"foreign_keys"`
	ReferencedBy   []ForeignKey    `json:"referenced_by"`
	Constraints    []pgConstraint  `json:"constraints"`
	ViewDefinition string          `json:"view_definition"`
}

const pgForeignKeySQL = `SELECT con.conname AS name, cn.nspname AS schema, cc.relname AS "table",
 (SELECT json_agg(a.attname ORDER BY k.ord) FROM unnest(con.conkey) WITH ORDINALITY AS k(attnum, ord)
    JOIN pg_attribute a ON a.attrelid = con.conrelid AND a.attnum = k.attnum) AS columns,
 rn.nspname AS ref_schema, rc.relname AS ref_table,
 (SELECT json_agg(a.attname ORDER BY k.ord) FROM unnest(con.confkey) WITH ORDINALITY AS k(attnum, ord)
    JOIN pg_attribute a ON a.attrelid = con.confrelid AND a.attnum = k.attnum) AS ref_columns
FROM pg_constraint con
JOIN pg_class cc ON cc.oid = con.conrelid JOIN pg_namespace cn ON cn.oid = cc.relnamespace
JOIN pg_class rc ON rc.oid = con.confrelid JOIN pg_namespace rn ON rn.oid = rc.relnamespace
WHERE con.contype = 'f' AND `

func pgStructureSQL(schema, table string) string {
	fk := func(cond string) string {
		return `(SELECT COALESCE(json_agg(f ORDER BY f.name), '[]'::json) FROM (` + pgForeignKeySQL + cond + `) f)`
	}
	return `SELECT json_build_object(
 'kind', CASE c.relkind WHEN 'r' THEN 'table' WHEN 'p' THEN 'table' WHEN 'v' THEN 'view' WHEN 'm' THEN 'materialized_view' ELSE 'foreign_table' END,
 'row_estimate', GREATEST(c.reltuples, 0)::bigint,
 'size_bytes', CASE WHEN c.relkind IN ('r','p','m') THEN pg_total_relation_size(c.oid) ELSE 0 END::bigint,
 'columns', ` + pgSchemaColumnsSQL + `,
 'indexes', (SELECT COALESCE(json_agg(json_build_object('name', ic.relname, 'definition', pg_get_indexdef(i.indexrelid),
      'unique', i.indisunique, 'primary', i.indisprimary,
      'constraint', EXISTS (SELECT 1 FROM pg_constraint x WHERE x.conindid = i.indexrelid)) ORDER BY ic.relname), '[]'::json)
   FROM pg_index i JOIN pg_class ic ON ic.oid = i.indexrelid WHERE i.indrelid = c.oid),
 'foreign_keys', ` + fk("con.conrelid = c.oid") + `,
 'referenced_by', ` + fk("con.confrelid = c.oid") + `,
 'constraints', (SELECT COALESCE(json_agg(json_build_object('name', con.conname, 'type', con.contype::text,
      'definition', pg_get_constraintdef(con.oid)) ORDER BY con.contype, con.conname), '[]'::json)
   FROM pg_constraint con WHERE con.conrelid = c.oid AND con.contype IN ('p','u','c','f')),
 'view_definition', CASE WHEN c.relkind IN ('v','m') THEN pg_get_viewdef(c.oid, true) ELSE '' END
) FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace
WHERE n.nspname = ` + quoteLiteral(schema) + ` AND c.relname = ` + quoteLiteral(table) + ` AND c.relkind IN ('r','p','v','m','f')`
}

// TableStructure returns the columns, indexes, foreign keys in both
// directions, and DDL of one table, or ErrNotFound.
func (t Target) TableStructure(ctx context.Context, schema, table string) (Structure, error) {
	if !validIdent(schema) || !validIdent(table) {
		return Structure{}, ErrNotFound
	}
	if t.Dialect == DialectPostgres {
		return t.postgresStructure(ctx, schema, table)
	}
	return t.mysqlStructure(ctx, schema, table)
}

func (t Target) postgresStructure(ctx context.Context, schema, table string) (Structure, error) {
	res, err := t.RunTrusted(ctx, pgStructureSQL(schema, table))
	if err != nil {
		return Structure{}, err
	}
	if len(res.Rows) == 0 || len(res.Rows[0]) == 0 || res.Rows[0][0] == nil {
		return Structure{}, ErrNotFound
	}
	var row pgStructureRow
	if err := json.Unmarshal([]byte(*res.Rows[0][0]), &row); err != nil {
		return Structure{}, fmt.Errorf("dbviewer: decode structure: %w", err)
	}
	out := Structure{
		Schema: schema, Name: table, Kind: row.Kind, RowEstimate: row.RowEstimate, SizeBytes: row.SizeBytes,
		Columns: nonNil(row.Columns), ForeignKeys: nonNil(row.ForeignKeys), ReferencedBy: nonNil(row.ReferencedBy),
		Indexes: make([]Index, 0, len(row.Indexes)),
	}
	standalone := []string{}
	for _, ix := range row.Indexes {
		out.Indexes = append(out.Indexes, ix.Index)
		if !ix.Constraint {
			standalone = append(standalone, ix.Definition)
		}
	}
	out.DDL = pgDDL(schema, table, row, standalone)
	return out, nil
}

func nonNil[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}

func pgQuote(s string) string { return `"` + strings.ReplaceAll(s, `"`, `""`) + `"` }

func pgDDL(schema, table string, row pgStructureRow, standaloneIndexes []string) string {
	full := pgQuote(schema) + "." + pgQuote(table)
	if row.Kind == "view" || row.Kind == "materialized_view" {
		kw := "VIEW"
		if row.Kind == "materialized_view" {
			kw = "MATERIALIZED VIEW"
		}
		return "CREATE " + kw + " " + full + " AS\n" + strings.TrimSpace(row.ViewDefinition)
	}
	lines := make([]string, 0, len(row.Columns)+len(row.Constraints))
	for _, c := range row.Columns {
		l := "  " + pgQuote(c.Name) + " " + c.Type
		if !c.Nullable {
			l += " NOT NULL"
		}
		if c.Default != "" {
			l += " DEFAULT " + c.Default
		}
		lines = append(lines, l)
	}
	for _, c := range row.Constraints {
		lines = append(lines, "  CONSTRAINT "+pgQuote(c.Name)+" "+c.Definition)
	}
	var b strings.Builder
	b.WriteString("CREATE TABLE " + full + " (\n" + strings.Join(lines, ",\n") + "\n);")
	for _, def := range standaloneIndexes {
		b.WriteString("\n" + def + ";")
	}
	return b.String()
}

func (t Target) mysqlStructure(ctx context.Context, schema, table string) (Structure, error) {
	s, n := quoteLiteral(schema), quoteLiteral(table)
	meta, err := t.RunTrusted(ctx, `SELECT table_type, COALESCE(table_rows,0), COALESCE(data_length,0)+COALESCE(index_length,0) FROM information_schema.tables WHERE table_schema = `+s+` AND table_name = `+n)
	if err != nil {
		return Structure{}, err
	}
	if len(meta.Rows) == 0 {
		return Structure{}, ErrNotFound
	}
	out := Structure{Schema: schema, Name: table, Kind: "table", Columns: []Column{}, Indexes: []Index{}, ForeignKeys: []ForeignKey{}, ReferencedBy: []ForeignKey{}}
	if strings.Contains(strings.ToUpper(cell(meta.Rows[0], 0)), "VIEW") {
		out.Kind = "view"
	}
	out.RowEstimate, _ = strconv.ParseInt(cell(meta.Rows[0], 1), 10, 64)
	out.SizeBytes, _ = strconv.ParseInt(cell(meta.Rows[0], 2), 10, 64)

	cols, err := t.RunTrusted(ctx, `SELECT column_name, column_type, is_nullable, COALESCE(column_default,''), column_key FROM information_schema.columns WHERE table_schema = `+s+` AND table_name = `+n+` ORDER BY ordinal_position`)
	if err != nil {
		return Structure{}, err
	}
	for _, r := range cols.Rows {
		out.Columns = append(out.Columns, Column{Name: cell(r, 0), Type: cell(r, 1), Nullable: strings.EqualFold(cell(r, 2), "YES"), Default: cell(r, 3), PrimaryKey: cell(r, 4) == "PRI"})
	}
	idx, err := t.RunTrusted(ctx, `SELECT index_name, MIN(non_unique), GROUP_CONCAT(column_name ORDER BY seq_in_index) FROM information_schema.statistics WHERE table_schema = `+s+` AND table_name = `+n+` GROUP BY index_name ORDER BY index_name`)
	if err != nil {
		return Structure{}, err
	}
	for _, r := range idx.Rows {
		name := cell(r, 0)
		out.Indexes = append(out.Indexes, Index{Name: name, Definition: "(" + cell(r, 2) + ")", Unique: cell(r, 1) == "0", Primary: name == "PRIMARY"})
	}
	if out.ForeignKeys, err = t.mysqlForeignKeys(ctx, "table_schema = "+s+" AND table_name = "+n); err != nil {
		return Structure{}, err
	}
	if out.ReferencedBy, err = t.mysqlForeignKeys(ctx, "referenced_table_schema = "+s+" AND referenced_table_name = "+n); err != nil {
		return Structure{}, err
	}
	kw := "TABLE"
	if out.Kind == "view" {
		kw = "VIEW"
	}
	ddl, err := t.RunTrusted(ctx, "SHOW CREATE "+kw+" "+t.quoteIdent(schema)+"."+t.quoteIdent(table))
	if err == nil && len(ddl.Rows) > 0 {
		out.DDL = cell(ddl.Rows[0], 1)
	}
	return out, nil
}

func (t Target) mysqlForeignKeys(ctx context.Context, cond string) ([]ForeignKey, error) {
	res, err := t.RunTrusted(ctx, `SELECT constraint_name, table_schema, table_name, column_name, referenced_table_schema, referenced_table_name, referenced_column_name FROM information_schema.key_column_usage WHERE referenced_table_name IS NOT NULL AND `+cond+` ORDER BY table_schema, table_name, constraint_name, ordinal_position`)
	if err != nil {
		return nil, err
	}
	out := []ForeignKey{}
	pos := map[string]int{}
	for _, r := range res.Rows {
		key := cell(r, 1) + "\x00" + cell(r, 2) + "\x00" + cell(r, 0)
		i, ok := pos[key]
		if !ok {
			i = len(out)
			pos[key] = i
			out = append(out, ForeignKey{Name: cell(r, 0), Schema: cell(r, 1), Table: cell(r, 2), RefSchema: cell(r, 4), RefTable: cell(r, 5), Columns: []string{}, RefColumns: []string{}})
		}
		out[i].Columns = append(out[i].Columns, cell(r, 3))
		out[i].RefColumns = append(out[i].RefColumns, cell(r, 6))
	}
	return out, nil
}
