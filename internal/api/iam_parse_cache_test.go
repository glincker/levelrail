package api

import (
	"fmt"
	"strings"
	"testing"
)

func TestParseDocumentCached(t *testing.T) {
	const doc = `{"Statement":[{"Effect":"Deny","Action":["*"],"Resource":["app:x"]}]}`
	a, err := parseDocumentCached(doc)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	b, _ := parseDocumentCached(doc)
	if a != b {
		t.Fatal("second parse of the same text should be served from the cache")
	}
	changed, err := parseDocumentCached(`{"Statement":[{"Effect":"Allow","Action":["read"],"Resource":["app:x"]}]}`)
	if err != nil || changed.Statement[0].Effect != EffectAllow {
		t.Fatalf("an edited document must not reuse the old parse: %v %+v", err, changed)
	}
	if _, err := parseDocumentCached(`{"Statement":[]}`); err == nil {
		t.Fatal("an invalid document must still be rejected")
	}
}

func TestParseDocumentCacheIsBounded(t *testing.T) {
	big := `{"Statement":[{"Effect":"Allow","Action":["read"],"Resource":["app:` + strings.Repeat("x", parsedDocumentMaxKey) + `"]}]}`
	a, err := parseDocumentCached(big)
	if err != nil {
		t.Fatalf("parse big: %v", err)
	}
	if b, _ := parseDocumentCached(big); a == b {
		t.Fatal("a document over the per-entry cap must not be cached")
	}
	for i := range parsedDocumentCacheMax + 10 {
		if _, err := parseDocumentCached(fmt.Sprintf(`{"Statement":[{"Effect":"Allow","Action":["read"],"Resource":["app:n%d"]}]}`, i)); err != nil {
			t.Fatal(err)
		}
	}
	parsedDocuments.RLock()
	n, size := len(parsedDocuments.m), parsedDocuments.bytes
	parsedDocuments.RUnlock()
	if n > parsedDocumentCacheMax || size > parsedDocumentCacheBytes {
		t.Fatalf("cache grew past its caps: %d entries, %d bytes", n, size)
	}
}
