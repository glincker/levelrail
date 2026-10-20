package secrets

import (
	"bytes"
	"testing"
)

func TestGenerateDEK_ProducesCorrectSizeAndWrapsCleanly(t *testing.T) {
	mk, err := GenerateMasterKey()
	if err != nil {
		t.Fatalf("GenerateMasterKey() error = %v", err)
	}

	raw, wrapped, err := mk.GenerateDEK()
	if err != nil {
		t.Fatalf("GenerateDEK() error = %v", err)
	}
	if len(raw) != dekSize {
		t.Errorf("raw DEK length = %d, want %d (AES-256)", len(raw), dekSize)
	}
	if len(wrapped) == 0 {
		t.Error("wrapped DEK is empty")
	}
	// The wrapped form must not contain the raw key bytes verbatim,
	// otherwise "wrapping" isn't doing anything.
	if bytes.Contains(wrapped, raw) {
		t.Error("wrapped DEK contains the raw key bytes in plaintext, wrapping did not encrypt it")
	}
}

func TestGenerateDEK_ProducesDistinctKeysEachCall(t *testing.T) {
	mk, err := GenerateMasterKey()
	if err != nil {
		t.Fatalf("GenerateMasterKey() error = %v", err)
	}

	rawA, _, err := mk.GenerateDEK()
	if err != nil {
		t.Fatalf("GenerateDEK() error = %v", err)
	}
	rawB, _, err := mk.GenerateDEK()
	if err != nil {
		t.Fatalf("GenerateDEK() error = %v", err)
	}
	if bytes.Equal(rawA, rawB) {
		t.Error("two calls to GenerateDEK produced the same key, want distinct keys per app")
	}
}

func TestUnwrapDEK_RoundTrip(t *testing.T) {
	mk, err := GenerateMasterKey()
	if err != nil {
		t.Fatalf("GenerateMasterKey() error = %v", err)
	}

	raw, wrapped, err := mk.GenerateDEK()
	if err != nil {
		t.Fatalf("GenerateDEK() error = %v", err)
	}

	unwrapped, err := mk.UnwrapDEK(wrapped)
	if err != nil {
		t.Fatalf("UnwrapDEK() error = %v", err)
	}
	if !bytes.Equal(raw, unwrapped) {
		t.Error("UnwrapDEK() did not return the same key GenerateDEK produced")
	}
}

func TestUnwrapDEK_WrongMasterKeyFails(t *testing.T) {
	mkA, err := GenerateMasterKey()
	if err != nil {
		t.Fatalf("GenerateMasterKey() error = %v", err)
	}
	mkB, err := GenerateMasterKey()
	if err != nil {
		t.Fatalf("GenerateMasterKey() error = %v", err)
	}

	_, wrapped, err := mkA.GenerateDEK()
	if err != nil {
		t.Fatalf("GenerateDEK() error = %v", err)
	}

	if _, err := mkB.UnwrapDEK(wrapped); err == nil {
		t.Fatal("UnwrapDEK() with the wrong master key error = nil, want an error: a leaked master key of one deployment must not unwrap another's secrets")
	}
}

func TestUnwrapDEK_TamperedWrappedKeyFails(t *testing.T) {
	mk, err := GenerateMasterKey()
	if err != nil {
		t.Fatalf("GenerateMasterKey() error = %v", err)
	}

	_, wrapped, err := mk.GenerateDEK()
	if err != nil {
		t.Fatalf("GenerateDEK() error = %v", err)
	}

	tampered := make(WrappedDEK, len(wrapped))
	copy(tampered, wrapped)
	tampered[len(tampered)-1] ^= 0xFF // flip bits in the last byte

	if _, err := mk.UnwrapDEK(tampered); err == nil {
		t.Fatal("UnwrapDEK() on tampered input error = nil, want an error")
	}
}

func TestUnwrapDEK_GarbageInputFails(t *testing.T) {
	mk, err := GenerateMasterKey()
	if err != nil {
		t.Fatalf("GenerateMasterKey() error = %v", err)
	}
	if _, err := mk.UnwrapDEK(WrappedDEK("not a wrapped key at all")); err == nil {
		t.Fatal("UnwrapDEK() on garbage input error = nil, want an error")
	}
}
