package store

import (
	"encoding/json"
	"errors"
	"testing"
	"time"
)

func TestStackRevisions_RefuseJSONEscapedSecretLeak(t *testing.T) {
	db := openTestDB(t)
	s := newTestStack(t, db, "escaped")
	value := `p<a"ss&w\rd`
	docs, _ := json.Marshal(map[string]string{"content": "env: {X: '" + value + "'}"})
	_, err := db.AppendStackRevision(t.Context(), NewStackRevision{
		StackID: s.ID, RenderedDocs: docs, Secrets: map[string]string{"DB_PASSWORD": value},
	}, time.Now())
	if !errors.Is(err, ErrStackSecretLeak) {
		t.Fatalf("err = %v", err)
	}
}

func TestStackRevisions_ShortSecretValueIsNotMatched(t *testing.T) {
	db := openTestDB(t)
	s := newTestStack(t, db, "short")
	_, err := db.AppendStackRevision(t.Context(), NewStackRevision{
		StackID: s.ID, RenderedDocs: []byte("env: {X: x}"), Secrets: map[string]string{"K": "x"},
	}, time.Now())
	if err != nil {
		t.Fatalf("one character value must not refuse the revision: %v", err)
	}
}
