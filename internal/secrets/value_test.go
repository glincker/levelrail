package secrets

import (
	"bytes"
	"crypto/rand"
	"errors"
	"strings"
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

var testBinding = Binding{Scope: ScopeServiceSecret, Owner: "web", Key: "DATABASE_URL"}

// encryptLegacy reproduces the pre-binding format (nonce || GCM, no AAD)
// so tests can seed values the way older releases wrote them.
func encryptLegacy(t *testing.T, dek []byte, plaintext string) []byte {
	t.Helper()
	gcm, err := newGCM(dek)
	if err != nil {
		t.Fatalf("newGCM() error = %v", err)
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		t.Fatalf("rand.Read() error = %v", err)
	}
	return gcm.Seal(nonce, nonce, []byte(plaintext), nil)
}

func TestEncryptDecryptValue_RoundTrip(t *testing.T) {
	dek := testDEK(t)
	const plaintext = "postgres://user:hunter2@db.internal:5432/app" //nolint:gosec // fake fixture, the whole test proves this exact string never appears in the output

	ciphertext, err := EncryptValue(dek, testBinding, plaintext)
	if err != nil {
		t.Fatalf("EncryptValue() error = %v", err)
	}
	if bytes.Contains(ciphertext, []byte("hunter2")) || bytes.Contains(ciphertext, []byte(testBinding.Key)) {
		t.Fatal("ciphertext contains plaintext or binding bytes, encryption did not happen")
	}
	if !hasBoundPrefix(ciphertext) {
		t.Fatal("new ciphertext lacks the bound envelope prefix")
	}

	decrypted, legacy, err := DecryptValue(dek, testBinding, ciphertext)
	if err != nil {
		t.Fatalf("DecryptValue() error = %v", err)
	}
	if legacy {
		t.Error("legacy = true for a freshly bound ciphertext")
	}
	if decrypted != plaintext {
		t.Errorf("decrypted = %q, want %q", decrypted, plaintext)
	}
}

func TestDecryptValue_BindingTable(t *testing.T) {
	dek := testDEK(t)
	ciphertext, err := EncryptValue(dek, testBinding, "v")
	if err != nil {
		t.Fatalf("EncryptValue() error = %v", err)
	}

	tests := []struct {
		name    string
		b       Binding
		wantErr error
	}{
		{"same slot", testBinding, nil},
		{"other key same owner", Binding{Scope: ScopeServiceSecret, Owner: "web", Key: "API_KEY"}, ErrBindingMismatch},
		{"other owner same key", Binding{Scope: ScopeServiceSecret, Owner: "api", Key: "DATABASE_URL"}, ErrBindingMismatch},
		{"other scope", Binding{Scope: "vault", Owner: "web", Key: "DATABASE_URL"}, ErrBindingMismatch},
		// Length prefixes keep a shifted field boundary from colliding.
		{"shifted boundary", Binding{Scope: ScopeServiceSecret, Owner: "webD", Key: "ATABASE_URL"}, ErrBindingMismatch},
		{"empty key", Binding{Scope: ScopeServiceSecret, Owner: "web"}, ErrInvalidBinding},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, legacy, err := DecryptValue(dek, tt.b, ciphertext)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("DecryptValue() error = %v, want %v", err, tt.wantErr)
			}
			if tt.wantErr != nil && got != "" {
				t.Errorf("mismatch returned plaintext %q, want empty", got)
			}
			if tt.wantErr == nil && (got != "v" || legacy) {
				t.Errorf("got (%q, legacy=%t), want (\"v\", false)", got, legacy)
			}
		})
	}
}

func TestDecryptValue_LegacyFormat(t *testing.T) {
	dek := testDEK(t)
	// The adversarial cases look like the bound prefix or frame once
	// decrypted; legacy plaintext must come back verbatim, never parsed.
	adversarial := string(envelopeV1) + string(testBinding.encodeHeader()) + "tail"
	for _, plaintext := range []string{"plain-legacy", "", string(envelopeV1), adversarial, string(testBinding.encodeHeader()) + "x"} {
		ct := encryptLegacy(t, dek, plaintext)
		got, legacy, err := DecryptValue(dek, testBinding, ct)
		if err != nil {
			t.Fatalf("DecryptValue(legacy %q) error = %v", plaintext, err)
		}
		if !legacy || got != plaintext {
			t.Errorf("DecryptValue(legacy %q) = (%q, legacy=%t), want verbatim and legacy", plaintext, got, legacy)
		}
	}
}

