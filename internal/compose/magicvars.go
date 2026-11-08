package compose

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"regexp"
	"sort"
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

// ExpandOption configures ExpandBuildService.
type ExpandOption func(*expandConfig)

type expandConfig struct {
	generate func(kind, key string, length int) (string, error)
	persist  func(serviceKey, envKey, value string) error
	cache    map[string]string
}

// WithMagicSecrets lets ExpandBuildService resolve generated SERVICE_
// variables with the same generate and persist hooks ResolveMagicVars
// takes. Without it, a generated variable is rejected (fail closed).
func WithMagicSecrets(generate func(kind, key string, length int) (string, error), persist func(serviceKey, envKey, value string) error) ExpandOption {
	return func(c *expandConfig) { c.generate, c.persist = generate, persist }
}

// resolveEnv expands every SERVICE_ token in one service's environment.
// Domain variables never need storage; generated ones become secret-backed
// keys whose value is handed to persist and kept out of the returned map.
func (c *expandConfig) resolveEnv(f *File, svcKey string, env Environment) (map[string]string, map[string]bool, []string, error) {
	out := make(map[string]string, len(env))
	var secretKeys map[string]bool
	var warnings []string
	for _, envKey := range sortedEnvKeys(env) {
		value := env[envKey]
		secret := false
		for _, v := range FindMagicVars(value) {
			val, isSecret, warn, err := c.resolveOne(f, svcKey, envKey, v)
			if err != nil {
				return nil, nil, nil, err
			}
			if warn != "" {
				warnings = append(warnings, warn)
			}
			secret = secret || isSecret
			value = strings.Replace(value, v.Token, val, 1)
		}
		if secret {
			if err := c.persist(svcKey, envKey, value); err != nil {
				return nil, nil, nil, fmt.Errorf("compose: service %q: env %q: persist generated value: %w", svcKey, envKey, err)
			}
			if secretKeys == nil {
				secretKeys = map[string]bool{}
			}
			secretKeys[envKey] = true
			out[envKey] = ""
			continue
		}
		out[envKey] = value
	}
	return out, secretKeys, warnings, nil
}

func (c *expandConfig) resolveOne(f *File, svcKey, envKey string, v MagicVar) (val string, secret bool, warning string, err error) {
	switch {
	case v.Kind == "FQDN" || v.Kind == "URL":
		target := svcKey
		if k := matchServiceKey(f, stripPortSuffix(v.Key)); k != "" {
			target = k
		}
		domain := f.Domains[target]
		if domain == "" {
			domain = f.RuntimeDomains[target]
		}
		if domain != "" && !strings.Contains(domain, "*") {
			if v.Kind == "URL" {
				return "https://" + domain, false, "", nil
			}
			return domain, false, "", nil
		}
		if v.HasDefault {
			return v.Default, false, "", nil
		}
		warning = fmt.Sprintf("service %q: env %q: %s has no domain assigned, using the internal name %q", svcKey, envKey, v.Token, target)
		if v.Kind == "URL" {
			// Keep the port the token named so the fallback still reaches the service.
			if i := strings.LastIndex(v.Key, "_"); i >= 0 && stripPortSuffix(v.Key) != v.Key {
				return "http://" + target + ":" + v.Key[i+1:], false, warning, nil
			}
			return "http://" + target, false, warning, nil
		}
		return target, false, warning, nil
	case v.Generatable:
		if c.generate == nil || c.persist == nil {
			return "", false, "", fmt.Errorf("compose: service %q: env %q: magic var %q is not supported here: generated variables need secret storage, which this deploy path does not provide", svcKey, envKey, v.Token)
		}
		ck := fmt.Sprintf("%s_%s_%d", v.Kind, v.Key, v.Length)
		if cached, ok := c.cache[ck]; ok {
			return cached, true, "", nil
		}
		gen, gerr := c.generate(v.Kind, v.Key, v.Length)
		if gerr != nil {
			return "", false, "", fmt.Errorf("compose: service %q: env %q: %w", svcKey, envKey, gerr)
		}
		if c.cache == nil {
			c.cache = map[string]string{}
		}
		c.cache[ck] = gen
		return gen, true, "", nil
	case v.HasDefault:
		return v.Default, false, "", nil
	}
	return "", false, "", fmt.Errorf("compose: service %q: env %q: magic var %q cannot be resolved, use a literal value or a ${...:-default}", svcKey, envKey, v.Token)
}

// matchServiceKey finds the compose service a SERVICE_FQDN_<NAME> token
// names, comparing case-insensitively with "-" read as "_".
func matchServiceKey(f *File, name string) string {
	want := strings.ToUpper(name)
	for _, k := range sortedServiceNames(f) {
		if strings.ToUpper(strings.ReplaceAll(k, "-", "_")) == want {
			return k
		}
	}
	return ""
}

func sortedEnvKeys(env Environment) []string {
	keys := make([]string, 0, len(env))
	for k := range env {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
