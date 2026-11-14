package cloudiac

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Layout of the private per-run directory. Only Shared is mounted, read-write, into steps.
const (
	RunSharedSubdir = "shared"
	RunWorkSubdir   = "work"
	RunStateSubdir  = "state"
	runModuleSubdir = "module"
)

// RunDir is the private host directory of one engine run. It holds the module, the plaintext
// state copy, the credential files and the plan; all of it lives on the host only for the run.
type RunDir struct {
	Root    string
	OnTmpfs bool
}

// NewRunDir creates a 0700 run dir under base and hands the shared subdirs to the step user.
func NewRunDir(base, user string) (*RunDir, error) {
	root, err := os.MkdirTemp(base, "run-")
	if err != nil {
		return nil, err
	}
	rd := &RunDir{Root: root, OnTmpfs: isTmpfs(root)}
	uid, gid, owned := stepOwner(user)
	for _, sub := range []string{RunWorkSubdir, RunStateSubdir} {
		dir := filepath.Join(root, RunSharedSubdir, sub)
		if err := os.MkdirAll(dir, 0o700); err != nil {
			rd.Cleanup()
			return nil, err
		}
		// Only a root control plane can chown; otherwise the step user must match the process user.
		if owned && os.Geteuid() == 0 {
			if err := os.Lchown(dir, uid, gid); err != nil {
				rd.Cleanup()
				return nil, err
			}
		}
	}
	return rd, nil
}

// Shared is the directory mounted into every step of the run.
func (r *RunDir) Shared() string { return filepath.Join(r.Root, RunSharedSubdir) }

// StateDir is where the plan step leaves the saved plan for show and apply.
func (r *RunDir) StateDir() string { return filepath.Join(r.Shared(), RunStateSubdir) }

// ModuleDir is the read-only module location, outside the shared mount.
func (r *RunDir) ModuleDir() string { return filepath.Join(r.Root, runModuleSubdir) }

// Cleanup overwrites every regular file best effort, then removes the tree.
func (r *RunDir) Cleanup() {
	if r == nil || r.Root == "" {
		return
	}
	_ = filepath.WalkDir(r.Root, func(p string, d fs.DirEntry, err error) error {
		if err == nil && d.Type().IsRegular() {
			scrubFile(p)
		}
		return nil
	})
	_ = os.RemoveAll(r.Root)
}

func scrubFile(p string) {
	f, err := os.OpenFile(p, os.O_WRONLY, 0) //nolint:gosec // path comes from walking our own run dir
	if err != nil {
		return
	}
	defer func() { _ = f.Close() }()
	st, err := f.Stat()
	if err != nil {
		return
	}
	_, _ = io.CopyN(f, zeroReader{}, st.Size())
	_ = f.Sync()
}

type zeroReader struct{}

func (zeroReader) Read(p []byte) (int, error) {
	clear(p)
	return len(p), nil
}

func stepOwner(user string) (int, int, bool) {
	u, g, ok := strings.Cut(user, ":")
	uid, err1 := strconv.Atoi(u)
	gid, err2 := strconv.Atoi(g)
	if !ok || errors.Join(err1, err2) != nil {
		return 0, 0, false
	}
	return uid, gid, true
}
