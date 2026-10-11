package appwizard

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/GLINCKER/levelrail/internal/importplan"
)

const sourceREADME = "README.md"

var (
	readmeAssignRe = regexp.MustCompile(`^\s*(?:export\s+)?([A-Z][A-Z0-9_]{2,})=`)
	readmeTableRe  = regexp.MustCompile("^\\|\\s*`?([A-Z][A-Z0-9_]{2,})`?\\s*\\|")
	composeFiles   = []string{"docker-compose.yml", "docker-compose.yaml", "compose.yaml", "compose.yml"}
)

// ignoredEnvKeys are set by the platform or tooling, not by the operator.
var ignoredEnvKeys = map[string]bool{"PORT": true, "HOSTNAME": true, "HOST": true, "PATH": true}

type composeDoc struct {
	Services map[string]struct {
		Image       string `yaml:"image"`
		Environment any    `yaml:"environment"`
	} `yaml:"services"`
}

func toKey(e importplan.EnvVar) EnvKey {
	return EnvKey{Key: e.Key, Required: e.Required, Secret: e.Secret, Source: e.Source, HasDefault: e.HasDefault}
}

// envFromExampleFiles reads the example env files under each directory.
// Values are dropped; only keys, secrecy and required-ness survive.
func envFromExampleFiles(read func(p string) (string, bool), dirs []string) []EnvKey {
	var out []EnvKey
	for _, d := range dirs {
		for _, f := range importplan.EnvExampleFiles {
			p := joinPath(d, f)
			text, ok := read(p)
			if !ok {
				continue
			}
			for _, e := range importplan.ParseEnvFile(text, p) {
				out = append(out, toKey(e))
			}
		}
	}
	return out
}

// parseCompose returns the compose document and the path it came from.
func parseCompose(read func(p string) (string, bool), dirs []string) (composeDoc, string, bool) {
	for _, d := range dirs {
		for _, f := range composeFiles {
			p := joinPath(d, f)
			text, ok := read(p)
			if !ok {
				continue
			}
			var doc composeDoc
			if err := yaml.Unmarshal([]byte(text), &doc); err != nil || len(doc.Services) == 0 {
				continue
			}
			return doc, p, true
		}
	}
	return composeDoc{}, "", false
}

// envFromCompose lists env keys of services that are not a known database image.
func envFromCompose(doc composeDoc, source string) []EnvKey {
	names := make([]string, 0, len(doc.Services))
	for n := range doc.Services {
		names = append(names, n)
	}
	sort.Strings(names)
	var out []EnvKey
	for _, n := range names {
		svc := doc.Services[n]
		if _, isDB := engineForImage(svc.Image); isDB {
			continue
		}
		var lines []string
		switch env := svc.Environment.(type) {
		case map[string]any:
			for k, v := range env {
				if v == nil {
					lines = append(lines, k+"=")
				} else {
					lines = append(lines, fmt.Sprintf("%s=%v", k, v))
				}
			}
		case []any:
			for _, item := range env {
				s := fmt.Sprint(item)
				if !strings.Contains(s, "=") {
					s += "="
				}
				lines = append(lines, s)
			}
		}
		sort.Strings(lines)
		for _, e := range importplan.ParseEnvFile(strings.Join(lines, "\n"), source) {
			out = append(out, toKey(e))
		}
	}
	return out
}

// envFromREADME reads keys from fenced blocks and tables, never values.
func envFromREADME(text string, limit int) []EnvKey {
	var out []EnvKey
	inFence := false
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			inFence = !inFence
			continue
		}
		var key string
		if m := readmeTableRe.FindStringSubmatch(line); m != nil {
			key = m[1]
		} else if inFence {
			if m := readmeAssignRe.FindStringSubmatch(line); m != nil {
				key = m[1]
			}
		}
		if key == "" {
			continue
		}
		out = append(out, EnvKey{Key: key, Secret: importplan.KeyLooksSecret(key), Source: sourceREADME})
		if len(out) >= limit {
			break
		}
	}
	return out
}

// mergeKeys keeps the first occurrence of each key, drops platform-managed
// keys, and caps the result.
func mergeKeys(limit int, lists ...[]EnvKey) []EnvKey {
	seen := map[string]bool{}
	var out []EnvKey
	for _, l := range lists {
		for _, k := range l {
			if seen[k.Key] || ignoredEnvKeys[k.Key] {
				continue
			}
			seen[k.Key] = true
			out = append(out, k)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Required != out[j].Required {
			return out[i].Required
		}
		return out[i].Key < out[j].Key
	})
	if len(out) > limit {
		out = out[:limit]
	}
	return out
}
