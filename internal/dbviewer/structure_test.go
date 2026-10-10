package dbviewer

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestPgDDLTable(t *testing.T) {
	row := pgStructureRow{
		Kind: "table",
		Columns: []Column{
			{Name: "id", Type: "integer", Nullable: false, Default: "nextval('s'::regclass)", PrimaryKey: true},
			{Name: "owner", Type: "text", Nullable: true},
		},
		Constraints: []pgConstraint{{Name: "t_pkey", Type: "p", Definition: "PRIMARY KEY (id)"}},
	}
	got := pgDDL("public", "t", row, []string{"CREATE INDEX t_owner ON public.t (owner)"})
	for _, want := range []string{
		`CREATE TABLE "public"."t" (`,
		`"id" integer NOT NULL DEFAULT nextval('s'::regclass)`,
		`"owner" text,`,
		`CONSTRAINT "t_pkey" PRIMARY KEY (id)`,
		"CREATE INDEX t_owner ON public.t (owner);",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("ddl missing %q:\n%s", want, got)
		}
	}
}

func TestPgDDLView(t *testing.T) {
	got := pgDDL("public", "v", pgStructureRow{Kind: "view", ViewDefinition: " SELECT 1;\n"}, nil)
	if !strings.HasPrefix(got, `CREATE VIEW "public"."v" AS`) {
		t.Errorf("ddl = %q", got)
	}
}

func TestStructureRejectsBadIdent(t *testing.T) {
	_, err := (Target{}).TableStructure(context.Background(), "", "x")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v", err)
	}
}
