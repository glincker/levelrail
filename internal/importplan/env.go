package importplan

import (
	"bufio"
	"regexp"
	"sort"
	"strings"

	"github.com/glincker/levelrail/internal/envrules"
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
	vars, _ := ParseEnvFileReport(text, source)
	return vars
}

// ParseEnvFileReport is ParseEnvFile plus a per-key report: every refused line
// is named with its reason instead of being skipped silently. New keys follow
// the strict env rules, the same ones every writer applies.
func ParseEnvFileReport(text, source string) ([]EnvVar, envrules.Report) {
	var vars []EnvVar
	set := map[string]string{}
	var bad []envrules.Rejection
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
		if !ok {
			bad = append(bad, envrules.Rejection{Key: k, Code: "not_assignment", Reason: "line is not KEY=VALUE"})
			continue
		}
		val := unquoteEnv(v)
		set[k] = val
		vars = append(vars, newEnvVar(k, val, source))
	}
	pol := envrules.DefaultPolicy()
	pol.KeyMode = envrules.KeyModeStrict
	res := envrules.Validate(set, envrules.Existing{}, pol, envrules.Options{Multiline: true})
	rep := envrules.BuildReport(set, res)
	rep.Rejected = append(rep.Rejected, bad...)
	refused := map[string]bool{}
	for _, r := range res.Errors {
		refused[r.Key] = true
	}
	out := vars[:0]
	for _, e := range vars {
		if !refused[e.Key] {
			out = append(out, e)
		}
	}
	return out, rep
}

// MergeReports combines the reports of several env example files.
func MergeReports(reps ...envrules.Report) envrules.Report {
	out := envrules.Report{Accepted: []string{}, Rejected: []envrules.Rejection{}, Warnings: []envrules.Issue{}}
	for _, r := range reps {
		out.Accepted = append(out.Accepted, r.Accepted...)
		out.Rejected = append(out.Rejected, r.Rejected...)
		out.Warnings = append(out.Warnings, r.Warnings...)
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
