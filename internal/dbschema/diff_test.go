package dbschema

import (
	"strings"
	"testing"
)

func baseSnap() Snapshot {
	return Snapshot{
		Tables: []Table{{
			Schema: "public", Name: "users",
			Columns:     []Column{{Name: "id", Type: "integer"}, {Name: "email", Type: "text", Nullable: true}, {Name: "legacy", Type: "text", Nullable: true}},
			Indexes:     []Index{{Name: "users_email_idx", Definition: "CREATE INDEX users_email_idx ON public.users USING btree (email)"}},
			Constraints: []Constraint{{Name: "users_pkey", Type: "p", Definition: "PRIMARY KEY (id)"}},
		}, {Schema: "public", Name: "old_t", Columns: []Column{{Name: "id", Type: "integer"}}}},
		Views:     []View{{Schema: "public", Name: "v_old", Definition: "SELECT 1"}},
		Sequences: []Sequence{{Schema: "public", Name: "s1", Type: "bigint", Start: 1, Increment: 1, Min: 1, Max: 100}},
	}
}

func headSnap() Snapshot {
	return Snapshot{
		Tables: []Table{{
			Schema: "public", Name: "users",
			Columns:     []Column{{Name: "id", Type: "bigint"}, {Name: "email", Type: "text", Default: "''::text"}, {Name: "age", Type: "integer", Nullable: true}},
			Indexes:     []Index{{Name: "users_age_idx", Definition: "CREATE INDEX users_age_idx ON public.users USING btree (age)"}},
			Constraints: []Constraint{{Name: "users_pkey", Type: "p", Definition: "PRIMARY KEY (id)"}, {Name: "users_age_chk", Type: "c", Definition: "CHECK ((age > 0))"}},
		}, {
			Schema: "public", Name: "orders",
			Columns:     []Column{{Name: "id", Type: "integer"}, {Name: "user_id", Type: "integer"}},
			Indexes:     []Index{{Name: "orders_user_idx", Definition: "CREATE INDEX orders_user_idx ON public.orders USING btree (user_id)"}},
			Constraints: []Constraint{{Name: "orders_pkey", Type: "p", Definition: "PRIMARY KEY (id)"}, {Name: "orders_user_fk", Type: "f", Definition: "FOREIGN KEY (user_id) REFERENCES users(id)"}},
		}},
		Views:     []View{{Schema: "public", Name: "v_new", Definition: "SELECT 2"}},
		Sequences: []Sequence{{Schema: "public", Name: "s1", Type: "bigint", Start: 1, Increment: 5, Min: 1, Max: 100}},
	}
}

func TestDiffChangesAndSummary(t *testing.T) {
	r := Diff(baseSnap(), headSnap())
	got := map[string]bool{}
	for _, c := range r.Changes {
		got[c.Kind+" "+c.Object+" "+c.Table+"."+c.Name] = true
	}
	want := []string{
		"added table .orders", "removed table .old_t", "changed column users.id", "changed column users.email",
		"added column users.age", "removed column users.legacy", "added index users.users_age_idx",
		"removed index users.users_email_idx", "added constraint users.users_age_chk",
		"added view .v_new", "removed view .v_old", "changed sequence .s1",
	}
	for _, w := range want {
		if !got[w] {
			t.Errorf("missing change %q in %v", w, got)
		}
	}
	if len(r.Changes) != len(want) {
		t.Errorf("got %d changes, want %d", len(r.Changes), len(want))
	}
	if r.Summary.Added != 5 || r.Summary.Removed != 4 || r.Summary.Changed != 3 {
		t.Errorf("summary = %+v", r.Summary)
	}
}

func TestDiffSQL(t *testing.T) {
	sql := Diff(baseSnap(), headSnap()).SQL
	for _, w := range []string{
		Header,
		`DROP TABLE "public"."old_t";`,
		`DROP INDEX "public"."users_email_idx";`,
		`DROP VIEW "public"."v_old";`,
		`CREATE TABLE "public"."orders" (`,
		`ALTER TABLE "public"."users" ADD COLUMN "age" integer;`,
		`ALTER TABLE "public"."users" DROP COLUMN "legacy";`,
		`ALTER TABLE "public"."users" ALTER COLUMN "id" TYPE bigint;`,
		`ALTER TABLE "public"."users" ALTER COLUMN "email" SET DEFAULT ''::text;`,
		`ALTER TABLE "public"."users" ALTER COLUMN "email" SET NOT NULL;`,
		`ALTER TABLE "public"."users" ADD CONSTRAINT "users_age_chk" CHECK ((age > 0));`,
		`ALTER TABLE "public"."orders" ADD CONSTRAINT "orders_user_fk" FOREIGN KEY (user_id) REFERENCES users(id);`,
		`ALTER SEQUENCE "public"."s1" AS bigint INCREMENT BY 5`,
		`CREATE VIEW "public"."v_new" AS`,
	} {
		if !strings.Contains(sql, w) {
			t.Errorf("sql missing %q\n%s", w, sql)
		}
	}
	// A foreign key must come after the tables it can reference exist.
	if strings.Index(sql, "orders_user_fk") < strings.Index(sql, `CREATE TABLE "public"."orders"`) {
		t.Error("foreign key emitted before CREATE TABLE")
	}
	if strings.Index(sql, "DROP TABLE") > strings.Index(sql, "CREATE TABLE") {
		t.Error("drops must come before creates")
	}
}

func TestDiffIdentical(t *testing.T) {
	r := Diff(baseSnap(), baseSnap())
	if len(r.Changes) != 0 || r.SQL != "" || r.Summary != (Summary{}) {
		t.Fatalf("identical schemas produced %+v", r)
	}
}

func TestDiffQuotesIdentifiers(t *testing.T) {
	r := Diff(Snapshot{}, Snapshot{Tables: []Table{{Schema: "public", Name: `we"ird`, Columns: []Column{{Name: "a", Type: "text", Nullable: true}}}}})
	if !strings.Contains(r.SQL, `"we""ird"`) {
		t.Fatalf("identifier not quoted:\n%s", r.SQL)
	}
}

func TestDiffTruncatedCarries(t *testing.T) {
	if !Diff(Snapshot{Truncated: true}, Snapshot{}).Truncated {
		t.Fatal("truncated flag lost")
	}
}

func TestDiffCreatesAndDropsSchemas(t *testing.T) {
	r := Diff(Snapshot{Schemas: []string{"public", "old"}}, Snapshot{Schemas: []string{"public", "New Schema"}})
	if !strings.Contains(r.SQL, `CREATE SCHEMA "New Schema";`) || !strings.Contains(r.SQL, `DROP SCHEMA "old";`) || r.Summary.Added != 1 || r.Summary.Removed != 1 {
		t.Fatalf("schema diff wrong: %+v\n%s", r.Summary, r.SQL)
	}
}
