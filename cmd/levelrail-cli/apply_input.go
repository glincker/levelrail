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
	"github.com/GLINCKER/levelrail/internal/iac"
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
	return readIaCFilesNamed(paths, false)
}

// iacEntry is one file read for apply: its display name, its path relative to
// the -f root and that root, so compose references resolve against the root.
type iacEntry struct {
	file apiclient.IaCFile
	rel  string
	root string
}

// readIaCFilesNamed reads like readIaCFiles. With relative set, each file is
// named by its path under the -f root (the base name for a plain file), so
// stored names do not carry a workstation or CI directory.
func readIaCFilesNamed(paths []string, relative bool) ([]apiclient.IaCFile, error) {
	entries, err := collectIaCEntries(paths)
	if err != nil {
		return nil, err
	}
	out := make([]apiclient.IaCFile, len(entries))
	for i, e := range entries {
		out[i] = e.file
		if relative {
			out[i].Name = e.rel
		}
	}
	return out, nil
}

func collectIaCEntries(paths []string) ([]iacEntry, error) {
	var out []iacEntry
	for _, p := range paths {
		if p == "-" {
			data, err := io.ReadAll(io.LimitReader(stdinSource, 8<<20))
			if err != nil {
				return nil, fmt.Errorf("read stdin: %w", err)
			}
			out = append(out, iacEntry{file: apiclient.IaCFile{Name: "stdin", Content: string(data)}, rel: "stdin"})
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
			out = append(out, iacEntry{file: f, rel: filepath.Base(p), root: filepath.Dir(p)})
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
			rel := f.Name
			if r, err := filepath.Rel(p, path); err == nil {
				rel = filepath.ToSlash(r)
			}
			out = append(out, iacEntry{file: f, rel: rel, root: p})
		}
	}
	if len(out) == 0 {
		return nil, errors.New("no resource files found")
	}
	return out, nil
}

// readIaCInput reads the -f inputs and moves compose files that App documents
// reference out of the document list into attachments. Referenced files are
// read below their root with no symlink on the way.
func readIaCInput(paths []string) ([]apiclient.IaCFile, map[string]string, error) {
	return readIaCInputNamed(paths, false)
}

// readIaCInputNamed is readIaCInput; relative names each document by its path under the -f root.
func readIaCInputNamed(paths []string, relative bool) ([]apiclient.IaCFile, map[string]string, error) {
	entries, err := collectIaCEntries(paths)
	if err != nil {
		return nil, nil, err
	}
	byRoot := map[string][]iacEntry{}
	var roots []string
	for _, e := range entries {
		if _, ok := byRoot[e.root]; !ok {
			roots = append(roots, e.root)
		}
		byRoot[e.root] = append(byRoot[e.root], e)
	}
	var files []apiclient.IaCFile
	att := map[string]string{}
	for _, root := range roots {
		group := byRoot[root]
		srcs := make([]iac.Source, len(group))
		display := map[string]apiclient.IaCFile{}
		for i, e := range group {
			srcs[i] = iac.Source{Name: e.rel, Data: []byte(e.file.Content)}
			display[e.rel] = e.file
		}
		docs, atts, _ := iac.SplitComposeRefs(srcs)
		for _, d := range docs {
			f := display[d.Name]
			if relative {
				f.Name = d.Name
			}
			files = append(files, f)
		}
		for name, data := range atts {
			if err := checkNoSymlink(root, name); err != nil {
				return nil, nil, err
			}
			att[name] = string(data)
		}
		for _, ref := range iac.ComposeRefPaths(docs) {
			if _, have := att[ref]; have || root == "" {
				continue
			}
			data, err := readBelowRoot(root, ref)
			if err != nil {
				if errors.Is(err, fs.ErrNotExist) {
					continue // the server reports compose_ref_missing
				}
				return nil, nil, err
			}
			att[ref] = data
		}
	}
	if len(att) == 0 {
		att = nil
	}
	return files, att, nil
}

// checkNoSymlink refuses a symlink on any segment of rel below root.
func checkNoSymlink(root, rel string) error {
	cur := root
	for _, seg := range strings.Split(rel, "/") {
		cur = filepath.Join(cur, seg)
		fi, err := os.Lstat(cur)
		if err != nil {
			return err
		}
		if fi.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("compose file %s: symlinks are not allowed on a compose path", rel)
		}
	}
	return nil
}

func readBelowRoot(root, rel string) (string, error) {
	if err := checkNoSymlink(root, rel); err != nil {
		return "", err
	}
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel))) //nolint:gosec // the path was cleaned and checked for symlinks
	return string(data), err
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
