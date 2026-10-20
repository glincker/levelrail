// Package totp holds the recovery-code format helpers and an RFC 6238 code
// generator the auth engine and its tests share. Verification and secret
// handling live in the auth library.
package totp

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1" //nolint:gosec // required by RFC 6238 itself, not a hashing-for-security-strength choice
	"encoding/base32"
	"encoding/binary"
	"fmt"
	"strings"
	"time"
)

const (
	// secretBytes is 160 bits, RFC 4226's own recommended HMAC-SHA1 key size.
	secretBytes = 20
	stepSeconds = 30
	digits      = 6
	digitsMod   = 1_000_000 // 10^digits, kept as an int constant since digits is fixed

	// skewSteps tolerates a device's clock running one step (30s) fast or
	// slow in either direction without widening how long a leaked code
	// stays valid beyond that.
	skewSteps = 1

	// recoveryCodeBytes is 80 bits, encoded as 16 base32 characters and
	// grouped into four hyphenated blocks of four for readability.
	recoveryCodeBytes = 10
)

var base32Encoding = base32.StdEncoding.WithPadding(base32.NoPadding)

// GenerateCode returns the 6-digit TOTP for secretBase32 at the step
// containing now, with no clock-skew tolerance (that's Validate's job,
// which calls this internally): the symmetric counterpart callers need
// wherever something other than a live authenticator app must produce a
// code, e.g. tests exercising the confirm/verify handlers end to end.
func GenerateCode(secretBase32 string, now time.Time) (string, error) {
	key, err := decodeSecret(secretBase32)
	if err != nil {
		return "", fmt.Errorf("totp: generate code: %w", err)
	}
	return generate(key, now.Unix()/stepSeconds), nil
}

// generate computes the HOTP-SHA1 value (RFC 4226 section 5.3) for key
// at counter, truncated to digits decimal digits.
func generate(key []byte, counter int64) string {
	var counterBytes [8]byte
	// counter is always Unix-time-derived (now.Unix()/stepSeconds) and
	// so always non-negative until well past this software's useful
	// life; the int64->uint64 conversion gosec flags can't actually
	// overflow in practice.
	binary.BigEndian.PutUint64(counterBytes[:], uint64(counter)) //nolint:gosec // see comment above

	mac := hmac.New(sha1.New, key)
	mac.Write(counterBytes[:])
	sum := mac.Sum(nil)

	offset := sum[len(sum)-1] & 0x0f
	truncated := (uint32(sum[offset]&0x7f) << 24) |
		(uint32(sum[offset+1]) << 16) |
		(uint32(sum[offset+2]) << 8) |
		uint32(sum[offset+3])

	return fmt.Sprintf("%0*d", digits, truncated%digitsMod)
}

func decodeSecret(secretBase32 string) ([]byte, error) {
	normalized := strings.ToUpper(strings.TrimSpace(secretBase32))
	return base32Encoding.DecodeString(normalized)
}

// GenerateRecoveryCode returns one single-use fallback code: 80 random
// bits, base32-encoded, grouped into four hyphenated blocks of four
// characters for readability when typed by hand.
func GenerateRecoveryCode() (string, error) {
	buf := make([]byte, recoveryCodeBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("totp: generate recovery code: %w", err)
	}
	encoded := base32Encoding.EncodeToString(buf)
	return encoded[0:4] + "-" + encoded[4:8] + "-" + encoded[8:12] + "-" + encoded[12:16], nil
}

// NormalizeRecoveryCode strips whitespace and hyphens and uppercases a
// recovery code, so a user's re-typed or copy-pasted input matches
// regardless of formatting differences from GenerateRecoveryCode's own
// output.
func NormalizeRecoveryCode(code string) string {
	code = strings.ToUpper(strings.TrimSpace(code))
	return strings.ReplaceAll(code, "-", "")
}
