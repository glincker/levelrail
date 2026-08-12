package secrets

import (
	"bytes"
	"testing"
)

func testDEK(t *testing.T) []byte {
	t.Helper()
	mk, err := GenerateMasterKey()
	if err != nil {
		t.Fatalf("GenerateMasterKey() error = %v", err)
	}
	raw, _, err := mk.GenerateDEK()
	if err != nil {
		t.Fatalf("GenerateDEK() error = %v", err)
	}
	return raw
}

func TestEncryptDecryptValue_RoundTrip(t *testing.T) {
	dek := testDEK(t)
	const plaintext = "postgres://user:hunter2@db.internal:5432/app" //nolint:gosec // fake fixture, the whole test proves this exact string never appears in the output

	ciphertext, err := EncryptValue(dek, plaintext)
	if err != nil {
		t.Fatalf("EncryptValue() error = %v", err)
	}
	if bytes.Contains(ciphertext, []byte(plaintext)) {
		t.Fatal("ciphertext contains the plaintext verbatim, encryption did not happen")
	}
	if bytes.Contains(ciphertext, []byte("hunter2")) {
		t.Fatal("ciphertext contains a plaintext substring, encryption did not happen")
	}

	decrypted, err := DecryptValue(dek, ciphertext)
	if err != nil {
		t.Fatalf("DecryptValue() error = %v", err)
	}
	if decrypted != plaintext {
		t.Errorf("decrypted = %q, want %q", decrypted, plaintext)
	}
}

func TestEncryptValue_SamePlaintextProducesDifferentCiphertext(t *testing.T) {
	// Proves the nonce is actually randomized per call: reusing a nonce
	// with the same AES-GCM key is a critical vulnerability class (it
	// breaks both confidentiality and authenticity of every message
	// encrypted under the reused nonce), so this is a real security
	// property, not a cosmetic one.
	dek := testDEK(t)
	const plaintext = "same value both times"

	a, err := EncryptValue(dek, plaintext)
	if err != nil {
		t.Fatalf("EncryptValue() error = %v", err)
	}
	b, err := EncryptValue(dek, plaintext)
	if err != nil {
		t.Fatalf("EncryptValue() error = %v", err)
	}
	if bytes.Equal(a, b) {
		t.Fatal("encrypting the same plaintext twice produced identical ciphertext, the nonce is not being randomized")
	}
}

func TestDecryptValue_WrongDEKFails(t *testing.T) {
	dekA := testDEK(t)
	dekB := testDEK(t)

	ciphertext, err := EncryptValue(dekA, "secret")
	if err != nil {
		t.Fatalf("EncryptValue() error = %v", err)
	}

	if _, err := DecryptValue(dekB, ciphertext); err == nil {
		t.Fatal("DecryptValue() with the wrong DEK error = nil, want an error")
	}
}

func TestDecryptValue_TamperedCiphertextFails(t *testing.T) {
	dek := testDEK(t)

	ciphertext, err := EncryptValue(dek, "secret")
	if err != nil {
		t.Fatalf("EncryptValue() error = %v", err)
	}

	tampered := make([]byte, len(ciphertext))
	copy(tampered, ciphertext)
	tampered[len(tampered)-1] ^= 0xFF

	if _, err := DecryptValue(dek, tampered); err == nil {
		t.Fatal("DecryptValue() on tampered ciphertext error = nil, want an error: GCM's auth tag must catch this")
	}
}

func TestDecryptValue_TruncatedCiphertextFails(t *testing.T) {
	dek := testDEK(t)
	if _, err := DecryptValue(dek, []byte("x")); err == nil {
		t.Fatal("DecryptValue() on truncated input error = nil, want an error")
	}
}

func TestEncryptValue_InvalidDEKSizeFails(t *testing.T) {
	_, err := EncryptValue([]byte("too short"), "secret")
	if err == nil {
		t.Fatal("EncryptValue() with an invalid DEK size error = nil, want an error, not a panic")
	}
}

func TestEncryptValue_EmptyPlaintextRoundTrips(t *testing.T) {
	dek := testDEK(t)
	ciphertext, err := EncryptValue(dek, "")
	if err != nil {
		t.Fatalf("EncryptValue(\"\") error = %v", err)
	}
	got, err := DecryptValue(dek, ciphertext)
	if err != nil {
		t.Fatalf("DecryptValue() error = %v", err)
	}
	if got != "" {
		t.Errorf("got %q, want empty string", got)
	}
}

// TestFullEnvelope_MasterKeyToValueAndBack exercises the entire chain
// CLAUDE.md 4.10 describes in one pass: a master key wraps a per-app
// DEK, the DEK encrypts a value, and both unwrap steps reverse it
// correctly, proving the pieces compose the way the design intends, not
// just that each piece works in isolation.
func TestFullEnvelope_MasterKeyToValueAndBack(t *testing.T) {
	mk, err := GenerateMasterKey()
	if err != nil {
		t.Fatalf("GenerateMasterKey() error = %v", err)
	}

	rawDEK, wrappedDEK, err := mk.GenerateDEK()
	if err != nil {
		t.Fatalf("GenerateDEK() error = %v", err)
	}

	const secret = "API_KEY=sk-abc123" //nolint:gosec // fake fixture for the full-envelope round-trip test
	ciphertext, err := EncryptValue(rawDEK, secret)
	if err != nil {
		t.Fatalf("EncryptValue() error = %v", err)
	}

	// Simulate a fresh process: only the master key (loaded from its
	// serialized form) and the two ciphertexts (wrappedDEK, ciphertext)
	// persist; rawDEK here stands in for "never persisted, held only in
	// memory during this operation".
	freshMK, err := LoadMasterKey(mk.String())
	if err != nil {
		t.Fatalf("LoadMasterKey() error = %v", err)
	}

	recoveredDEK, err := freshMK.UnwrapDEK(wrappedDEK)
	if err != nil {
		t.Fatalf("UnwrapDEK() error = %v", err)
	}

	recoveredSecret, err := DecryptValue(recoveredDEK, ciphertext)
	if err != nil {
		t.Fatalf("DecryptValue() error = %v", err)
	}
	if recoveredSecret != secret {
		t.Errorf("recovered secret = %q, want %q", recoveredSecret, secret)
	}
}
