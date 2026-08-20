package api

import (
	"testing"
	"time"
)

func TestPendingState_ConsumeSingleUse(t *testing.T) {
	s := newPendingState()

	state, err := s.begin()
	if err != nil {
		t.Fatalf("begin() error = %v", err)
	}
	if state == "" {
		t.Fatal("begin() returned an empty state")
	}

	if !s.consume(state) {
		t.Fatal("consume(state) = false on first use, want true")
	}
	if s.consume(state) {
		t.Fatal("consume(state) = true on second use, want false (single-use)")
	}
}

func TestPendingState_ConsumeWrongValue(t *testing.T) {
	s := newPendingState()
	if _, err := s.begin(); err != nil {
		t.Fatalf("begin() error = %v", err)
	}
	if s.consume("not-the-real-state") {
		t.Error("consume(wrong value) = true, want false")
	}
	// A wrong guess must not have burned the real pending state.
	realState, err := s.begin()
	if err != nil {
		t.Fatalf("begin() error = %v", err)
	}
	if !s.consume(realState) {
		t.Error("consume(real state) after a wrong guess = false, want true")
	}
}

func TestPendingState_Expired(t *testing.T) {
	s := newPendingState()
	state, err := s.begin()
	if err != nil {
		t.Fatalf("begin() error = %v", err)
	}
	s.mu.Lock()
	s.expiresAt = time.Now().Add(-time.Second)
	s.mu.Unlock()

	if s.consume(state) {
		t.Error("consume(state) after expiry = true, want false")
	}
}

func TestPendingState_EmptyInputsRejected(t *testing.T) {
	s := newPendingState()
	if s.consume("") {
		t.Error("consume(\"\") on a fresh state store = true, want false")
	}
	if _, err := s.begin(); err != nil {
		t.Fatalf("begin() error = %v", err)
	}
	if s.consume("") {
		t.Error("consume(\"\") with a real pending state = true, want false")
	}
}
