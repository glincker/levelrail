package dbviewer

import (
	"context"
	"fmt"
	"strings"
)

// Filter operators accepted by a table page.
const (
	OpContains = "contains"
	OpEquals   = "equals"
	OpIsNull   = "is_null"
	OpNotNull  = "not_null"
)

// PageQuery asks for one page of a table.
type PageQuery struct {
	Schema, Table string
	Limit, Offset int
	SortColumn    string
	SortDesc      bool
	FilterColumn  string
	FilterOp      string
	FilterValue   string
}

// Page is one page of rows. HasMore is true when rows exist past this page.
type Page struct {
	Result
	HasMore bool `json:"has_more"`
}

func validIdent(s string) bool {
	if s == "" || len(s) > 255 {
		return false
	}
	for _, r := range s {
		if r < 0x20 || r == '\\' || r == 0x7f {
			return false
		}
	}
	return true
}

func (t Target) quoteIdent(s string) string {
	if t.Dialect == DialectPostgres {
		return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
	}
	return "`" + strings.ReplaceAll(s, "`", "``") + "`"
}

func quoteLiteral(s string) string { return "'" + strings.ReplaceAll(s, "'", "''") + "'" }

// tableColumns returns the table's column names, or ErrNotFound.
func (t Target) tableColumns(ctx context.Context, schema, table string) ([]string, error) {
	var sql string
	if t.Dialect == DialectPostgres {
		sql = `SELECT a.attname FROM pg_attribute a JOIN pg_class c ON c.oid = a.attrelid JOIN pg_namespace n ON n.oid = c.relnamespace ` +
			`WHERE n.nspname = ` + quoteLiteral(schema) + ` AND c.relname = ` + quoteLiteral(table) +
			` AND c.relkind IN ('r','p','v','m','f') AND a.attnum > 0 AND NOT a.attisdropped ORDER BY a.attnum`
	} else {
		sql = `SELECT column_name FROM information_schema.columns WHERE table_schema = ` + quoteLiteral(schema) +
			` AND table_name = ` + quoteLiteral(table) + ` ORDER BY ordinal_position`
	}
	res, err := t.RunTrusted(ctx, sql)
	if err != nil {
		return nil, err
	}
	cols := make([]string, 0, len(res.Rows))
	for _, r := range res.Rows {
		cols = append(cols, cell(r, 0))
	}
	if len(cols) == 0 {
		return nil, ErrNotFound
	}
	return cols, nil
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// TablePage reads one page. Schema, table, and column names are checked
// against the database's own catalog before they are quoted into SQL.
func (t Target) TablePage(ctx context.Context, q PageQuery) (Page, error) {
	if !validIdent(q.Schema) || !validIdent(q.Table) {
		return Page{}, ErrNotFound
	}
	cols, err := t.tableColumns(ctx, q.Schema, q.Table)
	if err != nil {
		return Page{}, err
	}
	limit := q.Limit
	if maxPage := max(1, t.Limits.MaxRows-1); limit <= 0 || limit > maxPage {
		limit = min(100, maxPage)
	}
	if q.Offset < 0 {
		q.Offset = 0
	}

	var b strings.Builder
	fmt.Fprintf(&b, "SELECT * FROM %s.%s", t.quoteIdent(q.Schema), t.quoteIdent(q.Table))
	if q.FilterColumn != "" {
		if !contains(cols, q.FilterColumn) {
			return Page{}, fmt.Errorf("%w: column %q", ErrNotFound, q.FilterColumn)
		}
		clause, err := t.filterClause(q)
		if err != nil {
			return Page{}, err
		}
		b.WriteString(" WHERE " + clause)
	}
	if q.SortColumn != "" {
		if !contains(cols, q.SortColumn) {
			return Page{}, fmt.Errorf("%w: column %q", ErrNotFound, q.SortColumn)
		}
		dir := "ASC"
		if q.SortDesc {
			dir = "DESC"
		}
		fmt.Fprintf(&b, " ORDER BY %s %s", t.quoteIdent(q.SortColumn), dir)
	}
	fmt.Fprintf(&b, " LIMIT %d OFFSET %d", limit+1, q.Offset)

	res, err := t.RunTrusted(ctx, b.String())
	if err != nil {
		return Page{}, err
	}
	p := Page{Result: res}
	if len(res.Rows) > limit {
		p.HasMore = true
		p.Rows = res.Rows[:limit]
		p.RowCount = limit
	}
	return p, nil
}

func (t Target) filterClause(q PageQuery) (string, error) {
	col := t.quoteIdent(q.FilterColumn)
	switch q.FilterOp {
	case OpIsNull:
		return col + " IS NULL", nil
	case OpNotNull:
		return col + " IS NOT NULL", nil
	case OpContains, OpEquals, "":
	default:
		return "", fmt.Errorf("%w: unknown filter operator %q", ErrNotAllowed, q.FilterOp)
	}
	if strings.ContainsAny(q.FilterValue, "\\\x00") {
		return "", ErrBackslash
	}
	text := "CAST(" + col + " AS TEXT)"
	if t.Dialect != DialectPostgres {
		text = "CAST(" + col + " AS CHAR)"
	}
	if q.FilterOp == OpEquals {
		return text + " = " + quoteLiteral(q.FilterValue), nil
	}
	esc := strings.NewReplacer("!", "!!", "%", "!%", "_", "!_").Replace(q.FilterValue)
	like := "LIKE"
	if t.Dialect == DialectPostgres {
		like = "ILIKE"
	}
	return text + " " + like + " " + quoteLiteral("%"+esc+"%") + " ESCAPE '!'", nil
}
