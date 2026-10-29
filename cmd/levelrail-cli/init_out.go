package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// writeInitOut emits just the app.yaml content for init --out, so scripts can
// pipe it without the agent files.
func writeInitOut(files []initFile, out string, force, dryRun bool, stdout, stderr io.Writer) error {
	f := files[0]
	if f.content == "" {
		return newValidationError("no app.yaml to write: %s", f.Reason)
	}
	if out == "-" {
		_, err := io.WriteString(stdout, f.content)
		return err
	}
	if _, err := os.Stat(out); err == nil && !force {
		return newValidationError("%s already exists, pass --force to overwrite", out)
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("stat %s: %w", out, err)
	}
	if dryRun {
		_, _ = fmt.Fprintf(stderr, "Dry run: would write %s\n", out)
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil { //nolint:gosec // project directories meant to be committed
		return fmt.Errorf("create directory for %s: %w", out, err)
	}
	if err := os.WriteFile(out, []byte(f.content), 0o644); err != nil { //nolint:gosec // project files meant to be committed
		return fmt.Errorf("write %s: %w", out, err)
	}
	_, _ = fmt.Fprintf(stderr, "Wrote %s\n", out)
	return nil
}
