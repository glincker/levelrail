// Package dbschema computes a structured schema diff between two Postgres
// databases and renders a forward migration as text. It never runs the SQL.
package dbschema

// Column is one table column as the catalog reports it.
type Column struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Nullable bool   `json:"nullable"`
	Default  string `json:"default,omitempty"`
}

// Index is a standalone index; indexes that back a constraint are carried by the constraint.
type Index struct {
	Name       string `json:"name"`
	Definition string `json:"definition"`
}

// Constraint is a primary key, unique, check or foreign key constraint (types p, u, c, f).
type Constraint struct {
	Name       string `json:"name"`
	Type       string `json:"type"`
	Definition string `json:"definition"`
}

// Table is a base table with its columns, indexes and constraints.
type Table struct {
	Schema      string       `json:"schema"`
	Name        string       `json:"name"`
	Columns     []Column     `json:"columns"`
	Indexes     []Index      `json:"indexes"`
	Constraints []Constraint `json:"constraints"`
}

// View is a view or materialized view with its definition query.
type View struct {
	Schema       string `json:"schema"`
	Name         string `json:"name"`
	Materialized bool   `json:"materialized"`
	Definition   string `json:"definition"`
}

// Sequence is a standalone sequence; sequences owned by a column are left to the column.
type Sequence struct {
	Schema    string `json:"schema"`
	Name      string `json:"name"`
	Type      string `json:"type"`
	Start     int64  `json:"start"`
	Increment int64  `json:"increment"`
	Min       int64  `json:"min"`
	Max       int64  `json:"max"`
	Cycle     bool   `json:"cycle"`
}

// Snapshot is the schema of one database at one moment.
type Snapshot struct {
	Schemas   []string   `json:"schemas"`
	Tables    []Table    `json:"tables"`
	Views     []View     `json:"views"`
	Sequences []Sequence `json:"sequences"`
	// Truncated reports that the object cap cut the read short.
	Truncated bool `json:"truncated"`
}
