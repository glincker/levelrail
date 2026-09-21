package store

import (
	"context"
	"testing"
)

func TestGetOrCreateInstanceID_PersistsAcrossCalls(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	first, err := db.GetOrCreateInstanceID(ctx)
	if err != nil {
		t.Fatalf("GetOrCreateInstanceID: %v", err)
	}
	if first == "" {
		t.Fatal("GetOrCreateInstanceID returned an empty ID")
	}

	second, err := db.GetOrCreateInstanceID(ctx)
	if err != nil {
		t.Fatalf("GetOrCreateInstanceID (second call): %v", err)
	}
	if second != first {
		t.Errorf("GetOrCreateInstanceID() = %q on second call, want the same value as the first call %q", second, first)
	}
}

func TestGetOrCreateInstanceID_UniquePerDatabase(t *testing.T) {
	ctx := context.Background()
	dbA := openTestDB(t)
	dbB := openTestDB(t)

	idA, err := dbA.GetOrCreateInstanceID(ctx)
	if err != nil {
		t.Fatalf("GetOrCreateInstanceID (A): %v", err)
	}
	idB, err := dbB.GetOrCreateInstanceID(ctx)
	if err != nil {
		t.Fatalf("GetOrCreateInstanceID (B): %v", err)
	}
	if idA == idB {
		t.Errorf("two separate databases minted the same instance ID %q, want distinct IDs", idA)
	}
}

func TestNewInstanceID_HasExpectedPrefix(t *testing.T) {
	id, err := NewInstanceID()
	if err != nil {
		t.Fatalf("NewInstanceID: %v", err)
	}
	if len(id) <= len(instanceIDPrefix) || id[:len(instanceIDPrefix)] != instanceIDPrefix {
		t.Errorf("NewInstanceID() = %q, want it to start with %q", id, instanceIDPrefix)
	}
}