func TestDecryptValue_LegacyNonceThatLooksLikePrefix(t *testing.T) {
	dek := testDEK(t)
	gcm, err := newGCM(dek)
	if err != nil {
		t.Fatal(err)
	}
	nonce := make([]byte, gcm.NonceSize())
	copy(nonce, envelopeV1)
	ct := gcm.Seal(bytes.Clone(nonce), nonce, []byte("collide"), nil)
	if !hasBoundPrefix(ct) {
		t.Fatal("fixture should start with the bound prefix")
	}
	got, legacy, err := DecryptValue(dek, testBinding, ct)
	if err != nil || !legacy || got != "collide" {
		t.Fatalf("DecryptValue() = (%q, %t, %v), want (collide, true, nil)", got, legacy, err)
	}
}

func TestDecryptValue_StrippedPrefixDowngradeFails(t *testing.T) {
	dek := testDEK(t)
	ct, err := EncryptValue(dek, testBinding, "secret")
	if err != nil {
		t.Fatal(err)
	}
	stripped := ct[len(envelopeV1):]
	other := Binding{Scope: ScopeServiceSecret, Owner: "web", Key: "API_KEY"}
	if got, _, err := DecryptValue(dek, other, stripped); err == nil {
		t.Fatalf("stripped bound ciphertext decrypted as legacy to %q, want an error", got)
	}
}

func TestEncryptValue_SamePlaintextProducesDifferentCiphertext(t *testing.T) {
	dek := testDEK(t)
	a, err := EncryptValue(dek, testBinding, "same value both times")
	if err != nil {
		t.Fatalf("EncryptValue() error = %v", err)
	}
	b, err := EncryptValue(dek, testBinding, "same value both times")
	if err != nil {
		t.Fatalf("EncryptValue() error = %v", err)
	}
	if bytes.Equal(a, b) {
		t.Fatal("encrypting the same plaintext twice produced identical ciphertext, the nonce is not being randomized")
	}
}

func TestDecryptValue_WrongDEKFails(t *testing.T) {
	ciphertext, err := EncryptValue(testDEK(t), testBinding, "secret")
	if err != nil {
		t.Fatalf("EncryptValue() error = %v", err)
	}
	if _, _, err := DecryptValue(testDEK(t), testBinding, ciphertext); err == nil {
		t.Fatal("DecryptValue() with the wrong DEK error = nil, want an error")
	}
}

func TestDecryptValue_TamperedCiphertextFails(t *testing.T) {
	dek := testDEK(t)
	ciphertext, err := EncryptValue(dek, testBinding, "secret")
	if err != nil {
		t.Fatalf("EncryptValue() error = %v", err)
	}
	for _, i := range []int{0, len(envelopeV1), len(ciphertext) - 1} {
		tampered := bytes.Clone(ciphertext)
		tampered[i] ^= 0xFF
		if _, _, err := DecryptValue(dek, testBinding, tampered); err == nil {
			t.Fatalf("DecryptValue() with byte %d flipped error = nil, want an error", i)
		}
	}
}

func TestDecryptValue_TruncatedCiphertextFails(t *testing.T) {
	dek := testDEK(t)
	for _, in := range [][]byte{[]byte("x"), envelopeV1, append(bytes.Clone(envelopeV1), 1, 2, 3)} {
		if _, _, err := DecryptValue(dek, testBinding, in); err == nil {
			t.Fatalf("DecryptValue(%x) error = nil, want an error", in)
		}
	}
}

func TestEncryptValue_InvalidInputsFail(t *testing.T) {
	if _, err := EncryptValue([]byte("too short"), testBinding, "secret"); err == nil {
		t.Fatal("EncryptValue() with an invalid DEK size error = nil, want an error")
	}
	huge := Binding{Scope: ScopeServiceSecret, Owner: strings.Repeat("o", 1<<16), Key: "K"}
	if _, err := EncryptValue(testDEK(t), huge, "secret"); !errors.Is(err, ErrInvalidBinding) {
		t.Fatalf("EncryptValue() with an oversized owner error = %v, want ErrInvalidBinding", err)
	}
}

