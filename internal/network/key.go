package network

// This file: WireGuard's key material, handled directly with
// golang.org/x/crypto/curve25519 rather than through wireguard-go's own
// device.NoisePrivateKey. Deliberate: key generation, parsing, and
// formatting are pure value operations that every layer of this package
// needs (Plan validates them, the store persists them, the coordinator
// compares them), and routing all of that through a dependency whose
// other half needs a TUN device and root would make the testable part of
// this package depend on the untestable part for no benefit. The wire
// format is identical either way: 32 raw bytes, standard base64 when
// textual, which is what wg(8), wireguard-go's UAPI, and every
// wg0.conf in the world already use.

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"

	"golang.org/x/crypto/curve25519"
)

// KeyLen is the length in bytes of a Curve25519 key, public or private.
const KeyLen = 32

// Key is one Curve25519 key. The same type covers public and private
// keys because they are the same shape and WireGuard itself does not
// distinguish them at the wire level; which one a given value is comes
// from the field it sits in (DeviceConfig.PrivateKey versus
// PeerConfig.PublicKey), not from its type.
//
// An array, not a slice, on purpose: it is comparable with ==, usable as
// a map key (which the duplicate-key detection in Validate relies on),
// and cannot be nil or the wrong length, so no length check is needed at
// any use site.
type Key [KeyLen]byte

// GeneratePrivateKey returns a new random Curve25519 private key, clamped
// per RFC 7748 the way WireGuard requires.
//
// This runs on the node that will use the key, never on the control
// plane: a node's private key never leaves the machine it belongs to,
// and the control plane only ever sees public keys (see Coordinator's
// doc comment for why the protocol is shaped to make that the only
// possible flow, rather than a convention that could be broken by a
// later change).
func GeneratePrivateKey() (Key, error) {
	var k Key
	if _, err := rand.Read(k[:]); err != nil {
		return Key{}, fmt.Errorf("network: generate private key: %w", err)
	}
	clamp(&k)
	return k, nil
}

// clamp applies RFC 7748's Curve25519 scalar clamping. WireGuard stores
// private keys already clamped, and wg(8) clamps on input, so an
// unclamped key would produce a public key that does not match what the
// peer's own tooling derives from the same bytes.
func clamp(k *Key) {
	k[0] &= 248
	k[31] = (k[31] & 127) | 64
}

// PublicKey derives the public key for this private key.
func (k Key) PublicKey() (Key, error) {
	pub, err := curve25519.X25519(k[:], curve25519.Basepoint)
	if err != nil {
		return Key{}, fmt.Errorf("network: derive public key: %w", err)
	}
	var out Key
	copy(out[:], pub)
	return out, nil
}

// IsZero reports whether this is the all-zero key, which is what an
// unset field looks like and never a legitimate key. Callers use this
// rather than comparing against Key{} directly so the intent ("this
// node has not reported a key yet") reads at the call site.
func (k Key) IsZero() bool {
	return k == Key{}
}

// String returns the standard base64 encoding, the same textual form
// wg(8) prints and accepts. Note this is the *only* String on this type:
// a private key formats identically to a public one, so nothing here
// can distinguish them for redaction purposes. Callers must not log a
// DeviceConfig.PrivateKey; see DeviceConfig's own doc comment.
func (k Key) String() string {
	return base64.StdEncoding.EncodeToString(k[:])
}

// Hex returns the lowercase hex encoding, the form wireguard-go's UAPI
// protocol uses (unlike wg(8)'s config files, which use base64). Kept
// distinct from String rather than making one of them the default, so
// neither the human-facing form nor the wire form is a silent choice at
// the call site.
func (k Key) Hex() string {
	const hexDigits = "0123456789abcdef"
	out := make([]byte, 0, KeyLen*2)
	for _, b := range k {
		out = append(out, hexDigits[b>>4], hexDigits[b&0x0f])
	}
	return string(out)
}

// ParseKey decodes a standard-base64 key, the form wg(8) emits and the
// form the control plane stores and ships over the wire.
func ParseKey(s string) (Key, error) {
	raw, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return Key{}, fmt.Errorf("network: parse key: %w", err)
	}
	if len(raw) != KeyLen {
		return Key{}, fmt.Errorf("network: parse key: got %d bytes, want %d", len(raw), KeyLen)
	}
	var k Key
	copy(k[:], raw)
	return k, nil
}
