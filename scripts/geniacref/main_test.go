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
