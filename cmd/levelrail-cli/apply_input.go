package main

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

// stringListFlag is a repeatable string flag.
type stringListFlag []string

func (s *stringListFlag) String() string { return strings.Join(*s, ",") }

func (s *stringListFlag) Set(v string) error {
	if v == "" {
		return errors.New("value must not be empty")
	}
	*s = append(*s, v)
	return nil
}

var (
	stdinSource   io.Reader = os.Stdin
	envPlaceRe              = regexp.MustCompile(`\$\{\{\s*env\.([A-Za-z_][A-Za-z0-9_]*)\s*\}\}`)
	yamlFileNames           = regexp.MustCompile(`(?i)\.ya?ml$`)
)

// readIaCFiles expands each -f value: a file, a directory searched
// recursively for YAML files, or "-" for stdin.
func readIaCFiles(paths []string) ([]apiclient.IaCFile, error) {
	var out []apiclient.IaCFile
	for _, p := range paths {
		if p == "-" {
			data, err := io.ReadAll(io.LimitReader(stdinSource, 8<<20))
			if err != nil {
				return nil, fmt.Errorf("read stdin: %w", err)
			}
			out = append(out, apiclient.IaCFile{Name: "stdin", Content: string(data)})
			continue
		}
		info, err := os.Stat(p)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", p, err)
		}
		if !info.IsDir() {
			f, err := readOneIaCFile(p)
			if err != nil {
				return nil, err
			}
			out = append(out, f)
			continue
		}
		var found []string
		err = filepath.WalkDir(p, func(path string, d fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if !d.IsDir() && yamlFileNames.MatchString(d.Name()) {
				found = append(found, path)
			}
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("walk %s: %w", p, err)
		}
		sort.Strings(found)
		for _, path := range found {
			f, err := readOneIaCFile(path)
			if err != nil {
				return nil, err
			}
			out = append(out, f)
		}
	}
	if len(out) == 0 {
		return nil, errors.New("no resource files found")
	}
	return out, nil
}

func readOneIaCFile(path string) (apiclient.IaCFile, error) {
	data, err := os.ReadFile(path) //nolint:gosec // the operator names the file to apply
	if err != nil {
		return apiclient.IaCFile{}, fmt.Errorf("read %s: %w", path, err)
	}
	return apiclient.IaCFile{Name: filepath.ToSlash(path), Content: string(data)}, nil
}

// resolveSecretFlags turns NAME=env:VAR and NAME=file:PATH into values.
// The values only travel to the control plane; they are never printed.
func resolveSecretFlags(specs map[string]string, lookupEnv func(string) (string, bool)) (map[string]string, error) {
	out := make(map[string]string, len(specs))
	for name, src := range specs {
		kind, ref, ok := strings.Cut(src, ":")
		if !ok || ref == "" {
			return nil, fmt.Errorf("--secret %s: the source must be env:VAR or file:PATH", name)
		}
		switch kind {
		case "env":
			v, set := lookupEnv(ref)
			if !set || v == "" {
				return nil, fmt.Errorf("--secret %s: environment variable %s is not set", name, ref)
			}
			out[name] = v
		case "file":
			data, err := os.ReadFile(ref) //nolint:gosec // the operator names the secret file
			if err != nil {
				return nil, fmt.Errorf("--secret %s: read %s: %w", name, ref, err)
			}
			out[name] = strings.TrimRight(string(data), "\r\n")
		default:
			return nil, fmt.Errorf("--secret %s: the source must be env:VAR or file:PATH", name)
		}
	}
	return out, nil
}
