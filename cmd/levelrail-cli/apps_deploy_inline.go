package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

// deployFromInline builds a Dockerfile read from stdin or a local file, plus
// optional context files, with no git repository behind the app.
func deployFromInline(prog string, client *Client, name string, f sourceDeployFlags, stdin io.Reader, stdout, stderr io.Writer, jsonOut bool, of outputFlags) int {
	src, err := readInlineSource(f, stdin)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, err)
	}
	req := buildTriggerRequest{Build: buildTriggerRequestBuild{Type: "dockerfile"}, InlineSource: &src}
	result, err := client.TriggerBuild(context.Background(), name, req)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("deploy app %q from an inline Dockerfile: %w", name, err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, result, func() {
		_, _ = fmt.Fprintf(stdout, "deploying app %q from an inline Dockerfile with %d context files, deploy attempt %q (follow with \"%s apps deploys logs %s %s\")\n", name, len(src.Files), result.ID, prog, name, result.ID)
	})
}

func readInlineSource(f sourceDeployFlags, stdin io.Reader) (apiclient.InlineSource, error) {
	var src apiclient.InlineSource
	switch {
	case f.dockerfile != "" && f.dockerfileFile != "":
		return src, newValidationError("use one of --dockerfile - or --dockerfile-file, not both")
	case f.dockerfile != "" && f.dockerfile != "-":
		return src, newValidationError("--dockerfile only accepts \"-\" (stdin); pass a path with --dockerfile-file")
	case f.dockerfile == "-":
		b, err := io.ReadAll(stdin)
		if err != nil {
			return src, fmt.Errorf("read Dockerfile from stdin: %w", err)
		}
		src.Dockerfile = string(b)
	default:
		b, err := readRegularFile(f.dockerfileFile)
		if err != nil {
			return src, err
		}
		src.Dockerfile = string(b)
	}
	if strings.TrimSpace(src.Dockerfile) == "" {
		return src, newValidationError("the Dockerfile is empty")
	}
	for _, p := range f.contextFiles {
		rel := filepath.ToSlash(filepath.Clean(p))
		if !filepath.IsLocal(p) || rel == "Dockerfile" {
			return src, newValidationError("--context-file %q must be a relative path inside the current directory, other than Dockerfile", p)
		}
		b, err := readRegularFile(p)
		if err != nil {
			return src, err
		}
		src.Files = append(src.Files, apiclient.InlineFile{Path: rel, Content: string(b)})
	}
	return src, nil
}

// readRegularFile refuses symlinks and special files. The opened handle must be the
// same file the Lstat saw, so a symlink swapped in between is refused too.
func readRegularFile(p string) ([]byte, error) {
	lfi, err := os.Lstat(p)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", p, err)
	}
	if !lfi.Mode().IsRegular() {
		return nil, newValidationError("%s is not a regular file (symlinks are refused)", p)
	}
	f, err := os.Open(p) //nolint:gosec // the caller's own local file, named on the command line
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", p, err)
	}
	defer func() { _ = f.Close() }()
	fi, err := f.Stat()
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", p, err)
	}
	if !fi.Mode().IsRegular() || !os.SameFile(lfi, fi) {
		return nil, newValidationError("%s changed while it was read (symlinks are refused)", p)
	}
	b, err := io.ReadAll(f)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", p, err)
	}
	return b, nil
}