func TestEncryptValue_EmptyPlaintextRoundTrips(t *testing.T) {
	dek := testDEK(t)
	ciphertext, err := EncryptValue(dek, testBinding, "")
	if err != nil {
		t.Fatalf("EncryptValue(\"\") error = %v", err)
	}
	got, _, err := DecryptValue(dek, testBinding, ciphertext)
	if err != nil {
		t.Fatalf("DecryptValue() error = %v", err)
	}
	if got != "" {
		t.Errorf("got %q, want empty string", got)
	}
}

func TestCheckBinding_MalformedFrames(t *testing.T) {
	header := testBinding.encodeHeader()
	tests := []struct {
		name  string
		frame []byte
		want  error
	}{
		{"empty", nil, ErrMalformedEnvelope},
		{"one byte", []byte{0}, ErrMalformedEnvelope},
		{"zero length field", []byte{0, 0, 0, 1, 'a', 0, 1, 'b'}, ErrMalformedEnvelope},
		{"length past end", []byte{0, 9, 'a'}, ErrMalformedEnvelope},
		{"two fields only", []byte{0, 1, 'a', 0, 1, 'b'}, ErrMalformedEnvelope},
		{"truncated header", header[:len(header)-1], ErrMalformedEnvelope},
		{"valid other slot", Binding{Scope: "a", Owner: "b", Key: "c"}.encodeHeader(), ErrBindingMismatch},
		{"valid header", append(bytes.Clone(header), "value"...), nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := checkBinding(tt.frame, testBinding)
			if !errors.Is(err, tt.want) {
				t.Fatalf("checkBinding() error = %v, want %v", err, tt.want)
			}
		})
	}
}

func FuzzSplitFrame(f *testing.F) {
	f.Add(testBinding.encodeHeader())
	f.Add(append(testBinding.encodeHeader(), "value"...))
	f.Add([]byte{})
	f.Add([]byte{0xFF, 0xFF, 0x00})
	f.Add([]byte{0, 1, 'a', 0, 1, 'b', 0, 1})
	f.Fuzz(func(t *testing.T, frame []byte) {
		headerLen, value, err := splitFrame(frame)
		if err != nil {
			return
		}
		if headerLen < 9 || headerLen > len(frame) || headerLen+len(value) != len(frame) {
			t.Fatalf("splitFrame(%x) = (%d, %d bytes), inconsistent with input length %d", frame, headerLen, len(value), len(frame))
		}
		if _, err := checkBinding(frame, testBinding); err != nil && !errors.Is(err, ErrBindingMismatch) {
			t.Fatalf("checkBinding() on a frame splitFrame accepted error = %v", err)
		}
	})
}

func TestBinding_RoundTripAcrossArbitraryFields(t *testing.T) {
	dek := testDEK(t)
	for _, b := range []Binding{
		{Scope: "s", Owner: "o", Key: "k"},
		{Scope: ScopeServiceSecret, Owner: "backup-target/abc", Key: "secret_access_key"},
		{Scope: ScopeServiceSecret, Owner: "o\x00with\x00nuls", Key: string(envelopeV1)},
	} {
		ct, err := EncryptValue(dek, b, "value")
		if err != nil {
			t.Fatalf("EncryptValue(%+v) error = %v", b, err)
		}
		got, legacy, err := DecryptValue(dek, b, ct)
		if err != nil || legacy || got != "value" {
			t.Fatalf("DecryptValue(%+v) = (%q, %t, %v)", b, got, legacy, err)
		}
	}
}

// TestFullEnvelope_MasterKeyToValueAndBack exercises master key, DEK and
// value layers together across a simulated process restart.
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
	ciphertext, err := EncryptValue(rawDEK, testBinding, secret)
	if err != nil {
		t.Fatalf("EncryptValue() error = %v", err)
	}

	freshMK, err := LoadMasterKey(mk.String())
	if err != nil {
		t.Fatalf("LoadMasterKey() error = %v", err)
	}
	recoveredDEK, err := freshMK.UnwrapDEK(wrappedDEK)
	if err != nil {
		t.Fatalf("UnwrapDEK() error = %v", err)
	}
	recoveredSecret, _, err := DecryptValue(recoveredDEK, testBinding, ciphertext)
	if err != nil {
		t.Fatalf("DecryptValue() error = %v", err)
	}
	if recoveredSecret != secret {
		t.Errorf("recovered secret = %q, want %q", recoveredSecret, secret)
	}
}
