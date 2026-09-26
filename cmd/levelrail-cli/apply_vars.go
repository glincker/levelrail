package main

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

// varSources are the only places a ${{ env.NAME }} placeholder may be filled
// from. The process environment is read only for names on the allowlist, so a
// shared resource file cannot pull arbitrary variables out of the shell.
type varSources struct {
	vars     map[string]string
	varFiles []string
	allowEnv []string
}

var credentialPrefixes = []string{"AWS_", "AZURE_", "GOOGLE_", "GCP_", "SSH_", "ARM_", "ACTIONS_", "DIGITALOCEAN_", "CLOUDFLARE_API_"}

var credentialNames = map[string]bool{
	"GITHUB_TOKEN": true, "GH_TOKEN": true, "GITLAB_TOKEN": true, "NPM_TOKEN": true,
	"CI_JOB_TOKEN": true,
}

// looksLikeCredential reports names that hold cloud or CI credentials; these
// are read from the environment only when --allow-env names them exactly.
func looksLikeCredential(name string) bool {
	up := strings.ToUpper(name)
	if credentialNames[up] || strings.HasSuffix(up, "_SECRET_ACCESS_KEY") {
		return true
	}
	for _, p := range credentialPrefixes {
		if strings.HasPrefix(up, p) {
			return true
		}
	}
	return false
}

type envAllowlist struct {
	exact    map[string]bool
	prefixes []string
}

func parseAllowEnv(specs []string) (envAllowlist, error) {
	al := envAllowlist{exact: map[string]bool{}}
	for _, spec := range specs {
		for _, raw := range strings.Split(spec, ",") {
			name := strings.TrimSpace(raw)
			if prefix, ok := strings.CutSuffix(name, "*"); ok && prefix != "" && validEnvKey(prefix) {
				al.prefixes = append(al.prefixes, prefix)
				continue
			}
			if !validEnvKey(name) {
				return envAllowlist{}, newValidationError("--allow-env: %q is not a variable name (use NAME or PREFIX*)", raw)
			}
			al.exact[name] = true
		}
	}
	return al, nil
}

// permits reports whether name may be read from the environment and, if not,
// why a wildcard match was refused.
func (al envAllowlist) permits(name string) (bool, string) {
	if al.exact[name] {
		return true, ""
	}
	for _, p := range al.prefixes {
		if strings.HasPrefix(name, p) {
			if looksLikeCredential(name) {
				return false, "looks like a cloud or CI credential, so a wildcard does not cover it"
			}
			return true, ""
		}
	}
	return false, ""
}

func readVarFile(path string) (map[string]string, error) {
	data, err := os.ReadFile(path) //nolint:gosec // the operator names the var file
	if err != nil {
		return nil, fmt.Errorf("--var-file: read %s: %w", path, err)
	}
	out := map[string]string{}
	for _, e := range parseEnvFileBytes(data) {
		out[e.Key] = e.Value
	}
	return out, nil
}

func placeholderNames(files []apiclient.IaCFile) []string {
	seen := map[string]bool{}
	var names []string
	for _, f := range files {
		for _, m := range envPlaceRe.FindAllStringSubmatch(f.Content, -1) {
			if !seen[m[1]] {
				seen[m[1]] = true
				names = append(names, m[1])
			}
		}
	}
	sort.Strings(names)
	return names
}

// resolveVars fills every placeholder the files use from, in order, --var,
// --var-file and allowlisted environment variables. Any name left over is an
// error, raised before anything is sent to the control plane.
func resolveVars(files []apiclient.IaCFile, src varSources, lookupEnv func(string) (string, bool)) (map[string]string, error) {
	fromFiles := map[string]string{}
	for _, p := range src.varFiles {
		m, err := readVarFile(p)
		if err != nil {
			return nil, err
		}
		for k, v := range m {
			fromFiles[k] = v
		}
	}
	allow, err := parseAllowEnv(src.allowEnv)
	if err != nil {
		return nil, err
	}
	vars := map[string]string{}
	var missing []string
	for _, name := range placeholderNames(files) {
		if v, ok := src.vars[name]; ok {
			vars[name] = v
			continue
		}
		if v, ok := fromFiles[name]; ok {
			vars[name] = v
			continue
		}
		ok, why := allow.permits(name)
		if ok {
			if v, set := lookupEnv(name); set {
				vars[name] = v
				continue
			}
			why = "is allowed but not set in the environment"
		}
		missing = append(missing, unresolvedLine(name, why))
	}
	if len(missing) > 0 {
		return nil, newValidationError("unresolved ${{ env.NAME }} placeholders, nothing was sent:\n%s\nThe environment is only read for names listed with --allow-env.", strings.Join(missing, "\n"))
	}
	return vars, nil
}

func unresolvedLine(name, why string) string {
	line := fmt.Sprintf("  %s: pass --var %s=VALUE, put it in a --var-file, or --allow-env %s", name, name, name)
	if why != "" {
		line += " (" + name + " " + why + ")"
	}
	return line
}
