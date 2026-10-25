package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/GLINCKER/levelrail/internal/iac"
	"github.com/GLINCKER/levelrail/internal/platformimport"
)

const emitKindStack = "stack"

// importEmitFlags are the flags shared by "import config" and "import platform" for --emit stack.
type importEmitFlags struct {
	emit, out, stack, project, environment, imageRepo string
	force                                             bool
}

// bindImportEmitFlags registers the emit flags. outHelp differs because
// "import config" already uses --out for a single spec file.
func bindImportEmitFlags(fset *flag.FlagSet, e *importEmitFlags, bindOut bool) {
	fset.StringVar(&e.emit, "emit", "", "write reviewable documents instead of applying: stack")
	if bindOut {
		fset.StringVar(&e.out, "out", "", "with --emit stack: the directory to write the documents into")
	}
	fset.StringVar(&e.stack, "stack", "", "with --emit stack: the stack name (default derived from the source)")
	fset.StringVar(&e.project, "project", "", "with --emit stack: the project the documents belong to (default: the stack name)")
	fset.StringVar(&e.environment, "environment", "", "with --emit stack: also write an Environment document and place the apps in it")
	fset.StringVar(&e.imageRepo, "image-repo", "", "with --emit stack: registry path for apps that have no image, named REPO/APP:latest (apply runs images only)")
	fset.BoolVar(&e.force, "force", false, "with --emit stack: overwrite files that already exist in the output directory")
}

// importEmitResult is the --json shape of an emit.
type importEmitResult struct {
	Dir     string   `json:"dir"`
	Stack   string   `json:"stack"`
	Project string   `json:"project"`
	Files   []string `json:"files"`
	Next    []string `json:"nextSteps"`
}

func (e importEmitFlags) active() bool { return e.emit != "" }

// options validates the flags and resolves defaults. fallbackStack names the stack when --stack is empty.
func (e importEmitFlags) options(fallbackStack string) (platformimport.EmitOptions, error) {
	if e.emit != emitKindStack {
		return platformimport.EmitOptions{}, newValidationError("--emit must be %q, got %q", emitKindStack, e.emit)
	}
	if e.out == "" {
		return platformimport.EmitOptions{}, newValidationError("--emit stack needs --out DIR, the directory to write the documents into")
	}
	stack := e.stack
	if stack == "" {
		stack = fallbackStack
	}
	if err := iac.ValidateStackName(stack); err != nil {
		return platformimport.EmitOptions{}, newValidationError("--stack: %v", err)
	}
	project := e.project
	if project == "" {
		project = stack
	}
	return platformimport.EmitOptions{Stack: stack, Project: project, Environment: e.environment, ImageRepo: e.imageRepo}, nil
}

// writeEmitted writes the files into dir. Without force it refuses to touch an existing file.
func writeEmitted(dir string, files []platformimport.EmitFile, force bool) ([]string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil { //nolint:gosec // a directory of reviewable documents the operator asked for
		return nil, fmt.Errorf("create %s: %w", dir, err)
	}
	if !force {
		for _, f := range files {
			if _, err := os.Stat(filepath.Join(dir, f.Name)); err == nil {
				return nil, newValidationError("%s already exists, pass --force to overwrite", filepath.Join(dir, f.Name))
			} else if !errors.Is(err, fs.ErrNotExist) {
				return nil, fmt.Errorf("stat %s: %w", f.Name, err)
			}
		}
	}
	names := make([]string, 0, len(files))
	for _, f := range files {
		p := filepath.Join(dir, f.Name)
		if err := os.WriteFile(p, f.Content, 0o644); err != nil { //nolint:gosec // documents meant to be read and committed
			return nil, fmt.Errorf("write %s: %w", p, err)
		}
		names = append(names, f.Name)
	}
	return names, nil
}

// finishEmit writes the files and prints the result and the next commands.
func finishEmit(prog string, o platformimport.EmitOptions, dir string, files []platformimport.EmitFile, force, jsonOut bool, stdout, stderr io.Writer) int {
	names, err := writeEmitted(dir, files, force)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, err)
	}
	next := []string{
		fmt.Sprintf("Fill the secrets listed in %s (names only, no values were read).", filepath.Join(dir, platformimport.EmitSecretsFile)),
		fmt.Sprintf("%s stack plan %s -f %s", prog, o.Stack, dir),
		fmt.Sprintf("%s stack apply %s -f %s --yes --secret NAME=env:CI_SECRET", prog, o.Stack, dir),
	}
	if jsonOut {
		if err := writeJSONValue(stdout, importEmitResult{Dir: dir, Stack: o.Stack, Project: o.Project, Files: names, Next: next}); err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return exitNetwork
		}
		return exitOK
	}
	_, _ = fmt.Fprintf(stdout, "Wrote %d files to %s for stack %s (project %s):\n", len(names), dir, o.Stack, o.Project)
	for _, n := range names {
		_, _ = fmt.Fprintf(stdout, "  %s\n", n)
	}
	_, _ = fmt.Fprintf(stdout, "\nNEXT STEPS\n")
	for _, s := range next {
		_, _ = fmt.Fprintf(stdout, "  %s\n", s)
	}
	_, _ = fmt.Fprintf(stdout, "\nBefore you apply:\n")
	_, _ = fmt.Fprintf(stdout, "  Apply runs prebuilt images only: push the images named in apps.yaml (--image-repo sets the registry path), or point each App at its image.\n")
	_, _ = fmt.Fprintf(stdout, "  Database connection variables are secret names in %s until the schema can reference a database (databases: sections and from: refs).\n", platformimport.EmitSecretsFile)
	_, _ = fmt.Fprintf(stdout, "  Volumes, scheduled jobs and data are listed in %s, they are not in the documents.\n", platformimport.EmitNotesFile)
	return exitOK
}

// stackNameFromDir derives a valid stack name from a directory, or "imported".
func stackNameFromDir(dir string) string {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "imported"
	}
	n := platformimport.SanitizeName(strings.TrimPrefix(filepath.Base(abs), "."))
	if iac.ValidateStackName(n) != nil {
		return "imported"
	}
	return n
}
