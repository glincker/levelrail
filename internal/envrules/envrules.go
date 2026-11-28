// Package envrules holds the one set of rules for env keys and values. Every
// writer (API, CLI pre-check, import, templates, clone) calls it so a key is
// accepted or refused the same way on every surface.
package envrules

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// Code names why a key or value was flagged. UI and CLI map codes to text.
type Code string

// Codes returned in Issue.Code.
const (
	CodeEmptyKey      Code = "empty_key"
	CodeEquals        Code = "equals"
	CodeCharset       Code = "charset"
	CodeLength        Code = "length"
	CodeReserved      Code = "reserved"
	CodePrevious      Code = "previous_suffix"
	CodeDangerous     Code = "dangerous"
	CodeDuplicate     Code = "duplicate"
	CodeNUL           Code = "nul"
	CodeValueSize     Code = "value_size"
	CodeCRLF          Code = "crlf"
	CodeMultiline     Code = "multiline"
	CodeWhitespace    Code = "whitespace"
	CodeInterpolation Code = "interpolation"
	CodeEmptyValue    Code = "empty_value"
	CodeLegacyKey     Code = "legacy_key"
)

// Key policy and refuse-or-warn modes, matching the runtime setting values.
const (
	KeyModeStrict = "strict"
	KeyModeLegacy = "legacy"
	ModeRefuse    = "refuse"
	ModeWarn      = "warn"
	ModeOff       = "off"
)

// Issue is one finding for one key. Values never appear in it.
type Issue struct {
	Key     string `json:"key"`
	Code    Code   `json:"code"`
	Message string `json:"message"`
}

// Policy carries the runtime-setting driven limits.
type Policy struct {
	KeyMode          string   `json:"key_mode"`
	KeyMaxLen        int      `json:"key_max_len"`
	ValueMaxBytes    int      `json:"value_max_bytes"`
	ReservedPrefixes []string `json:"reserved_prefixes"`
	LoaderMode       string   `json:"loader_mode"`
	PathMode         string   `json:"path_mode"`
}

// DefaultPolicy matches the seeded runtime-setting defaults.
func DefaultPolicy() Policy {
	return Policy{
		KeyMode: KeyModeLegacy, KeyMaxLen: 255, ValueMaxBytes: 65536,
		ReservedPrefixes: []string{"LEVELRAIL_"}, LoaderMode: ModeRefuse, PathMode: ModeWarn,
	}
}

// Options describe one write.
type Options struct {
	// Secret applies the rules for secret keys (the _PREVIOUS grace suffix is reserved).
	Secret bool `json:"secret"`
	// Multiline marks a path that carries values byte for byte (JSON bodies),
	// so a trailing CR is a real value and not a CRLF line-ending leak.
	Multiline bool `json:"multiline"`
	// ExistingUnknown is set by clients that cannot see the stored keys; the
	// server decides about legacy keys.
	ExistingUnknown bool `json:"existing_unknown"`
}

// Existing is what the app already holds. Plain maps key to current value so
// an unchanged key is skipped; Names adds keys with no readable value (secrets).
type Existing struct {
	Plain map[string]string `json:"plain"`
	Names []string          `json:"names"`
}

// Result separates refusals from warnings.
type Result struct {
	Errors   []Issue `json:"errors"`
	Warnings []Issue `json:"warnings"`
}

// Err returns the first refusal as one sentence, or "".
func (r Result) Err() string {
	if len(r.Errors) == 0 {
		return ""
	}
	return r.Errors[0].Message
}

