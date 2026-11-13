package main

import (
	"bytes"
	"os"
	"testing"

	"github.com/GLINCKER/levelrail/internal/iac"
)

func TestReferencePageIsCurrent(t *testing.T) {
	raw, err := iac.SchemaJSON()
	if err != nil {
		t.Fatal(err)
	}
	page, err := render(raw)
	if err != nil {
		t.Fatal(err)
	}
	again, _ := render(raw)
	if !bytes.Equal(page, again) {
		t.Fatal("render is not deterministic")
	}
	if !bytes.Contains(page, []byte("## App")) || !bytes.Contains(page, []byte("`spec.service.port`")) {
		t.Fatal("reference page misses the App kind or its service fields")
	}
	cur, err := os.ReadFile("../../" + pagePath)
	if err != nil || !bytes.Equal(cur, page) {
		t.Fatalf("%s is stale, run: go run ./scripts/geniacref", pagePath)
	}
}

func TestErrorsPageIsCurrentAndComplete(t *testing.T) {
	page, err := renderErrors(iac.AllCodes)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range iac.AllCodes {
		if !bytes.Contains(page, []byte("`"+c+"`")) {
			t.Fatalf("code %s missing from the page", c)
		}
	}
	if len(codeDocs) != len(iac.AllCodes) {
		t.Fatalf("codeDocs has %d entries for %d codes", len(codeDocs), len(iac.AllCodes))
	}
	cur, err := os.ReadFile("../../" + errorsPagePath)
	if err != nil || !bytes.Equal(cur, page) {
		t.Fatalf("%s is stale, run: go run ./scripts/geniacref", errorsPagePath)
	}
}
