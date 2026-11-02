// Command record-verified folds one passed smoke run into verified.json and
// images.lock.json. CI points it at copies and uploads them as an artifact;
// maintainers fold the artifact in, CI never commits.
//
//	go run ./scripts/record-verified -id ntfy -arch amd64 -crane -run-url https://...
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/GLINCKER/levelrail/internal/catalog"
	"github.com/GLINCKER/levelrail/scripts/smokerecord"
)

func main() {
	os.Exit(run(context.Background(), os.Args[1:], os.Stderr))
}

func run(ctx context.Context, args []string, stderr io.Writer) int {
	fs := flag.NewFlagSet("record-verified", flag.ContinueOnError)
	fs.SetOutput(stderr)
	id := fs.String("id", "", "template id")
	arch := fs.String("arch", "", "architecture the run was made on: amd64 or arm64")
	status := fs.Int("http-status", 0, "health path status the run saw")
	runURL := fs.String("run-url", "", "CI run link, empty for a local run")
	refsFile := fs.String("refs-file", "", "file of 'ref digest' lines from the admission job")
	useCrane := fs.Bool("crane", false, "resolve digests with crane when no refs file is given")
	smokeJSON := fs.String("smoke-json", "", "template-smoke JSON report; the template must have passed")
	verified := fs.String("verified", "internal/catalog/verified.json", "verified.json to update")
	lock := fs.String("lock", "internal/catalog/images.lock.json", "images.lock.json to update")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if err := record(ctx, *id, *arch, *status, *runURL, *refsFile, *smokeJSON, *verified, *lock, *useCrane); err != nil {
		_, _ = fmt.Fprintln(stderr, "record-verified:", err)
		return 1
	}
	return 0
}

func record(ctx context.Context, id, arch string, status int, runURL, refsFile, smokeJSON, verified, lock string, useCrane bool) error {
	t, ok := catalog.TemplateByID(id)
	if !ok {
		return fmt.Errorf("unknown template %q", id)
	}
	if smokeJSON != "" {
		s, err := passedStatus(smokeJSON, id)
		if err != nil {
			return err
		}
		if status == 0 {
			status = s
		}
	}
	refs, ok := catalog.PinnedImages(t)
	if !ok {
		return fmt.Errorf("template %s has unpinned images", id)
	}
	var digests map[string]string
	switch {
	case refsFile != "":
		raw, err := os.ReadFile(refsFile) //nolint:gosec // path is a dev-tool flag
		if err != nil {
			return err
		}
		digests = smokerecord.ParseRefs(raw)
	case useCrane:
		var err error
		if digests, err = smokerecord.CraneDigests(ctx, refs); err != nil {
			return err
		}
	}
	return smokerecord.Apply(verified, lock, t, smokerecord.Run{Arch: arch, HTTPStatus: status, RunURL: runURL, Digests: digests})
}

// passedStatus returns the recorded http_status of id, and fails unless the run passed.
func passedStatus(path, id string) (int, error) {
	raw, err := os.ReadFile(path) //nolint:gosec // path is a dev-tool flag
	if err != nil {
		return 0, err
	}
	var rs []struct {
		ID         string `json:"id"`
		Status     string `json:"status"`
		HTTPStatus int    `json:"http_status"`
	}
	if err := json.Unmarshal(raw, &rs); err != nil {
		return 0, fmt.Errorf("read %s: %w", path, err)
	}
	for _, r := range rs {
		if r.ID == id {
			if r.Status != "pass" {
				return 0, fmt.Errorf("%s did not pass (%s), nothing to record", id, r.Status)
			}
			return r.HTTPStatus, nil
		}
	}
	return 0, errors.New("template not in smoke report")
}
