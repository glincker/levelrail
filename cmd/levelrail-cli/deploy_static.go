package main

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/sourceupload"
	"github.com/GLINCKER/levelrail/internal/spec"
)

const staticSitePort = 80

// staticOpts carries the flags a static drop deploy needs from "deploy".
type staticOpts struct {
	appName, fromHash string
	include           []string
	timeout           time.Duration
	maxBytes          int64
	dryRun, yes       bool
}

func isZipPath(p string) bool { return strings.EqualFold(filepath.Ext(p), ".zip") }

// staticAppName derives a valid app name from a folder or zip file name.
func staticAppName(src string) string {
	return dirAppName(strings.TrimSuffix(src, filepath.Ext(src)))
}

// runDeployStatic publishes a folder or a .zip as a static site with no git and no build.
func runDeployStatic(prog, src string, o staticOpts, newClient func() *Client, stdout, stderr io.Writer, of outputFlags, jsonOut bool, confirm func() bool) int {
	say := func(format string, a ...any) {
		if !jsonOut {
			_, _ = fmt.Fprintf(stderr, format+"\n", a...)
		}
	}
	name := o.appName
	if name == "" {
		name = staticAppName(src)
	}
	if name == "" {
		return reportError(stdout, stderr, jsonOut, newValidationError("could not derive an app name from %q, pass --app", src))
	}
	inf := upInference{Flags: createFlags{name: name, port: staticSitePort, buildType: spec.BuildStatic}}
	res := deployUploadResult{App: name, DryRun: o.dryRun}
	ctx := context.Background()

	if o.fromHash != "" {
		if !validUploadHash(o.fromHash) {
			return reportError(stdout, stderr, jsonOut, newValidationError("--upload-hash must be a 64 character hex digest"))
		}
		res.Hash = o.fromHash
		if o.dryRun {
			say("would republish upload %s as static site %q", o.fromHash[:12], name)
			return writeScheduledTaskResult(stdout, stderr, of, res, func() {})
		}
		client := newClient()
		if err := ensureUploadApp(ctx, client, inf, nil, &res, say); err != nil {
			return reportError(stdout, stderr, jsonOut, err)
		}
		return finishUploadDeploy(ctx, client, prog, inf, o.fromHash, nil, o.timeout, &res, stdout, stderr, of, jsonOut)
	}

	st, err := os.Stat(src)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, newValidationError("cannot read %q: %v", src, err))
	}
	var upload func(*Client) (string, error)
	if st.IsDir() {
		plan, err := sourceupload.Scan(ctx, src, sourceupload.Options{Include: o.include, MaxBytes: o.maxBytes})
		if err != nil {
			return reportError(stdout, stderr, jsonOut, uploadScanError(err, o.maxBytes))
		}
		if len(plan.Files) == 0 {
			return reportError(stdout, stderr, jsonOut, newValidationError("nothing to publish in %s: every file is ignored", src))
		}
		res.Files, res.Bytes = len(plan.Files), plan.Total
		hasIndex := false
		for _, f := range plan.Files {
			res.FileList = append(res.FileList, f.Path)
			hasIndex = hasIndex || f.Path == "index.html"
		}
		say("deploy: %s as static site %q, %d files, %s", plan.Root, name, res.Files, humanBytes(plan.Total))
		if !hasIndex {
			say("  warning: no index.html at the top of %s, the site root will show a 404; pass the folder that holds it", src)
		}
		upload = func(c *Client) (string, error) {
			return packAndUpload(ctx, c, name, plan, o.maxBytes, &res, stderr, jsonOut)
		}
	} else {
		if !isZipPath(src) {
			return reportError(stdout, stderr, jsonOut, newValidationError("%q is not a folder or a .zip file", src))
		}
		if st.Size() > o.maxBytes {
			return reportError(stdout, stderr, jsonOut, newValidationError("%s is %s, over the %s upload limit; trim the site or raise %s", src, humanBytes(st.Size()), humanBytes(o.maxBytes), envUploadMaxBytes))
		}
		if err := listZip(src, &res); err != nil {
			return reportError(stdout, stderr, jsonOut, err)
		}
		say("deploy: %s as static site %q, %d files, %s", src, name, res.Files, humanBytes(res.Bytes))
		upload = func(c *Client) (string, error) {
			return uploadZipFile(ctx, c, name, src, st.Size(), &res, stderr, jsonOut)
		}
	}
	if o.dryRun {
		return writeScheduledTaskResult(stdout, stderr, of, res, func() {
			for _, p := range res.FileList {
				_, _ = fmt.Fprintln(stdout, p)
			}
		})
	}
	if !jsonOut && !o.yes && confirm != nil && !confirm() {
		_, _ = fmt.Fprintln(stderr, "cancelled, nothing uploaded")
		return exitUsage
	}
	client := newClient()
	if err := ensureUploadApp(ctx, client, inf, nil, &res, say); err != nil {
		return reportError(stdout, stderr, jsonOut, err)
	}
	hash, err := upload(client)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, err)
	}
	return finishUploadDeploy(ctx, client, prog, inf, hash, nil, o.timeout, &res, stdout, stderr, of, jsonOut)
}

// listZip counts a zip's files and bytes so the plan can be shown before anything is sent.
func listZip(path string, res *deployUploadResult) error {
	zr, err := zip.OpenReader(path)
	if err != nil {
		return newValidationError("%q is not a readable zip file: %v", path, err)
	}
	defer func() { _ = zr.Close() }()
	for _, f := range zr.File {
		if f.FileInfo().IsDir() {
			continue
		}
		res.Files++
		res.Bytes += int64(f.UncompressedSize64) //nolint:gosec // display only, the server enforces the real caps
		res.FileList = append(res.FileList, f.Name)
	}
	if res.Files == 0 {
		return newValidationError("%q has no files to publish", path)
	}
	return nil
}

func uploadZipFile(ctx context.Context, client *Client, app, path string, size int64, res *deployUploadResult, stderr io.Writer, quiet bool) (string, error) {
	f, err := os.Open(path) //nolint:gosec // the operator named this file on the command line
	if err != nil {
		return "", fmt.Errorf("open %q: %w", path, err)
	}
	defer func() { _ = f.Close() }()
	res.Archive = size
	var lastStep int64
	progress := func(sent int64) {
		if quiet || size == 0 {
			return
		}
		if step := sent * 4 / size; step > lastStep {
			lastStep = step
			_, _ = fmt.Fprintf(stderr, "  uploading %d%% (%s of %s)\n", step*25, humanBytes(sent), humanBytes(size))
		}
	}
	up, err := client.UploadSiteZip(ctx, app, f, size, progress)
	if err != nil {
		return "", fmt.Errorf("upload to app %q: %w", app, err)
	}
	res.Hash, res.Deduped = up.Hash, up.Deduped
	if !quiet && up.HasIndex != nil && !*up.HasIndex {
		_, _ = fmt.Fprintln(stderr, "  warning: the zip has no index.html at its top level, the site root will show a 404")
	}
	return up.Hash, nil
}
