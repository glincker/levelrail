package importplan

import (
	"bufio"
	"regexp"
	"sort"
	"strings"
)

var (
	secretKeyRe   = regexp.MustCompile(`(?i)(SECRET|TOKEN|PASSWORD|PASSWD|PASSPHRASE|API_?KEY|PRIVATE_?KEY|ACCESS_?KEY|CREDENTIAL|AUTH|SALT|SIGNING)`)
	credURLRe     = regexp.MustCompile(`^[a-z][a-z0-9+.-]*://[^/\s:@]+:[^/\s@]+@`)
	placeholderRe = regexp.MustCompile(`(?i)^(changeme|change_me|change-me|todo|xxx+|your[_-].*|.*[_-]here|replace[_-]?me|<.*>|\$\{.*\}|\{\{.*\}\}|secret|password|example)$`)
	envKeyRe      = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
)

// EnvExampleFiles are the repo files scanned for env variable declarations.
var EnvExampleFiles = []string{".env.example", ".env.sample", ".env.template", "env.example", ".env.dist"}

// isPlaceholder reports whether v is empty or an obvious fill-me-in value.
func isPlaceholder(v string) bool {
	v = strings.TrimSpace(v)
	return v == "" || placeholderRe.MatchString(v)
}

// looksSecret guesses from the key and value whether a variable is sensitive.
func looksSecret(key, value string) bool {
	return secretKeyRe.MatchString(key) || credURLRe.MatchString(value)
}

// newEnvVar builds an EnvVar, applying the required/default/secret rules.
// A secret-looking default is never copied into Value.
func newEnvVar(key, value, source string) EnvVar {
	secret := looksSecret(key, value)
	placeholder := isPlaceholder(value)
	v := EnvVar{Key: key, Secret: secret, Source: source, Required: placeholder}
	if !placeholder {
		v.HasDefault = true
		if !secret {
			v.Value = value
		}
	}
	return v
}

// ParseEnvFile parses KEY=VALUE lines (comments, `export`, quotes handled).
// Values are treated as data only and are never interpolated.
func ParseEnvFile(text, source string) []EnvVar {
	var out []EnvVar
	sc := bufio.NewScanner(strings.NewReader(text))
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimSpace(strings.TrimPrefix(line, "export "))
		k, v, ok := strings.Cut(line, "=")
		k = strings.TrimSpace(k)
		if !ok || !envKeyRe.MatchString(k) {
			continue
		}
		out = append(out, newEnvVar(k, unquoteEnv(v), source))
	}
	return out
}

func unquoteEnv(v string) string {
	v = strings.TrimSpace(v)
	if len(v) >= 2 && (v[0] == '"' || v[0] == '\'') {
		if end := strings.LastIndexByte(v, v[0]); end > 0 {
			return v[1:end]
		}
	}
	if i := strings.Index(v, " #"); i >= 0 {
		v = strings.TrimSpace(v[:i])
	}
	return v
}

// mergeEnv combines lists, first occurrence of a key winning, sorted by key.
func mergeEnv(lists ...[]EnvVar) []EnvVar {
	seen := map[string]bool{}
	var out []EnvVar
	for _, l := range lists {
		for _, e := range l {
			if seen[e.Key] {
				continue
			}
			seen[e.Key] = true
			out = append(out, e)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out
}

// missingRequired lists required variable keys across the services.
func missingRequired(services []ServicePlan) []string {
	var out []string
	for _, s := range services {
		for _, e := range s.Env {
			if e.Required {
				out = append(out, e.Key)
			}
		}
	}
	sort.Strings(out)
	return dedupe(out)
}

func dedupe(in []string) []string {
	var out []string
	for i, s := range in {
		if i == 0 || s != in[i-1] {
			out = append(out, s)
		}
	}
	return out
}
