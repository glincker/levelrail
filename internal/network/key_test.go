package network

import (
	"strings"
	"testing"
)

func TestGeneratePrivateKey_IsClampedAndUnique(t *testing.T) {
	a, err := GeneratePrivateKey()
	if err != nil {
		t.Fatalf("GeneratePrivateKey: %v", err)
	}
	b, err := GeneratePrivateKey()
	if err != nil {
		t.Fatalf("GeneratePrivateKey: %v", err)
	}
	if a == b {
		t.Fatal("two generated keys are identical")
	}
	if a.IsZero() {
		t.Fatal("generated key is the zero key")
	}
	// RFC 7748 clamping, the property wg(8) also enforces on input: a
	// key that is not clamped derives a different public key than the
	// peer's own tooling would from the same bytes.
	if a[0]&7 != 0 {
		t.Errorf("low three bits of byte 0 not cleared: %08b", a[0])
	}
	if a[31]&128 != 0 {
		t.Errorf("high bit of byte 31 not cleared: %08b", a[31])
	}
	if a[31]&64 == 0 {
		t.Errorf("bit 6 of byte 31 not set: %08b", a[31])
	}
}

func TestKey_PublicKeyIsDeterministic(t *testing.T) {
	priv, err := GeneratePrivateKey()
	if err != nil {
		t.Fatalf("GeneratePrivateKey: %v", err)
	}
	first, err := priv.PublicKey()
	if err != nil {
		t.Fatalf("PublicKey: %v", err)
	}
	second, err := priv.PublicKey()
	if err != nil {
		t.Fatalf("PublicKey: %v", err)
	}
	if first != second {
		t.Fatal("the same private key derived two different public keys")
	}
	if first == priv {
		t.Fatal("public key equals the private key it was derived from")
	}
}

func TestKey_RoundTrip(t *testing.T) {
	tests := []struct {
		name string
		key  Key
	}{
		{name: "zero", key: Key{}},
		{name: "all ones", key: func() Key {
			var k Key
			for i := range k {
				k[i] = 0xff
			}
			return k
		}()},
		{name: "ascending", key: func() Key {
			var k Key
			for i := range k {
				k[i] = byte(i)
			}
			return k
		}()},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseKey(tc.key.String())
			if err != nil {
				t.Fatalf("ParseKey(%q): %v", tc.key.String(), err)
			}
			if got != tc.key {
				t.Errorf("base64 round trip: got %v, want %v", got, tc.key)
			}

			gotHex, err := parseHexKey(tc.key.Hex())
			if err != nil {
				t.Fatalf("parseHexKey(%q): %v", tc.key.Hex(), err)
			}
			if gotHex != tc.key {
				t.Errorf("hex round trip: got %v, want %v", gotHex, tc.key)
			}
			if len(tc.key.Hex()) != KeyLen*2 {
				t.Errorf("Hex() length: got %d, want %d", len(tc.key.Hex()), KeyLen*2)
			}
		})
	}
}

func TestParseKey_Rejects(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "empty", input: "", want: "0 bytes"},
		{name: "not base64", input: "!!!!not base64!!!!", want: "parse key"},
		{name: "too short", input: "AAAA", want: "want 32"},
		{name: "too long", input: strings.Repeat("A", 48), want: "want 32"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseKey(tc.input)
			if err == nil {
				t.Fatalf("ParseKey(%q): want an error, got nil", tc.input)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("ParseKey(%q) error = %q, want it to mention %q", tc.input, err, tc.want)
			}
		})
	}
}

func TestParseHexKey_Rejects(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{name: "empty", input: ""},
		{name: "too short", input: strings.Repeat("ab", 31)},
		{name: "too long", input: strings.Repeat("ab", 33)},
		{name: "non hex character", input: strings.Repeat("a", 63) + "z"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := parseHexKey(tc.input); err == nil {
				t.Fatalf("parseHexKey(%q): want an error, got nil", tc.input)
			}
		})
	}
}

// testKey builds a deterministic, non-zero key for tests that need
// distinguishable peers without caring about the key's cryptographic
// value. Not GeneratePrivateKey, because a table-driven test asserting
// on a planned config needs the same inputs to produce the same output
// every run.
func testKey(seed byte) Key {
	var k Key
	for i := range k {
		k[i] = seed + byte(i)
	}
	return k
}
