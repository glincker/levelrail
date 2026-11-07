package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/spec"
)

// minUploadHashPrefix keeps a typo from matching an unrelated stored upload.
const minUploadHashPrefix = 8

// validUploadHashPrefix reports whether s is 8 to 64 lowercase hex characters.
func validUploadHashPrefix(s string) bool {
	if len(s) < minUploadHashPrefix || len(s) > 64 {
		return false
	}
	for _, c := range s {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}

// shortHash trims a digest for display without panicking on a short prefix.
func shortHash(h string) string {
	if len(h) > 12 {
		return h[:12]
	}
	return h
}

// validateUploadFlags rejects every flag the detected build cannot honor, so a refused
// command has created, uploaded and queued nothing.
func validateUploadFlags(prog string, inf upInference, buildEnv map[string]string, fromHash string) error {
	if len(buildEnv) > 0 && inf.Flags.buildType != spec.BuildDockerfile {
		// The server rejects build args for Railpack, which only reads the app env at build time.
		return newValidationError("--build-env only applies to Dockerfile builds, this directory builds with %s; set the value with --env KEY=VALUE instead (variables such as NEXT_PUBLIC_* are inlined at build time)", inf.Flags.buildType)
	}
	if fromHash != "" && !validUploadHashPrefix(strings.ToLower(fromHash)) {
		return newValidationError("--upload-hash must be a hex digest or a unique prefix of at least %d characters; \"%s deploy\" prints the full digest after an upload", minUploadHashPrefix, prog)
	}
	return nil
}

// resolveUploadHash expands a unique prefix of a stored upload to the full digest.
func resolveUploadHash(ctx context.Context, client *Client, app, in string) (string, error) {
	in = strings.ToLower(in)
	if validUploadHash(in) {
		return in, nil
	}
	list, err := client.ListSourceUploads(ctx, app)
	if err != nil {
		return "", fmt.Errorf("look up stored uploads of app %q: %w", app, err)
	}
	var match []string
	for _, u := range list.Items {
		if strings.HasPrefix(u.Hash, in) {
			match = append(match, u.Hash)
		}
	}
	switch len(match) {
	case 0:
		return "", newValidationError("no stored upload of app %q starts with %q (uploads expire; deploy the directory again)", app, in)
	case 1:
		return match[0], nil
	}
	return "", newValidationError("%q matches %d stored uploads of app %q, use a longer prefix", in, len(match), app)
}

// createdAppNote tells the user about the empty app this run created when it fails before a build starts.
func createdAppNote(prog string, res *deployUploadResult, err error) error {
	if err == nil || res == nil || !res.Created {
		return err
	}
	return fmt.Errorf("%w\napp %q was created by this run and has no build yet; retry the deploy, or remove it with \"%s apps delete %s\"", err, res.App, prog, res.App)
}

// rollbackCreatedApp removes the empty app this run created when no build was queued, so a
// rejected deploy leaves nothing behind. If removal fails it falls back to the manual note.
func rollbackCreatedApp(ctx context.Context, client *Client, prog string, res *deployUploadResult, err error) error {
	if err == nil || res == nil || !res.Created || res.DeployID != "" {
		return err
	}
	if delErr := client.DeleteApp(ctx, res.App); delErr != nil {
		return createdAppNote(prog, res, err)
	}
	res.Created = false
	return fmt.Errorf("%w\nremoved app %q that this run had created, nothing was left behind", err, res.App)
}

// followHint is the command that re-attaches to a queued build.
func followHint(prog, app, deployID string) string {
	return fmt.Sprintf("%s apps deploys logs %s %s --follow", prog, app, deployID)
}

// stampTrace prefixes the server's phase markers with the time since the build started.
func stampTrace(line string, since time.Duration) string {
	if !strings.HasPrefix(line, "[trace] ") {
		return line
	}
	return fmt.Sprintf("[+%s] %s", since.Round(time.Second), line)
}
