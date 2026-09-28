package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/GLINCKER/levelrail/internal/compose"
	"github.com/GLINCKER/levelrail/internal/spec"
)

// runAppsValidate implements "apps validate --file <path>": a purely
// local, no-API-call parse of an app.yaml or a raw Docker Compose file.
// It tries spec.Parse first; on failure it falls back to compose.Parse
// plus the same compose.Validate "apps deploy-compose" itself enforces
// (no git checkout here, so build: entries are rejected the same way
// that direct-import endpoint rejects them).
func runAppsValidate(prog string, args []string, stdout, stderr io.Writer, _ func(string) (string, bool)) int {
	fs := flag.NewFlagSet(prog+" apps validate", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var file string
	var jsonOut bool
	fs.StringVar(&file, "file", "", "path to an app.yaml or a Docker Compose YAML file (required)")
	fs.BoolVar(&jsonOut, "json", false, "print the validation result as JSON to stdout and nothing else")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, appsValidateUsage(prog)) }
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}

	if file == "" {
		return reportError(stdout, stderr, jsonOut, newValidationError("--file is required"))
	}
	data, readErr := os.ReadFile(file) //nolint:gosec // operator-supplied CLI flag, the same pattern apps_create.go's own --file read uses
	if readErr != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("read %s: %w", file, readErr))
	}

	result, err := validateAppFile(data)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, newValidationError("%s: %v", file, err))
	}

	if jsonOut {
		if err := writeJSONValue(stdout, result); err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return exitCodeForError(err)
		}
		return exitOK
	}
	printValidateResultHuman(stdout, file, result)
	return exitOK
}

// validateFileResult is "apps validate"'s --json shape.
type validateFileResult struct {
	Format   string   `json:"format"` // "app.yaml" or "compose"
	Services int      `json:"services"`
	Notices  []string `json:"notices,omitempty"`
}

// validateAppFile tries spec.Parse first, then falls back to compose
// parsing; the first one that succeeds decides the file's format. A file
// that is neither returns spec.Parse's own error, the more likely
// intended format for an unrecognized file (app.yaml is this platform's
// primary spec).
func validateAppFile(data []byte) (validateFileResult, error) {
	specResult, specErr := spec.Parse(data)
	if specErr == nil {
		return validateFileResult{Format: "app.yaml", Services: len(specResult.Services)}, nil
	}

	f, composeErr := compose.Parse(data)
	if composeErr != nil {
		return validateFileResult{}, specErr
	}
	if err := f.Validate(); err != nil {
		return validateFileResult{}, err
	}
	var notices []string
	for _, n := range f.Notices() {
		notices = append(notices, fmt.Sprintf("[%s] %s", n.Level, n.Message))
	}
	return validateFileResult{Format: "compose", Services: len(f.Services), Notices: notices}, nil
}

func printValidateResultHuman(out io.Writer, file string, r validateFileResult) {
	_, _ = fmt.Fprintf(out, "%s: valid %s, %d service(s)\n", file, r.Format, r.Services)
	for _, n := range r.Notices {
		_, _ = fmt.Fprintf(out, "  %s\n", n)
	}
}

func appsValidateUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps validate --file <path> [flags]

Locally parses and validates an app.yaml or a Docker Compose YAML file.
Makes no API call: this only checks the file itself, not whether the
control plane could actually deploy it (a compose file's build: contexts,
for instance, are only resolved at real deploy time).

Flags:
  --file string   path to an app.yaml or a Docker Compose YAML file (required)
  --json          print the validation result as JSON to stdout, nothing else
`, prog)
}
