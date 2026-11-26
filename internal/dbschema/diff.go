package dbschema

import "sort"

// Kinds of change and of object, as they appear in the API.
const (
	ChangeAdded   = "added"
	ChangeRemoved = "removed"
	ChangeChanged = "changed"

	ObjectTable      = "table"
	ObjectColumn     = "column"
	ObjectIndex      = "index"
	ObjectConstraint = "constraint"
	ObjectView       = "view"
	ObjectSequence   = "sequence"
)

// Change is one difference. Table is set for columns, indexes and constraints.
type Change struct {
	Object string `json:"object"`
	Kind   string `json:"kind"`
	Schema string `json:"schema"`
	Table  string `json:"table,omitempty"`
	Name   string `json:"name"`
	Before string `json:"before,omitempty"`
	After  string `json:"after,omitempty"`
}

// Summary counts the changes by kind.
type Summary struct {
	Added   int `json:"added"`
	Removed int `json:"removed"`
	Changed int `json:"changed"`
}

// Result is a diff with the forward migration that turns base into head.
type Result struct {
	Summary   Summary  `json:"summary"`
	Changes   []Change `json:"changes"`
	SQL       string   `json:"sql"`
	Truncated bool     `json:"truncated"`
}

// Diff compares base with head: added means head has it and base does not.
func Diff(base, head Snapshot) Result {
	d := differ{}
	d.tables(base.Tables, head.Tables)
	d.views(base.Views, head.Views)
	d.sequences(base.Sequences, head.Sequences)
	sort.SliceStable(d.changes, func(i, j int) bool {
		a, b := d.changes[i], d.changes[j]
		if a.Schema != b.Schema {
			return a.Schema < b.Schema
		}
		if a.Table != b.Table {
			return a.Table < b.Table
		}
		if a.Object != b.Object {
			return a.Object < b.Object
		}
		return a.Name < b.Name
	})
	r := Result{Changes: d.changes, Truncated: base.Truncated || head.Truncated}
	if r.Changes == nil {
		r.Changes = []Change{}
	}
	for _, c := range r.Changes {
		switch c.Kind {
		case ChangeAdded:
			r.Summary.Added++
		case ChangeRemoved:
			r.Summary.Removed++
		default:
			r.Summary.Changed++
		}
	}
	r.SQL = renderSQL(d)
	return r
}

type differ struct {
	changes []Change
	// Objects the SQL needs beyond the change list.
	addedTables, removedTables []Table
	addedViews, removedViews   []View
	changedViews               []View
	addedSeqs, removedSeqs     []Sequence
	changedSeqs                []Sequence
	tableOps                   []tableOp
}

type tableOp struct {
	schema, table string
	dropIndexes   []string
	addIndexes    []Index
	dropCons      []string
	addCons       []Constraint
	addCols       []Column
	dropCols      []string
	alterCols     []colAlter
}

type colAlter struct {
	name     string
	from, to Column
}

func key(schema, name string) string { return schema + "." + name }

func (d *differ) add(c Change) { d.changes = append(d.changes, c) }

func (d *differ) tables(base, head []Table) {
	bm, hm := map[string]Table{}, map[string]Table{}
	for _, t := range base {
		bm[key(t.Schema, t.Name)] = t
	}
	for _, t := range head {
		hm[key(t.Schema, t.Name)] = t
	}
	for _, t := range head {
		old, ok := bm[key(t.Schema, t.Name)]
		if !ok {
			d.addedTables = append(d.addedTables, t)
			d.add(Change{Object: ObjectTable, Kind: ChangeAdded, Schema: t.Schema, Name: t.Name})
			continue
		}
		d.tableInner(old, t)
	}
	for _, t := range base {
		if _, ok := hm[key(t.Schema, t.Name)]; !ok {
			d.removedTables = append(d.removedTables, t)
			d.add(Change{Object: ObjectTable, Kind: ChangeRemoved, Schema: t.Schema, Name: t.Name})
		}
	}
}

func (d *differ) tableInner(base, head Table) {
	op := tableOp{schema: head.Schema, table: head.Name}
	d.columns(&op, base, head)
	d.indexes(&op, base, head)
	d.constraints(&op, base, head)
	d.tableOps = append(d.tableOps, op)
}

func colSummary(c Column) string {
	s := c.Type
	if !c.Nullable {
		s += " not null"
	}
	if c.Default != "" {
		s += " default " + c.Default
	}
	return s
}

func (d *differ) columns(op *tableOp, base, head Table) {
	bm, hm := map[string]Column{}, map[string]Column{}
	for _, c := range base.Columns {
		bm[c.Name] = c
	}
	for _, c := range head.Columns {
		hm[c.Name] = c
	}
	for _, c := range head.Columns {
		old, ok := bm[c.Name]
		switch {
		case !ok:
			op.addCols = append(op.addCols, c)
			d.add(Change{Object: ObjectColumn, Kind: ChangeAdded, Schema: head.Schema, Table: head.Name, Name: c.Name, After: colSummary(c)})
		case old != c:
			op.alterCols = append(op.alterCols, colAlter{name: c.Name, from: old, to: c})
			d.add(Change{Object: ObjectColumn, Kind: ChangeChanged, Schema: head.Schema, Table: head.Name, Name: c.Name, Before: colSummary(old), After: colSummary(c)})
		}
	}
	for _, c := range base.Columns {
		if _, ok := hm[c.Name]; !ok {
			op.dropCols = append(op.dropCols, c.Name)
			d.add(Change{Object: ObjectColumn, Kind: ChangeRemoved, Schema: head.Schema, Table: head.Name, Name: c.Name, Before: colSummary(c)})
		}
	}
}

