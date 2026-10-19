package api

import "testing"

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
