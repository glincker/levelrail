package api

import (
	"testing"

	"golang.org/x/crypto/bcrypt"
)

func TestDummyPasswordHash_MatchesRealHashCost(t *testing.T) {
	hash := dummyPasswordHash()
	if hash == nil {
		t.Fatal("dummyPasswordHash() = nil, want a bcrypt hash")
	}
	cost, err := bcrypt.Cost(hash)
	if err != nil {
		t.Fatalf("bcrypt.Cost() error = %v", err)
	}
	if cost != bcrypt.DefaultCost {
		t.Errorf("dummy hash cost = %d, want %d (same as stored password hashes)", cost, bcrypt.DefaultCost)
	}
	if err := bcrypt.CompareHashAndPassword(hash, []byte("")); err == nil {
		t.Error("dummy hash matched an empty password")
	}
}
