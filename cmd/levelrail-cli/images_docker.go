package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

func runImagesLint(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	var fix bool
	c, code, ok := parseImagesCall(prog, "images lint", args, stderr, lookupEnv, func(fs *flag.FlagSet) {
		fs.BoolVar(&fix, "fix", false, "write the safe fixes back to the Dockerfile and create a missing .dockerignore")
	})
	if !ok {
		return code
	}
	if len(c.rest) != 1 {
		return usageError(stderr, prog, "images lint takes one Dockerfile path")
	}
	path := c.rest[0]
	src, err := os.ReadFile(path) //nolint:gosec // the operator names their own Dockerfile
	if err != nil {
		return reportError(stdout, stderr, c.jsonO, fmt.Errorf("read %s: %w", path, err))
	}
	ignorePath := filepath.Join(filepath.Dir(path), ".dockerignore")
	_, statErr := os.Stat(ignorePath)
	res, err := c.client.LintDockerfile(bg(), string(src), statErr == nil)
	if err != nil {
		return reportError(stdout, stderr, c.jsonO, fmt.Errorf("lint dockerfile: %w", err))
	}
	if fix {
		if res.FixedDockerfile != string(src) {
			if err := os.WriteFile(path, []byte(res.FixedDockerfile), 0o644); err != nil { //nolint:gosec // a Dockerfile is not secret
				return reportError(stdout, stderr, c.jsonO, fmt.Errorf("write %s: %w", path, err))
			}
		}
		if res.DockerignoreHint != "" {
			if err := os.WriteFile(ignorePath, []byte(res.DockerignoreHint), 0o644); err != nil { //nolint:gosec // .dockerignore is not secret
				return reportError(stdout, stderr, c.jsonO, fmt.Errorf("write %s: %w", ignorePath, err))
			}
		}
	}
	rc := writeScheduledTaskResult(stdout, stderr, c.of, res, func() {
		for _, f := range res.Findings {
			loc := "-"
			if f.Line > 0 {
				loc = fmt.Sprintf("line %d", f.Line)
			}
			_, _ = fmt.Fprintf(stdout, "%s  %s  %s\n  %s\n", f.Severity, f.Rule, loc, f.Message)
		}
		if len(res.Findings) == 0 {
			_, _ = fmt.Fprintln(stdout, "no findings")
		}
	})
	if rc != exitOK {
		return rc
	}
	if !fix && len(res.Findings) > 0 {
		return exitCheckFailed
	}
	return exitOK
}

func runImagesHealth(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	c, code, ok := parseImagesCall(prog, "images health", args, stderr, lookupEnv, nil)
	if !ok {
		return code
	}
	res, err := c.client.GetDockerHealth(bg())
	if err != nil {
		return reportError(stdout, stderr, c.jsonO, fmt.Errorf("docker health: %w", err))
	}
	rc := writeScheduledTaskResult(stdout, stderr, c.of, res, func() {
		_, _ = fmt.Fprintf(stdout, "docker %s on %s/%s, %s reclaimable of %s\n", res.Version, res.OS, res.Arch, humanBytes(res.DiskReclaim), humanBytes(res.DiskTotal))
		for _, ch := range res.Checks {
			_, _ = fmt.Fprintf(stdout, "%-5s %-8s %s\n", ch.Status, ch.Code, ch.Message)
		}
	})
	if rc != exitOK {
		return rc
	}
	if !res.Reachable {
		return exitCheckFailed
	}
	return exitOK
}

func runImagesStatus(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	c, code, ok := parseImagesCall(prog, "images status", args, stderr, lookupEnv, nil)
	if !ok {
		return code
	}
	if len(c.rest) != 1 {
		return usageError(stderr, prog, "images status takes one app name")
	}
	res, err := c.client.GetImageStatus(bg(), c.rest[0])
	if err != nil {
		return reportError(stdout, stderr, c.jsonO, fmt.Errorf("image status for app %q: %w", c.rest[0], err))
	}
	return writeScheduledTaskResult(stdout, stderr, c.of, res, func() {
		_, _ = fmt.Fprintf(stdout, "image:   %s\n", res.Image)
		if res.Message != "" {
			_, _ = fmt.Fprintf(stdout, "note:    %s\n", res.Message)
		}
		if res.Source == "registry" {
			_, _ = fmt.Fprintf(stdout, "update:  %t\n", res.UpdateAvailable)
		}
		if res.ImageArch != "" {
			_, _ = fmt.Fprintf(stdout, "arch:    image %s, node %s, mismatch %t\n", res.ImageArch, res.NodeArch, res.ArchMismatch)
		}
		if res.UpdateAvailable {
			_, _ = fmt.Fprintf(stdout, "apply it with: %s apps auto-update check %s\n", prog, c.rest[0])
		}
	})
}
