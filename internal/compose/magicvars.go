package compose

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// generationKinds are the SERVICE_<KIND>_... prefixes a real template
// catalog's own convention uses for "auto-generate this at deploy
// time" (a random password, username, or encoded secret). Every other
// SERVICE_ name (FQDN, or an arbitrary third-party credential name) is
// either satisfied by its own bash-style ${...:-default} or left
// unresolved for the caller to reject.
var generationKinds = map[string]bool{
	"PASSWORD":   true,
	"USER":       true,
	"BASE64":     true,
	"HEX":        true,
	"REALBASE64": true,
}

var magicVarPattern = regexp.MustCompile(`\$\{SERVICE_([A-Za-z0-9_]+?)(?::-([^}]*))?\}|\$SERVICE_([A-Za-z0-9_]+)`)

// MagicVar is one parsed SERVICE_ placeholder found in an env value.
type MagicVar struct {
	Token       string
	Kind        string
	Key         string
	Length      int
	Default     string
	HasDefault  bool
	Generatable bool
}

// FindMagicVars returns every SERVICE_ token in s, in encounter order,
// including duplicates.
func FindMagicVars(s string) []MagicVar {
	matches := magicVarPattern.FindAllStringSubmatch(s, -1)
	out := make([]MagicVar, 0, len(matches))
	for _, m := range matches {
		name := m[1]
		if name == "" {
			name = m[3]
		}
		kind, key, length := splitMagicVarName(name)
		out = append(out, MagicVar{
			Token:       m[0],
			Kind:        kind,
			Key:         key,
			Length:      length,
			Default:     m[2],
			HasDefault:  strings.Contains(m[0], ":-"),
			Generatable: generationKinds[kind],
		})
	}
	return out
}

// splitMagicVarName splits "PASSWORD_64_APIKEY" into ("PASSWORD", 64,
// "APIKEY"), or "FQDN_ACTIVEPIECES" into ("FQDN", 0, "ACTIVEPIECES").
// The length segment only applies to generatable kinds: "DB_NAME"
// stays ("DB", 0, "NAME"), not misread as a length hint.
func splitMagicVarName(name string) (kind, key string, length int) {
	kind, rest, found := strings.Cut(name, "_")
	if !found {
		return kind, "", 0
	}
	if generationKinds[kind] {
		if lenPart, keyPart, found := strings.Cut(rest, "_"); found {
			if n, err := strconv.Atoi(lenPart); err == nil {
				return kind, keyPart, n
			}
		} else if n, err := strconv.Atoi(rest); err == nil {
			return kind, "", n
		}
	}
	return kind, rest, 0
}

const (
	defaultPasswordLength = 32
	defaultUsernameLength = 16
	defaultHexLength      = 64
	defaultBase64Length   = 32
)

// GenerateValue produces a real value for a generatable MagicVar kind.
// length <= 0 uses each kind's own sane default.
func GenerateValue(kind string, length int) (string, error) {
	switch kind {
	case "PASSWORD":
		return randomString(alphanumeric, orDefault(length, defaultPasswordLength))
	case "USER":
		return randomString(lowerAlphanumeric, orDefault(length, defaultUsernameLength))
	case "HEX":
		return randomHex(orDefault(length, defaultHexLength))
	case "BASE64", "REALBASE64":
		return randomBase64(orDefault(length, defaultBase64Length))
	default:
		return "", fmt.Errorf("compose: %q is not a generatable magic-var kind", kind)
	}
}

func orDefault(length, def int) int {
	if length <= 0 {
		return def
	}
	return length
}

const (
	alphanumeric      = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"
	lowerAlphanumeric = "abcdefghijklmnopqrstuvwxyz0123456789"
)

func randomString(alphabet string, length int) (string, error) {
	buf := make([]byte, length)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("compose: generate random string: %w", err)
	}
	out := make([]byte, length)
	for i, b := range buf {
		out[i] = alphabet[int(b)%len(alphabet)]
	}
	return string(out), nil
}

func randomHex(length int) (string, error) {
	buf := make([]byte, (length+1)/2)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("compose: generate random hex: %w", err)
	}
	return hex.EncodeToString(buf)[:length], nil
}

// randomBase64 generates enough random bytes that URL-safe,
// unpadded base64 encoding produces at least length characters, then
// trims to exactly length: the exact byte/char ratio doesn't need to
// match any particular convention, only "long enough to be a real
// secret."
func randomBase64(length int) (string, error) {
	buf := make([]byte, length)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("compose: generate random base64: %w", err)
	}
	enc := base64.RawURLEncoding.EncodeToString(buf)
	if len(enc) < length {
		return enc, nil
	}
	return enc[:length], nil
}
