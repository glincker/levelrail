// Package identity normalizes and validates user-facing identifiers and
// names (emails, usernames, org, team, project and token names) so lookalike
// or invisible-character input cannot forge or duplicate an identity.
package identity

import (
	"errors"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

// Default caps, exported vars so a deployment can tune them at boot.
var (
	MaxNameRunes  = 100
	MaxNameBytes  = 400
	MaxEmailBytes = 254
)

// ErrInvalid wraps every validation failure.
var ErrInvalid = errors.New("invalid text")

// Normalize returns s in NFKC with surrounding space trimmed, so composed,
// decomposed and compatibility forms of one string compare equal.
func Normalize(s string) string {
	return strings.TrimSpace(norm.NFKC.String(s))
}

// NormalizeEmail is the canonical form used to store and look up an email
// or username: NFKC, trimmed, lowercased.
func NormalizeEmail(s string) string {
	return strings.ToLower(Normalize(s))
}

// ValidateEmail normalizes s and rejects control, format, space and
// over-long input. A value without "@" passes, legacy bare usernames exist.
func ValidateEmail(s string) (string, error) {
	n := NormalizeEmail(s)
	if n == "" {
		return "", fmt.Errorf("%w: email is required", ErrInvalid)
	}
	if len(n) > MaxEmailBytes {
		return "", fmt.Errorf("%w: email is at most %d bytes", ErrInvalid, MaxEmailBytes)
	}
	if err := checkRunes(n, false, false); err != nil {
		return "", fmt.Errorf("%w: email %w", ErrInvalid, err)
	}
	return n, nil
}

// ValidateName normalizes a display name to NFC and rejects control,
// bidi, zero-width and invisible-filler characters and over-long input.
// ZWNJ and ZWJ stay allowed: scripts such as Persian need them.
func ValidateName(label, s string) (string, error) {
	n := strings.TrimSpace(norm.NFC.String(s))
	if n == "" {
		return "", fmt.Errorf("%w: %s is required", ErrInvalid, label)
	}
	if utf8.RuneCountInString(n) > MaxNameRunes || len(n) > MaxNameBytes {
		return "", fmt.Errorf("%w: %s is at most %d characters", ErrInvalid, label, MaxNameRunes)
	}
	if err := checkRunes(n, true, true); err != nil {
		return "", fmt.Errorf("%w: %s %w", ErrInvalid, label, err)
	}
	return n, nil
}

// Sanitize replaces characters that could spoof or break a log or audit
// view with U+FFFD, for values that are shown but cannot be rejected.
func Sanitize(s string) string {
	if !utf8.ValidString(s) {
		s = strings.ToValidUTF8(s, "�")
	}
	return strings.Map(func(r rune) rune {
		if forbidden(r, false) || (r != ' ' && unicode.IsSpace(r)) {
			return '�'
		}
		return r
	}, s)
}

func checkRunes(s string, allowSpace, allowJoiners bool) error {
	if !utf8.ValidString(s) {
		return errors.New("is not valid UTF-8")
	}
	for _, r := range s {
		if forbidden(r, allowJoiners) || (!allowSpace && unicode.IsSpace(r)) || (allowSpace && r != ' ' && unicode.IsSpace(r)) {
			return fmt.Errorf("contains a disallowed character (U+%04X)", r)
		}
	}
	return nil
}

// forbidden covers controls, bidi overrides and isolates, zero-width and
// other format characters, surrogates, private use, noncharacters and
// visually blank fillers.
func forbidden(r rune, allowJoiners bool) bool {
	if allowJoiners && (r == 0x200C || r == 0x200D) {
		return false
	}
	switch r {
	case 0x115F, 0x1160, 0x3164, 0xFFA0, 0x2800, 0xFFFD:
		return true
	}
	if r&0xFFFE == 0xFFFE || (r >= 0xFDD0 && r <= 0xFDEF) {
		return true
	}
	return unicode.In(r, unicode.Cc, unicode.Cf, unicode.Cs, unicode.Co, unicode.Zl, unicode.Zp)
}
