package api

import (
	"testing"
	"time"
)

func TestPendingState_ConsumeSingleUse(t *testing.T) {
	s := newPendingState()

	state, err := s.begin("")
	if err != nil {
		t.Fatalf("begin() error = %v", err)
	}
	if state == "" {
		t.Fatal("begin() returned an empty state")
	}

	if _, ok := s.consume(state); !ok {
		t.Fatal("consume(state) ok = false on first use, want true")
	}
	if _, ok := s.consume(state); ok {
		t.Fatal("consume(state) ok = true on second use, want false (single-use)")
	}
}

func TestPendingState_ConsumeWrongValue(t *testing.T) {
	s := newPendingState()
	if _, err := s.begin(""); err != nil {
		t.Fatalf("begin() error = %v", err)
	}
	if _, ok := s.consume("not-the-real-state"); ok {
		t.Error("consume(wrong value) ok = true, want false")
	}
	// A wrong guess must not have burned the real pending state.
	realState, err := s.begin("")
	if err != nil {
		t.Fatalf("begin() error = %v", err)
	}
	if _, ok := s.consume(realState); !ok {
		t.Error("consume(real state) after a wrong guess ok = false, want true")
	}
}

func TestPendingState_Expired(t *testing.T) {
	s := newPendingState()
	state, err := s.begin("")
	if err != nil {
		t.Fatalf("begin() error = %v", err)
	}
	s.mu.Lock()
	s.expiresAt = time.Now().Add(-time.Second)
	s.mu.Unlock()

	if _, ok := s.consume(state); ok {
		t.Error("consume(state) after expiry ok = true, want false")
	}
}

func TestPendingState_EmptyInputsRejected(t *testing.T) {
	s := newPendingState()
	if _, ok := s.consume(""); ok {
		t.Error("consume(\"\") on a fresh state store ok = true, want false")
	}
	if _, err := s.begin(""); err != nil {
		t.Fatalf("begin() error = %v", err)
	}
	if _, ok := s.consume(""); ok {
		t.Error("consume(\"\") with a real pending state ok = true, want false")
	}
}

// TestPendingState_CarriesValue proves the value passed to begin comes
// back out of consume unchanged: GitHub Enterprise Server support
// (pendingState's own doc comment) depends on this to remember which
// instance a registration attempt targeted from register/start through
// to the callback.
func TestPendingState_CarriesValue(t *testing.T) {
	s := newPendingState()
	state, err := s.begin("https://ghe.example.com")
	if err != nil {
		t.Fatalf("begin() error = %v", err)
	}
	value, ok := s.consume(state)
	if !ok {
		t.Fatal("consume(state) ok = false, want true")
	}
	if value != "https://ghe.example.com" {
		t.Errorf("consume(state) value = %q, want the value passed to begin", value)
	}
}
