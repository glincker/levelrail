package compose

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	envMaxEnvFileBytes     = "APP_COMPOSE_MAX_ENV_FILE_BYTES"
	defaultMaxEnvFileBytes = 256 << 10
)

// EnvFileRef is one env_file: entry. Required is false only for the long
// form with required: false, where a missing file is skipped.
type EnvFileRef struct {
	Path     string
	Required bool
}

// envFileRefsByService reads env_file: from the raw document, in the string,
// list-of-strings and list-of-{path, required} forms.
func envFileRefsByService(data []byte) (map[string][]EnvFileRef, error) {
	var doc struct {
		Services map[string]map[string]any `yaml:"services"`
	}
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, nil
	}
	out := map[string][]EnvFileRef{}
	for name, svc := range doc.Services {
		raw, ok := svc["env_file"]
		if !ok {
			continue
		}
		for _, item := range toList(raw) {
			switch t := item.(type) {
			case string:
				out[name] = append(out[name], EnvFileRef{Path: t, Required: true})
			case map[string]any:
				p, _ := t["path"].(string)
				required := true
				if b, isBool := t["required"].(bool); isBool {
					required = b
				}
				out[name] = append(out[name], EnvFileRef{Path: p, Required: required})
			default:
				return nil, fmt.Errorf("compose: service %q: env_file: entries must be a path or {path, required}", name)
			}
		}
	}
	return out, nil
}

func maxEnvFileBytes() int64 {
	return int64(envCap(envMaxEnvFileBytes, defaultMaxEnvFileBytes))
}

// ResolveEnvFiles reads each service's env_file: entries from inside root
// (composeDir is the compose file's directory relative to root) and merges
// them under environment:, which wins. Only a git checkout has a root, so
// the pasted-file path keeps the unsupported warning instead.
func (f *File) ResolveEnvFiles(root, composeDir string) error {
	names := make([]string, 0, len(f.Services))
	for name, svc := range f.Services {
		if len(svc.EnvFiles) > 0 {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	for _, name := range names {
		svc := f.Services[name]
		env := Environment{}
		for _, ref := range svc.EnvFiles {
			vals, err := readEnvFile(root, composeDir, ref)
			if err != nil {
				return fmt.Errorf("compose: service %q: env_file %q: %w", name, ref.Path, err)
			}
			for k, v := range vals {
				env[k] = v
			}
		}
		for k, v := range svc.Environment {
			env[k] = v
		}
		svc.Environment = env
		svc.EnvFiles = nil
		f.Services[name] = svc
		kept := f.Unsupported[:0]
		for _, u := range f.Unsupported {
			if u.Service == name && u.Key == "env_file" {
				continue
			}
			kept = append(kept, u)
		}
		f.Unsupported = kept
	}
	return nil
}

// One message for missing and outside-repo paths so the answer does not reveal what exists on the host.
var errEnvFileUnavailable = errors.New("file not found in the repository")

func readEnvFile(root, composeDir string, ref EnvFileRef) (map[string]string, error) {
	if ref.Path == "" {
		return nil, errors.New("path is empty")
	}
	rel := filepath.Join(composeDir, ref.Path)
	if filepath.IsAbs(ref.Path) || !filepath.IsLocal(rel) {
		return nil, errors.New("must be a relative path inside the repository")
	}
	rootReal, err := filepath.EvalSymlinks(root)
	if err != nil {
		return nil, fmt.Errorf("resolve repository root: %w", err)
	}
	resolved, err := filepath.EvalSymlinks(filepath.Join(rootReal, rel))
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			if !ref.Required {
				return nil, nil
			}
			return nil, errEnvFileUnavailable
		}
		return nil, fmt.Errorf("resolve: %w", err)
	}
	back, err := filepath.Rel(rootReal, resolved)
	if err != nil || (back != "." && !filepath.IsLocal(back)) {
		return nil, errEnvFileUnavailable
	}
	st, err := os.Stat(resolved)
	if err != nil {
		return nil, fmt.Errorf("stat: %w", err)
	}
	if !st.Mode().IsRegular() {
		return nil, errors.New("not a regular file")
	}
	limit := maxEnvFileBytes()
	if st.Size() > limit {
		return nil, fmt.Errorf("larger than the %d byte limit (%s)", limit, envMaxEnvFileBytes)
	}
	fh, err := os.Open(resolved) //nolint:gosec // resolved is verified to sit inside the repository root above
	if err != nil {
		return nil, fmt.Errorf("open: %w", err)
	}
	defer func() { _ = fh.Close() }()
	body, err := io.ReadAll(io.LimitReader(fh, limit+1))
	if err != nil {
		return nil, fmt.Errorf("read: %w", err)
	}
	if int64(len(body)) > limit {
		return nil, fmt.Errorf("larger than the %d byte limit (%s)", limit, envMaxEnvFileBytes)
	}
	return parseDotenv(string(body))
}

// parseDotenv reads KEY=VALUE lines. Errors name the line number only, so a
// secret value is never echoed.
func parseDotenv(body string) (map[string]string, error) {
	out := map[string]string{}
	for i, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(strings.TrimSuffix(line, "\r"))
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimSpace(strings.TrimPrefix(line, "export "))
		key, val, ok := strings.Cut(line, "=")
		key = strings.TrimSpace(key)
		if !ok || key == "" || strings.ContainsAny(key, " \t") {
			return nil, fmt.Errorf("line %d: expected KEY=VALUE", i+1)
		}
		parsed, err := parseDotenvValue(strings.TrimSpace(val))
		if err != nil {
			return nil, fmt.Errorf("line %d: %w", i+1, err)
		}
		out[key] = parsed
	}
	return out, nil
}

func parseDotenvValue(v string) (string, error) {
	if v == "" {
		return "", nil
	}
	switch v[0] {
	case '\'':
		end := strings.IndexByte(v[1:], '\'')
		if end < 0 {
			return "", errors.New("unterminated single quote")
		}
		return v[1 : 1+end], nil
	case '"':
		var b strings.Builder
		for i := 1; i < len(v); i++ {
			switch c := v[i]; c {
			case '"':
				return b.String(), nil
			case '\\':
				if i+1 >= len(v) {
					return "", errors.New("unterminated double quote")
				}
				i++
				switch v[i] {
				case 'n':
					b.WriteByte('\n')
				case 't':
					b.WriteByte('\t')
				default:
					b.WriteByte(v[i])
				}
			default:
				b.WriteByte(c)
			}
		}
		return "", errors.New("unterminated double quote")
	}
	if idx := strings.Index(v, " #"); idx >= 0 {
		v = v[:idx]
	}
	return strings.TrimSpace(v), nil
}
