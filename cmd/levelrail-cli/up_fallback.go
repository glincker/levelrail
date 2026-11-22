package main

import (
	"context"
	"fmt"
	"io"

	"github.com/GLINCKER/levelrail/internal/sourceupload"
)

// upSourceKind is where "up" gets the code the server builds.
type upSourceKind int

const (
	upSourceGit upSourceKind = iota
	upSourceUpload
)

// decideUpSource picks upload only when there is nothing to build from in git and the caller did
// not refuse it; every other case stays on the git path, which reports its own guidance.
func decideUpSource(f upFlags, hasRemote, noUpload, noWait bool) upSourceKind {
	switch {
	case f.repo != "" || hasRemote:
		return upSourceGit
	case noUpload:
		return upSourceGit
	case !upUploadCompatible(f, noWait):
		return upSourceGit
	}
	return upSourceUpload
}

// printUpUploadNotice says once, before anything is sent, why files are uploaded and how many.
func printUpUploadNotice(ctx context.Context, w io.Writer, dir string, maxBytes int64) {
	plan, err := sourceupload.Scan(ctx, dir, sourceupload.Options{MaxBytes: maxBytes})
	if err != nil {
		// The upload itself reports a scan failure with the fix.
		_, _ = fmt.Fprintf(w, "no git remote, uploading %s, honoring .gitignore\n", dir)
		return
	}
	_, _ = fmt.Fprintf(w, "no git remote, uploading %d files (%s), honoring .gitignore\n", len(plan.Files), humanMB(plan.Total))
}

func humanMB(n int64) string { return fmt.Sprintf("%.1f MB", float64(n)/(1<<20)) }
