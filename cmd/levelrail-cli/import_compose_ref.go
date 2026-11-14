package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/GLINCKER/levelrail/internal/compose"
	"github.com/GLINCKER/levelrail/internal/iac"
	"github.com/GLINCKER/levelrail/internal/platformimport"
)

const (
	envComposeRefMaxDepth = "LEVELRAIL_IMPORT_COMPOSE_MAX_DEPTH"
	envComposeRefMaxBytes = "LEVELRAIL_IMPORT_COMPOSE_MAX_BYTES"
	defaultComposeDepth   = 4
	defaultComposeBytes   = 1 << 20
)

var composeFileRe = regexp.MustCompile(`(?i)^(docker-compose|compose)([._-][A-Za-z0-9._-]+)?\.ya?ml$`)

// composeRefSkipDirs are never scanned: they hold copies of other projects.
var composeRefSkipDirs = map[string]bool{".git": true, "node_modules": true, "vendor": true, ".terraform": true}

type composeRefFlags struct {
	stack, project, environment, write string
	profiles                           string
	force, jsonOut, reference          bool
}

// composeScan is the read only result of walking a directory.
type composeScan struct {
	Files []string
	Notes []string
}

type composeRefOutput struct {
	Stack string            `json:"stack"`
	Files []composeRefFile  `json:"files"`
	Notes []string          `json:"notes"`
	Wrote *importEmitResult `json:"wrote,omitempty"`
}

type composeRefFile struct {
	Name    string `json:"name"`
	Content string `json:"content"`
}

func composeRefUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s import compose-ref [dir] [--reference [--profiles a,b]] [--stack NAME] [--project NAME] [--environment NAME] [--write DIR [--force]] [--json]

