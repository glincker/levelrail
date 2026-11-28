package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/glincker/levelrail/internal/apiclient"
)

func TestPrintSchemaDiffMasksControlCharacters(t *testing.T) {
	d := apiclient.DatabaseBranchSchemaDiff{Branch: "b\x1b[31m", Against: "main", Changes: []apiclient.SchemaDiffChange{{Object: "table", Kind: "added", Schema: "public", Name: "t\x07"}}}
	var out bytes.Buffer
	printSchemaDiff(&out, d, false)
	if strings.ContainsAny(out.String(), "\x1b\x07") || !strings.Contains(out.String(), "+ table public.t?") {
		t.Fatalf("unsafe or wrong output: %q", out.String())
	}
	out.Reset()
	printSchemaDiff(&out, apiclient.DatabaseBranchSchemaDiff{SQL: "SELECT 1;\nSELECT 2;"}, true)
	if strings.Count(out.String(), "\n") != 2 {
		t.Fatalf("sql lines lost: %q", out.String())
	}
}
