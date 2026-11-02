package compose

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/GLINCKER/levelrail/kit/ids"
)

// generationKinds are the SERVICE_<KIND>_... prefixes a real template
// catalog's own convention uses for "auto-generate this at deploy
// time" (a random password, username, or encoded secret). Every other
// SERVICE_ name (FQDN, or an arbitrary third-party credential name) is
// either satisfied by its own bash-style ${...:-default} or left
// unresolved for the caller to reject.
var generationKinds = map[string]bool{
	"PASSWORD":    true,
	"USER":        true,
	"BASE64":      true,
	"HEX":         true,
	"REALBASE64":  true,
	"BASE64BYTES": true,
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
	defaultBase64Bytes    = 32
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
	case "BASE64BYTES":
		return randomBase64Bytes(orDefault(length, defaultBase64Bytes))
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
	s, err := ids.HexLen(length)
	if err != nil {
		return "", fmt.Errorf("compose: generate random hex: %w", err)
	}
	return s, nil
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

// randomBase64Bytes returns standard padded base64 of n random bytes, the
// shape frameworks that decode their key (e.g. a 32 byte AES key) expect.
func randomBase64Bytes(n int) (string, error) {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("compose: generate random base64 bytes: %w", err)
	}
	return base64.StdEncoding.EncodeToString(buf), nil
}

// Translation is the result of TranslateMagicVars. Generated names the
// secret variables that got a fresh value; the values stay in Compose only.
type Translation struct {
	Compose     string
	Generated   []string
	Hosts       []string
	Unsupported []string
}

var (
	magicDeclList = regexp.MustCompile(`(?m)^(\s*-\s+)(SERVICE_[A-Za-z0-9_]+)[ \t]*$`)
	magicDeclMap  = regexp.MustCompile(`(?m)^(\s*)(SERVICE_[A-Za-z0-9_]+):[ \t]*(?:""|'')?[ \t]*$`)
)

// TranslateMagicVars rewrites Coolify SERVICE_FQDN_*, SERVICE_URL_* and
// generatable SERVICE_* tokens in a compose body into literal values, so the
// result has no placeholder left. host maps a service key to its hostname.
// Tokens that cannot be resolved stay untouched and are listed in Unsupported.
func TranslateMagicVars(src string, host func(key string) string) (Translation, error) {
	t := Translation{}
	cache := map[string]string{}
	seenGen, seenHost, seenBad := map[string]bool{}, map[string]bool{}, map[string]bool{}
	var firstErr error

	resolve := func(m MagicVar, raw string) (string, bool) {
		switch m.Kind {
		case "FQDN", "URL":
			h := host(stripPortSuffix(m.Key))
			if !seenHost[h] {
				seenHost[h] = true
				t.Hosts = append(t.Hosts, h)
			}
			if m.Kind == "URL" {
				return "https://" + h, true
			}
			return h, true
		}
		if m.Generatable {
			ck := m.Kind + "_" + m.Key
			if v, ok := cache[ck]; ok {
				return v, true
			}
			v, err := GenerateValue(m.Kind, m.Length)
			if err != nil {
				if firstErr == nil {
					firstErr = err
				}
				return "", false
			}
			cache[ck] = v
			if !seenGen[ck] {
				seenGen[ck] = true
				t.Generated = append(t.Generated, "SERVICE_"+m.Kind+"_"+m.Key)
			}
			return v, true
		}
		if m.HasDefault {
			return m.Default, true
		}
		if !seenBad[raw] {
			seenBad[raw] = true
			t.Unsupported = append(t.Unsupported, raw)
		}
		return "", false
	}

	decl := func(re *regexp.Regexp, format string) {
		src = re.ReplaceAllStringFunc(src, func(line string) string {
			sub := re.FindStringSubmatch(line)
			vars := FindMagicVars("$" + sub[2])
			if len(vars) != 1 {
				return line
			}
			v, ok := resolve(vars[0], sub[2])
			if !ok {
				return line
			}
			return fmt.Sprintf(format, sub[1], sub[2], v)
		})
	}
	decl(magicDeclList, "%s%s=%s")
	decl(magicDeclMap, "%s%s: %q")

	src = magicVarPattern.ReplaceAllStringFunc(src, func(tok string) string {
		vars := FindMagicVars(tok)
		if len(vars) != 1 {
			return tok
		}
		if v, ok := resolve(vars[0], strings.Trim(tok, "${}")); ok {
			return v
		}
		return tok
	})
	if firstErr != nil {
		return Translation{}, firstErr
	}
	t.Compose = src
	return t, nil
}

// stripPortSuffix turns "APP_3000" into "APP"; Coolify appends the port a
// domain should route to, which the host name does not carry.
func stripPortSuffix(key string) string {
	i := strings.LastIndex(key, "_")
	if i < 0 {
		return key
	}
	if _, err := strconv.Atoi(key[i+1:]); err != nil {
		return key
	}
	return key[:i]
}