func (d *differ) indexes(op *tableOp, base, head Table) {
	bm, hm := map[string]Index{}, map[string]Index{}
	for _, i := range base.Indexes {
		bm[i.Name] = i
	}
	for _, i := range head.Indexes {
		hm[i.Name] = i
	}
	for _, i := range head.Indexes {
		old, ok := bm[i.Name]
		switch {
		case !ok:
			op.addIndexes = append(op.addIndexes, i)
			d.add(Change{Object: ObjectIndex, Kind: ChangeAdded, Schema: head.Schema, Table: head.Name, Name: i.Name, After: i.Definition})
		case old.Definition != i.Definition:
			op.dropIndexes = append(op.dropIndexes, i.Name)
			op.addIndexes = append(op.addIndexes, i)
			d.add(Change{Object: ObjectIndex, Kind: ChangeChanged, Schema: head.Schema, Table: head.Name, Name: i.Name, Before: old.Definition, After: i.Definition})
		}
	}
	for _, i := range base.Indexes {
		if _, ok := hm[i.Name]; !ok {
			op.dropIndexes = append(op.dropIndexes, i.Name)
			d.add(Change{Object: ObjectIndex, Kind: ChangeRemoved, Schema: head.Schema, Table: head.Name, Name: i.Name, Before: i.Definition})
		}
	}
}

func (d *differ) constraints(op *tableOp, base, head Table) {
	bm, hm := map[string]Constraint{}, map[string]Constraint{}
	for _, c := range base.Constraints {
		bm[c.Name] = c
	}
	for _, c := range head.Constraints {
		hm[c.Name] = c
	}
	for _, c := range head.Constraints {
		old, ok := bm[c.Name]
		switch {
		case !ok:
			op.addCons = append(op.addCons, c)
			d.add(Change{Object: ObjectConstraint, Kind: ChangeAdded, Schema: head.Schema, Table: head.Name, Name: c.Name, After: c.Definition})
		case old.Definition != c.Definition:
			op.dropCons = append(op.dropCons, c.Name)
			op.addCons = append(op.addCons, c)
			d.add(Change{Object: ObjectConstraint, Kind: ChangeChanged, Schema: head.Schema, Table: head.Name, Name: c.Name, Before: old.Definition, After: c.Definition})
		}
	}
	for _, c := range base.Constraints {
		if _, ok := hm[c.Name]; !ok {
			op.dropCons = append(op.dropCons, c.Name)
			d.add(Change{Object: ObjectConstraint, Kind: ChangeRemoved, Schema: head.Schema, Table: head.Name, Name: c.Name, Before: c.Definition})
		}
	}
}

func (d *differ) views(base, head []View) {
	bm, hm := map[string]View{}, map[string]View{}
	for _, v := range base {
		bm[key(v.Schema, v.Name)] = v
	}
	for _, v := range head {
		hm[key(v.Schema, v.Name)] = v
	}
	for _, v := range head {
		old, ok := bm[key(v.Schema, v.Name)]
		switch {
		case !ok:
			d.addedViews = append(d.addedViews, v)
			d.add(Change{Object: ObjectView, Kind: ChangeAdded, Schema: v.Schema, Name: v.Name, After: v.Definition})
		case old != v:
			d.changedViews = append(d.changedViews, v)
			d.add(Change{Object: ObjectView, Kind: ChangeChanged, Schema: v.Schema, Name: v.Name, Before: old.Definition, After: v.Definition})
		}
	}
	for _, v := range base {
		if _, ok := hm[key(v.Schema, v.Name)]; !ok {
			d.removedViews = append(d.removedViews, v)
			d.add(Change{Object: ObjectView, Kind: ChangeRemoved, Schema: v.Schema, Name: v.Name, Before: v.Definition})
		}
	}
}

func (d *differ) sequences(base, head []Sequence) {
	bm, hm := map[string]Sequence{}, map[string]Sequence{}
	for _, s := range base {
		bm[key(s.Schema, s.Name)] = s
	}
	for _, s := range head {
		hm[key(s.Schema, s.Name)] = s
	}
	for _, s := range head {
		old, ok := bm[key(s.Schema, s.Name)]
		switch {
		case !ok:
			d.addedSeqs = append(d.addedSeqs, s)
			d.add(Change{Object: ObjectSequence, Kind: ChangeAdded, Schema: s.Schema, Name: s.Name, After: seqSummary(s)})
		case old != s:
			d.changedSeqs = append(d.changedSeqs, s)
			d.add(Change{Object: ObjectSequence, Kind: ChangeChanged, Schema: s.Schema, Name: s.Name, Before: seqSummary(old), After: seqSummary(s)})
		}
	}
	for _, s := range base {
		if _, ok := hm[key(s.Schema, s.Name)]; !ok {
			d.removedSeqs = append(d.removedSeqs, s)
			d.add(Change{Object: ObjectSequence, Kind: ChangeRemoved, Schema: s.Schema, Name: s.Name, Before: seqSummary(s)})
		}
	}
}
