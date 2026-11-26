package dbschema

import (
	"fmt"
	"sort"
	"strings"
)

// Header marks the script as text for a human; this package never executes it.
const Header = "-- Generated schema migration. Review before running; Levelrail does not run it."

func quoteIdent(s string) string { return `"` + strings.ReplaceAll(s, `"`, `""`) + `"` }

func qual(schema, name string) string { return quoteIdent(schema) + "." + quoteIdent(name) }

func seqSummary(s Sequence) string {
	c := ""
	if s.Cycle {
		c = " cycle"
	}
	return fmt.Sprintf("%s start %d increment %d min %d max %d%s", s.Type, s.Start, s.Increment, s.Min, s.Max, c)
}

func columnDef(c Column) string {
	s := quoteIdent(c.Name) + " " + c.Type
	if c.Default != "" {
		s += " DEFAULT " + c.Default
	}
	if !c.Nullable {
		s += " NOT NULL"
	}
	return s
}

func seqCreate(s Sequence) string {
	cy := ""
	if s.Cycle {
		cy = " CYCLE"
	}
	return fmt.Sprintf("CREATE SEQUENCE %s AS %s INCREMENT BY %d MINVALUE %d MAXVALUE %d START WITH %d%s;",
		qual(s.Schema, s.Name), s.Type, s.Increment, s.Min, s.Max, s.Start, cy)
}

func seqAlter(s Sequence) string {
	cy := " NO CYCLE"
	if s.Cycle {
		cy = " CYCLE"
	}
	return fmt.Sprintf("ALTER SEQUENCE %s AS %s INCREMENT BY %d MINVALUE %d MAXVALUE %d%s;",
		qual(s.Schema, s.Name), s.Type, s.Increment, s.Min, s.Max, cy)
}

// renderSQL orders the statements so drops come first and foreign keys last, which keeps the script valid
// without knowing the dependency graph. Matching names sort so the text is stable.
func renderSQL(d differ) string {
	var drops, schemas, creates, alters, indexes, cons, fks, tail []string

	for _, v := range d.removedViews {
		drops = append(drops, "DROP "+viewKind(v)+" "+qual(v.Schema, v.Name)+";")
	}
	for _, v := range d.changedViews {
		if v.Materialized {
			drops = append(drops, "DROP MATERIALIZED VIEW "+qual(v.Schema, v.Name)+";")
		}
	}
	for _, op := range d.tableOps {
		t := qual(op.schema, op.table)
		for _, n := range op.dropCons {
			drops = append(drops, fmt.Sprintf("ALTER TABLE %s DROP CONSTRAINT %s;", t, quoteIdent(n)))
		}
		for _, n := range op.dropIndexes {
			drops = append(drops, "DROP INDEX "+qual(op.schema, n)+";")
		}
	}
	for _, t := range d.removedTables {
		drops = append(drops, "DROP TABLE "+qual(t.Schema, t.Name)+";")
	}

	for _, n := range d.addedSchemas {
		schemas = append(schemas, "CREATE SCHEMA "+quoteIdent(n)+";")
	}
	for _, n := range d.removedSchemas {
		tail = append(tail, "DROP SCHEMA "+quoteIdent(n)+";")
	}
	for _, s := range d.addedSeqs {
		creates = append(creates, seqCreate(s))
	}
	for _, s := range d.changedSeqs {
		alters = append(alters, seqAlter(s))
	}
	for _, t := range d.addedTables {
		var parts []string
		for _, c := range t.Columns {
			parts = append(parts, "  "+columnDef(c))
		}
		for _, c := range t.Constraints {
			if c.Type == "f" {
				fks = append(fks, addConstraint(t.Schema, t.Name, c))
				continue
			}
			parts = append(parts, fmt.Sprintf("  CONSTRAINT %s %s", quoteIdent(c.Name), c.Definition))
		}
		creates = append(creates, fmt.Sprintf("CREATE TABLE %s (\n%s\n);", qual(t.Schema, t.Name), strings.Join(parts, ",\n")))
		for _, i := range t.Indexes {
			indexes = append(indexes, i.Definition+";")
		}
	}
	for _, op := range d.tableOps {
		t := qual(op.schema, op.table)
		for _, c := range op.addCols {
			alters = append(alters, fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s;", t, columnDef(c)))
		}
		for _, n := range op.dropCols {
			alters = append(alters, fmt.Sprintf("ALTER TABLE %s DROP COLUMN %s;", t, quoteIdent(n)))
		}
		for _, a := range op.alterCols {
			alters = append(alters, alterColumn(t, a)...)
		}
		for _, i := range op.addIndexes {
			indexes = append(indexes, i.Definition+";")
		}
		for _, c := range op.addCons {
			if c.Type == "f" {
				fks = append(fks, addConstraint(op.schema, op.table, c))
			} else {
				cons = append(cons, addConstraint(op.schema, op.table, c))
			}
		}
	}
	for _, v := range d.addedViews {
		tail = append(tail, createView(v, false))
	}
	for _, v := range d.changedViews {
		tail = append(tail, createView(v, !v.Materialized))
	}
	for _, s := range d.removedSeqs {
		tail = append(tail, "DROP SEQUENCE IF EXISTS "+qual(s.Schema, s.Name)+";")
	}

	groups := [][]string{drops, schemas, creates, alters, indexes, cons, fks, tail}
	var out []string
	for _, g := range groups {
		sort.Strings(g)
		out = append(out, g...)
	}
	if len(out) == 0 {
		return ""
	}
	return Header + "\n" + strings.Join(out, "\n") + "\n"
}

func viewKind(v View) string {
	if v.Materialized {
		return "MATERIALIZED VIEW"
	}
	return "VIEW"
}

func createView(v View, replace bool) string {
	kw := "CREATE "
	if replace {
		kw = "CREATE OR REPLACE "
	}
	return fmt.Sprintf("%s%s %s AS\n%s", kw, viewKind(v), qual(v.Schema, v.Name), strings.TrimRight(strings.TrimSpace(v.Definition), ";")+";")
}

func addConstraint(schema, table string, c Constraint) string {
	return fmt.Sprintf("ALTER TABLE %s ADD CONSTRAINT %s %s;", qual(schema, table), quoteIdent(c.Name), c.Definition)
}

func alterColumn(table string, a colAlter) []string {
	pre := "ALTER TABLE " + table + " ALTER COLUMN " + quoteIdent(a.name)
	var out []string
	if a.from.Type != a.to.Type {
		out = append(out, fmt.Sprintf("%s TYPE %s;", pre, a.to.Type))
	}
	if a.from.Default != a.to.Default {
		if a.to.Default == "" {
			out = append(out, pre+" DROP DEFAULT;")
		} else {
			out = append(out, fmt.Sprintf("%s SET DEFAULT %s;", pre, a.to.Default))
		}
	}
	if a.from.Nullable != a.to.Nullable {
		if a.to.Nullable {
			out = append(out, pre+" DROP NOT NULL;")
		} else {
			out = append(out, pre+" SET NOT NULL;")
		}
	}
	return out
}