var (
	strictKey = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
	legacyKey = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_.-]*$`)
	interp    = regexp.MustCompile(`\$(\$|\{|[A-Za-z_])`)
)

var loaderKeys = map[string]bool{
	"LD_PRELOAD": true, "LD_LIBRARY_PATH": true, "LD_AUDIT": true, "LD_PROFILE": true,
	"BASH_ENV": true, "SHELLOPTS": true, "BASHOPTS": true,
}

// PreviousSuffix mirrors the secret grace key suffix owned by secret rotation.
const PreviousSuffix = "_PREVIOUS"

// Validate checks every key and value in set. Keys already stored with the
// same value are skipped, so an old legacy entry never blocks an unrelated save.
func Validate(set map[string]string, ex Existing, p Policy, o Options) Result {
	var res Result
	keys := make([]string, 0, len(set))
	for k := range set {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	known := map[string]bool{}
	for k := range ex.Plain {
		known[k] = true
	}
	for _, k := range ex.Names {
		known[k] = true
	}
	lower := map[string]string{}
	for k := range known {
		lower[strings.ToLower(k)] = k
	}
	seen := map[string]string{}
	for _, k := range keys {
		v := set[k]
		if old, ok := ex.Plain[k]; ok && old == v && !o.Secret {
			continue
		}
		if prev, dup := seen[strings.ToLower(k)]; dup {
			res.err(k, CodeDuplicate, fmt.Sprintf("%s and %s differ only by case; keep one", prev, k))
		}
		seen[strings.ToLower(k)] = k
		if other, ok := lower[strings.ToLower(k)]; ok && other != k {
			res.err(k, CodeDuplicate, fmt.Sprintf("%s differs only by case from the existing key %s; use that name", k, other))
		}
		checkKey(&res, k, known[k], p, o)
		checkValue(&res, k, v, p, o)
	}
	return res
}

// ValidateKeys checks keys that carry no value (unset, delete, convert): only
// the shape rules that make a name unusable, so an old odd key can still be removed.
func ValidateKeys(keys []string) Result {
	var res Result
	for _, k := range keys {
		switch {
		case k == "":
			res.err(k, CodeEmptyKey, "env keys must not be empty")
		case strings.ContainsAny(k, "=\x00"):
			res.err(k, CodeEquals, k+": env keys must not contain = or NUL")
		}
	}
	return res
}

func checkKey(res *Result, k string, exists bool, p Policy, o Options) {
	switch {
	case k == "":
		res.err(k, CodeEmptyKey, "env keys must not be empty")
		return
	case strings.ContainsAny(k, "=\x00"):
		res.err(k, CodeEquals, k+": env keys must not contain = or NUL")
		return
	case p.KeyMaxLen > 0 && len(k) > p.KeyMaxLen:
		res.err(k, CodeLength, fmt.Sprintf("%s: key is longer than %d characters", short(k), p.KeyMaxLen))
		return
	}
	if !strictKey.MatchString(k) {
		legacyOK := legacyKey.MatchString(k) && p.KeyMode != KeyModeStrict && (exists || o.ExistingUnknown)
		if !legacyOK {
			res.err(k, CodeCharset, k+": use letters, digits and underscore, not starting with a digit")
			return
		}
		res.warn(k, CodeLegacyKey, k+": has a dot or dash; kept because it already exists, new keys cannot")
	}
	if !exists {
		for _, pre := range p.ReservedPrefixes {
			if pre != "" && strings.HasPrefix(strings.ToUpper(k), strings.ToUpper(pre)) {
				res.err(k, CodeReserved, k+": the "+pre+" prefix is reserved for the platform")
				return
			}
		}
	}
	if o.Secret && strings.HasSuffix(k, PreviousSuffix) {
		res.err(k, CodePrevious, k+": the "+PreviousSuffix+" suffix is reserved for secret rotation")
		return
	}
	checkDangerous(res, k, p)
}

func checkDangerous(res *Result, k string, p Policy) {
	if loaderKeys[k] || strings.HasPrefix(k, "DYLD_") {
		msg := k + ": changes how every process loads code and can break or hijack the app"
		if p.LoaderMode == ModeWarn {
			res.warn(k, CodeDangerous, msg)
		} else {
			res.err(k, CodeDangerous, msg)
		}
		return
	}
	if k == "PATH" && p.PathMode != ModeOff {
		msg := "PATH: overriding it can hide the commands the image needs"
		if p.PathMode == ModeRefuse {
			res.err(k, CodeDangerous, msg)
		} else {
			res.warn(k, CodeDangerous, msg)
		}
	}
}

func checkValue(res *Result, k, v string, p Policy, o Options) {
	switch {
	case strings.ContainsRune(v, 0):
		res.err(k, CodeNUL, k+": the value contains a NUL byte")
		return
	case p.ValueMaxBytes > 0 && len(v) > p.ValueMaxBytes:
		res.err(k, CodeValueSize, fmt.Sprintf("%s: the value is longer than %d bytes", k, p.ValueMaxBytes))
		return
	case !o.Multiline && strings.HasSuffix(v, "\r"):
		res.err(k, CodeCRLF, k+": the value ends with a carriage return, usually a CRLF file; fix the line endings")
		return
	}
	if v == "" {
		res.warn(k, CodeEmptyValue, k+": set to an empty string, which is not the same as unset; use unset to remove it")
		return
	}
	if strings.ContainsAny(v, "\r\n") {
		res.warn(k, CodeMultiline, k+": the value spans several lines")
	}
	if strings.TrimSpace(v) != v {
		res.warn(k, CodeWhitespace, k+": the value has leading or trailing whitespace, which is kept as is")
	}
	if interp.MatchString(v) {
		res.warn(k, CodeInterpolation, k+": the value contains $ sequences that compose may expand; write $$ for a literal $")
	}
}

func short(k string) string {
	if len(k) > 40 {
		return k[:40] + "..."
	}
	return k
}

func (r *Result) err(k string, c Code, m string) {
	r.Errors = append(r.Errors, Issue{Key: k, Code: c, Message: m})
}

func (r *Result) warn(k string, c Code, m string) {
	r.Warnings = append(r.Warnings, Issue{Key: k, Code: c, Message: m})
}