Scans dir (default .) for docker-compose*.yml, compose*.yaml and friends and prints
reviewable Levelrail documents for every image based service. Read only: nothing is
written unless --write DIR is given, and existing files are kept without --force.
With --reference, one App per compose file points at it with spec.compose.path (the file is never
copied; --write DIR must be the folder that holds the compose files, no parent-folder paths). Files strict
mode would refuse (bind mounts, build, env_file, include) are reported with their codes and skipped.
Environment values are never read into the output: each variable becomes a secret
name listed in SECRETS.md. Services with a build context are skipped with a note.
Limits: %[2]s (default %[3]d) and %[4]s (default %[5]d bytes per file).
`, prog, envComposeRefMaxDepth, defaultComposeDepth, envComposeRefMaxBytes, defaultComposeBytes)
}

func runImportComposeRef(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	var f composeRefFlags
	fset := flag.NewFlagSet(prog+" import compose-ref", flag.ContinueOnError)
	fset.SetOutput(stderr)
	fset.Usage = func() { _, _ = fmt.Fprint(stderr, composeRefUsage(prog)) }
	fset.StringVar(&f.stack, "stack", "", "stack name (default: the directory name)")
	fset.StringVar(&f.project, "project", "", "project name (default: the stack name)")
	fset.StringVar(&f.environment, "environment", "", "environment name for the Stack document")
	fset.StringVar(&f.write, "write", "", "write the documents into this directory instead of printing them")
	fset.BoolVar(&f.force, "force", false, "overwrite existing files in --write")
	fset.BoolVar(&f.reference, "reference", false, "emit one App per compose file that references it in place (spec.compose) instead of converting services")
	fset.StringVar(&f.profiles, "profiles", "", "with --reference, compose profiles to enable (comma separated)")
	fset.BoolVar(&f.jsonOut, "json", false, "print the result as JSON")
	dir := "."
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		dir, args = args[0], args[1:]
	}
	if err := fset.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return exitOK
		}
		return exitUsage
	}
	if fset.NArg() > 0 {
		_, _ = fmt.Fprint(stderr, composeRefUsage(prog))
		return exitUsage
	}
	depth := envInt(lookupEnv, envComposeRefMaxDepth, defaultComposeDepth)
	maxBytes := int64(envInt(lookupEnv, envComposeRefMaxBytes, defaultComposeBytes))

	scan, err := scanComposeDir(dir, depth, maxBytes)
	if err != nil {
		return reportError(stdout, stderr, f.jsonOut, err)
	}
	if len(scan.Files) == 0 {
		return reportError(stdout, stderr, f.jsonOut, newValidationError("no compose files found under %s", dir))
	}
	if f.reference {
		return runComposeReference(prog, f, dir, scan, stdout, stderr)
	}
	stack := f.stack
	if stack == "" {
		stack = stackNameFromDir(dir)
	}
	if err := iac.ValidateStackName(stack); err != nil {
		return reportError(stdout, stderr, f.jsonOut, newValidationError("--stack: %v", err))
	}
	project := f.project
	if project == "" {
		project = stack
	}
	opts := platformimport.EmitOptions{Stack: stack, Project: project, Environment: f.environment}
	in, err := composeRefInput(dir, scan)
	if err != nil {
		return reportError(stdout, stderr, f.jsonOut, err)
	}
	if len(in.Apps) == 0 {
		return reportError(stdout, stderr, f.jsonOut, newValidationError("no image based service to emit:\n  %s", strings.Join(in.Notes, "\n  ")))
	}
	files, err := platformimport.EmitStack(in, opts)
	if err != nil {
		return reportError(stdout, stderr, f.jsonOut, err)
	}
	// The real loader gates the output so a bad document never reaches disk.
	if err := validateEmitFiles(files); err != nil {
		return reportError(stdout, stderr, f.jsonOut, fmt.Errorf("emitted documents do not validate: %w", err))
	}
	if f.write != "" {
		return finishEmit(prog, opts, f.write, files, f.force, f.jsonOut, stdout, stderr)
	}
	return printComposeRef(stdout, stderr, stack, files, in.Notes, f.jsonOut)
}

func printComposeRef(stdout, stderr io.Writer, stack string, files []platformimport.EmitFile, notes []string, jsonOut bool) int {
	if jsonOut {
		out := composeRefOutput{Stack: stack, Notes: notes}
		for _, ef := range files {
			out.Files = append(out.Files, composeRefFile{Name: ef.Name, Content: string(ef.Content)})
		}
		if err := writeJSONValue(stdout, out); err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return exitNetwork
		}
		return exitOK
	}
	for _, ef := range files {
		_, _ = fmt.Fprintf(stdout, "# ---- %s ----\n%s\n", ef.Name, ef.Content)
	}
	hasNotes := false
	for _, ef := range files {
		hasNotes = hasNotes || ef.Name == platformimport.EmitNotesFile
	}
	if !hasNotes {
		for _, n := range notes {
			_, _ = fmt.Fprintf(stdout, "# NOTE %s\n", n)
		}
	}
	_, _ = fmt.Fprintln(stdout, "# Dry run: nothing was written. Pass --write DIR to save these files.")
	return exitOK
}

func validateEmitFiles(files []platformimport.EmitFile) error {
	var srcs []iac.Source
	for _, f := range files {
		if strings.HasSuffix(f.Name, ".yaml") && f.Name != platformimport.EmitStackFile {
			srcs = append(srcs, iac.Source{Name: f.Name, Data: f.Content})
		}
	}
	docs, issues := iac.ParseDocuments(srcs)
	if len(issues) == 0 {
		_, issues = iac.Build(docs, iac.Options{})
	}
	if len(issues) > 0 {
		return fmt.Errorf("%v", issues)
	}
	return nil
}

func envInt(lookupEnv func(string) (string, bool), key string, def int) int {
	if v, ok := lookupEnv(key); ok {
		if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil && n > 0 {
			return n
		}
	}
	return def
}

// scanComposeDir lists compose files under root. Symlinks and oversized files
// are skipped with a note, so nothing outside root is ever read.
func scanComposeDir(root string, maxDepth int, maxBytes int64) (composeScan, error) {
	var out composeScan
	info, err := os.Stat(root)
	if err != nil {
		return out, fmt.Errorf("read %s: %w", root, err)
	}
	if !info.IsDir() {
		return out, newValidationError("%s is not a directory", root)
	}
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, _ := filepath.Rel(root, path)
		if d.IsDir() {
			if path != root && (composeRefSkipDirs[d.Name()] || strings.Count(filepath.ToSlash(rel), "/")+1 > maxDepth) {
				return filepath.SkipDir
			}
			return nil
		}
		if !composeFileRe.MatchString(d.Name()) {
			return nil
		}
		if d.Type()&fs.ModeSymlink != 0 {
			out.Notes = append(out.Notes, fmt.Sprintf("%s is a symlink, not followed", filepath.ToSlash(rel)))
			return nil
		}
		if fi, err := d.Info(); err != nil || fi.Size() > maxBytes {
			out.Notes = append(out.Notes, fmt.Sprintf("%s is larger than %d bytes, skipped", filepath.ToSlash(rel), maxBytes))
			return nil
		}
		out.Files = append(out.Files, filepath.ToSlash(rel))
		return nil
	})
	sort.Strings(out.Files)
	return out, err
}

// composeRefInput turns the scanned files into emit input. It reads service
// shape only: environment values and env files are never copied.
func composeRefInput(root string, scan composeScan) (platformimport.EmitInput, error) {
	in := platformimport.EmitInput{Source: "compose files under " + filepath.Base(root), Notes: append([]string(nil), scan.Notes...)}
	used := map[string]bool{}
	for _, rel := range scan.Files {
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel))) //nolint:gosec // the operator names the scanned directory
		if err != nil {
			return in, fmt.Errorf("read %s: %w", rel, err)
		}
		cf, err := compose.Parse(data)
		if err != nil {
			in.Notes = append(in.Notes, fmt.Sprintf("%s does not parse, skipped: %v", rel, err))
			continue
		}
		names := make([]string, 0, len(cf.Services))
		for n := range cf.Services {
			names = append(names, n)
		}
		sort.Strings(names)
		for _, n := range names {
			svc := cf.Services[n]
			if svc.Image == "" {
				in.Notes = append(in.Notes, fmt.Sprintf("%s: service %s has a build context, skipped: build and push an image, then add an App for it", rel, n))
				continue
			}
			app := platformimport.EmitApp{Name: uniqueComposeAppName(n, rel, used), Image: svc.Image}
			if len(svc.Ports) > 0 {
				app.Port = svc.Ports[0].ContainerPort
			}
			keys := make([]string, 0, len(svc.Environment))
			for k := range svc.Environment {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				app.Secrets = append(app.Secrets, platformimport.EmitSecret{Name: k, Hint: "environment of service " + n + " in " + rel + ", value not read"})
			}
			app.Notes = composeServiceNotes(rel, n, svc)
			in.Apps = append(in.Apps, app)
		}
	}
	in.Notes = append(in.Notes, "These Apps are converted from image services. Referencing the compose file itself (spec.compose) is not available yet, so the compose file and these documents are two definitions: keep one of them.")
	return in, nil
}

func composeServiceNotes(rel, name string, svc compose.Service) []string {
	var notes []string
	for _, ref := range svc.EnvFiles {
		if filepath.IsAbs(ref.Path) || strings.Contains(filepath.ToSlash(ref.Path), "..") {
			notes = append(notes, fmt.Sprintf("env_file %s of %s is outside the project and was refused", ref.Path, name))
			continue
		}
		notes = append(notes, fmt.Sprintf("env_file %s of %s in %s is not read: add each variable name as a secret", ref.Path, name, rel))
	}
	if len(svc.Profiles) > 0 {
		notes = append(notes, fmt.Sprintf("%s in %s is gated by compose profiles %s, decide whether it belongs in this stack", name, rel, strings.Join(svc.Profiles, ",")))
	}
	if len(svc.Volumes) > 0 {
		notes = append(notes, fmt.Sprintf("%s in %s mounts volumes, they are not emitted: add storage for them", name, rel))
	}
	if len(svc.Command) > 0 || len(svc.Entrypoint) > 0 {
		notes = append(notes, fmt.Sprintf("%s in %s overrides command or entrypoint, not emitted", name, rel))
	}
	return notes
}

// uniqueComposeAppName makes a valid App name, prefixed by the file's
// folder when two compose files declare the same service.
func uniqueComposeAppName(service, rel string, used map[string]bool) string {
	base := platformimport.SanitizeName(service)
	name := base
	if used[name] {
		prefix := strings.ReplaceAll(strings.TrimSuffix(rel, filepath.Ext(rel)), "/", "-")
		name = platformimport.SanitizeName(prefix + "-" + base)
	}
	for i := 2; used[name]; i++ {
		name = fmt.Sprintf("%s-%d", base, i)
	}
	used[name] = true
	return name
}
