package api

import (
	"testing"

	"golang.org/x/crypto/bcrypt"
)

func TestProductionPasswordHashCostIsUnchanged(t *testing.T) {
	if productionPasswordHashCost != bcrypt.DefaultCost {
		t.Fatalf("productionPasswordHashCost = %d, want bcrypt.DefaultCost (%d)", productionPasswordHashCost, bcrypt.DefaultCost)
	}
	hash, err := bcrypt.GenerateFromPassword([]byte("pw"), productionPasswordHashCost)
	if err != nil {
		t.Fatalf("GenerateFromPassword: %v", err)
	}
	if got, err := bcrypt.Cost(hash); err != nil || got != bcrypt.DefaultCost {
		t.Fatalf("cost = %d, %v; want %d", got, err, bcrypt.DefaultCost)
	}
}

func TestHashPasswordUsesConfiguredCost(t *testing.T) {
	hash, err := hashPassword("pw")
	if err != nil {
		t.Fatalf("hashPassword: %v", err)
	}
	if got, _ := bcrypt.Cost(hash); got != passwordHashCost {
		t.Fatalf("cost = %d, want %d", got, passwordHashCost)
	}
}
