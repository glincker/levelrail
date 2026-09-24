package main

import (
	"sort"
	"strings"
)

// envFileEntry is one parsed key/value pair from a .env-format file.
type envFileEntry struct {
	Key   string
	Value string
}

// parseEnvFileBytes parses .env-format text into entries in file order
// (duplicates preserved, the caller decides which wins). It handles
// comments, an "export " prefix, single and double quotes, multiline
// quoted values, inline " #" comments on unquoted values, and CRLF.
func parseEnvFileBytes(data []byte) []envFileEntry {
	s := strings.ReplaceAll(strings.TrimPrefix(string(data), "\xef\xbb\xbf"), "\r\n", "\n")
	var entries []envFileEntry
	for s != "" {
		var line string
		line, s = cutEnvLine(s)
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if rest, ok := strings.CutPrefix(line, "export "); ok {
			line = strings.TrimSpace(rest)
		}
		eq := strings.Index(line, "=")
		if eq == -1 {
			continue
		}
		key := strings.TrimSpace(line[:eq])
		if !validEnvKey(key) {
			continue
		}
		var value string
		value, s = parseEnvValue(strings.TrimSpace(line[eq+1:]), s)
		entries = append(entries, envFileEntry{Key: key, Value: value})
	}
	return entries
}

func cutEnvLine(s string) (line, rest string) {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i], s[i+1:]
	}
	return s, ""
}

func validEnvKey(k string) bool {
	if k == "" {
		return false
	}
	for i, r := range k {
		alpha := r == '_' || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')
		digit := r >= '0' && r <= '9'
		rest := i > 0 && (digit || r == '.' || r == '-')
		if !alpha && !rest {
			return false
		}
	}
	return true
}

// parseEnvValue parses the text after "=" (already left-trimmed) and
// returns the value plus the input remaining after the value's last line.
// A quoted value may continue onto following lines held in rest.
func parseEnvValue(v, rest string) (value, remaining string) {
	if v != "" && (v[0] == '"' || v[0] == '\'') {
		q := v[0]
		full := v[1:] + "\n" + rest
		if end := closingQuote(full, q); end >= 0 {
			raw := full[:end]
			_, remaining = cutEnvLine(full[end+1:])
			if q == '"' {
				raw = unescapeDoubleQuoted(raw)
			}
			return raw, remaining
		}
	}
	for _, sep := range []string{" #", "\t#"} {
		if i := strings.Index(v, sep); i >= 0 {
			v = v[:i]
		}
	}
	return strings.TrimSpace(v), rest
}

func closingQuote(s string, q byte) int {
	for i := 0; i < len(s); i++ {
		if q == '"' && s[i] == '\\' {
			i++
			continue
		}
		if s[i] == q {
			return i
		}
	}
	return -1
}

var doubleQuoteUnescaper = strings.NewReplacer(`\n`, "\n", `\r`, "\r", `\t`, "\t", `\"`, `"`, `\\`, `\`)

func unescapeDoubleQuoted(s string) string { return doubleQuoteUnescaper.Replace(s) }

// renderDotenv formats env as .env text with sorted keys. Every key in
// secretKeys is written empty with a comment: secret values are never
// exported, whether or not the caller holds them.
func renderDotenv(env map[string]string, secretKeys []string) string {
	secrets := make(map[string]bool, len(secretKeys))
	for _, k := range secretKeys {
		secrets[k] = true
	}
	keys := make([]string, 0, len(env)+len(secretKeys))
	for k := range env {
		if !secrets[k] {
			keys = append(keys, k)
		}
	}
	for k := range secrets {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var b strings.Builder
	for _, k := range keys {
		if secrets[k] {
			b.WriteString("# secret, value not exported\n" + k + "=\n")
			continue
		}
		b.WriteString(k + "=" + quoteDotenvValue(env[k]) + "\n")
	}
	return b.String()
}

func quoteDotenvValue(v string) string {
	if v == "" || (v == strings.TrimSpace(v) && !strings.ContainsAny(v, "\"'#\n\r\\ \t")) {
		return v
	}
	r := strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`, "\r", `\r`, "\t", `\t`)
	return `"` + r.Replace(v) + `"`
}

// envImportPlan classifies incoming .env entries against an app's current
// plain env. Later duplicates of a key win.
type envImportPlan struct {
	New       []string          `json:"new"`
	Changed   []string          `json:"changed"`
	Unchanged []string          `json:"unchanged"`
	Secret    []string          `json:"skipped_secret"`
	Merged    map[string]string `json:"-"`
}

func planEnvImport(current map[string]string, entries []envFileEntry, secretKeys []string, keepExisting bool) envImportPlan {
	secrets := make(map[string]bool, len(secretKeys))
	for _, k := range secretKeys {
		secrets[k] = true
	}
	incoming := map[string]string{}
	var order []string
	for _, e := range entries {
		if _, seen := incoming[e.Key]; !seen {
			order = append(order, e.Key)
		}
		incoming[e.Key] = e.Value
	}

	plan := envImportPlan{Merged: make(map[string]string, len(current)+len(order))}
	for k, v := range current {
		plan.Merged[k] = v
	}
	for _, k := range order {
		cur, exists := current[k]
		switch {
		case secrets[k]:
			plan.Secret = append(plan.Secret, k)
		case !exists:
			plan.New = append(plan.New, k)
			plan.Merged[k] = incoming[k]
		case cur == incoming[k]:
			plan.Unchanged = append(plan.Unchanged, k)
		default:
			plan.Changed = append(plan.Changed, k)
			if !keepExisting {
				plan.Merged[k] = incoming[k]
			}
		}
	}
	return plan
}
