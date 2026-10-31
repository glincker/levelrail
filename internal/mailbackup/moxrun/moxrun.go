// Package moxrun runs mailbackup steps against a mox container over the
// Docker API: exec for mox commands, tar copy for staging, and a short-lived
// helper container for the data swap while mox is stopped.
package moxrun

import (
	"archive/tar"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
)

// Paths inside the mox container. Staging sits under the data volume so mox
// can hardlink message files instead of copying them.
const (
	DataDir    = "/mox/data"
	ConfigDir  = "/mox/config"
	StagingDir = DataDir + "/tmp/backup"
	// WorkDir holds restore staging and pre-restore copies on the same volume
	// as the live data (so swaps are renames) but outside tmp, which mox owns.
	WorkDir = ".levelrail"
	moxUID  = 1000
)

var idPattern = regexp.MustCompile(`^[0-9]{8}T[0-9]{6}Z$`)

// ValidID reports whether id has the backup id shape, so it is safe in paths and shell.
func ValidID(id string) bool { return idPattern.MatchString(id) }

// Execer is the exec half of docker.Runtime.
type Execer interface {
	Exec(ctx context.Context, containerID string, cmd []string) (io.ReadCloser, error)
}

// ExecRunner implements mailbackup.Runner with `docker exec <id> mox ...`.
type ExecRunner struct {
	RT          Execer
	ContainerID string
}

// Run runs mox with args and returns its stdout; a non-zero exit is an error.
func (e ExecRunner) Run(ctx context.Context, args ...string) (string, error) {
	rc, err := e.RT.Exec(ctx, e.ContainerID, append([]string{"mox"}, args...))
	if err != nil {
		return "", err
	}
	defer func() { _ = rc.Close() }()
	return drain(rc)
}

// maxExecOutput caps how much command output is kept; the rest is discarded.
const maxExecOutput = 1 << 20

// drain keeps the first maxExecOutput bytes and reads on to EOF, where the
// Docker runtime reports a non-zero exit as the read error.
func drain(r io.Reader) (string, error) {
	b, err := io.ReadAll(io.LimitReader(r, maxExecOutput))
	if err != nil {
		return string(b), err
	}
	_, err = io.Copy(io.Discard, r)
	return string(b), err
}

// RemoveStaging deletes one staging directory inside the container.
func RemoveStaging(ctx context.Context, rt Execer, containerID, dir string) error {
	if !strings.HasPrefix(dir, StagingDir+"/") && !strings.HasPrefix(dir, DataDir+"/"+WorkDir+"/restore-") && !strings.HasPrefix(dir, ConfigDir+"/"+WorkDir+"/restore-") {
		return fmt.Errorf("moxrun: refusing to remove %q", dir)
	}
	rc, err := rt.Exec(ctx, containerID, []string{"rm", "-rf", "--", dir})
	if err != nil {
		return err
	}
	_, err = io.Copy(io.Discard, rc)
	_ = rc.Close()
	return err
}

// Fetch copies containerDir out of the container into hostDir. maxBytes caps
// the tar stream so a runaway mailbox cannot fill the control plane disk.
func Fetch(ctx context.Context, ar docker.ArchiveRuntime, containerID, containerDir, hostDir string, maxBytes int64) error {
	rc, _, err := ar.CopyFromContainer(ctx, containerID, containerDir, maxBytes)
	if err != nil {
		return err
	}
	defer func() { _ = rc.Close() }()
	if err := os.MkdirAll(hostDir, 0o700); err != nil {
		return err
	}
	tr := tar.NewReader(rc)
	for {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		// Docker roots the tar at the base name of containerDir.
		_, rel, _ := strings.Cut(strings.TrimPrefix(h.Name, "./"), "/")
		if rel == "" {
			continue
		}
		dst, err := safeJoin(hostDir, rel)
		if err != nil {
			return err
		}
		switch h.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(dst, 0o700); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := writeFile(dst, tr); err != nil {
				return err
			}
		case tar.TypeLink:
			_, lrel, _ := strings.Cut(strings.TrimPrefix(h.Linkname, "./"), "/")
			src, err := safeJoin(hostDir, lrel)
			if err != nil {
				return err
			}
			if err := os.Link(src, dst); err != nil {
				return err
			}
		}
	}
}

func writeFile(dst string, r io.Reader) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o700); err != nil {
		return err
	}
	f, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0o600) //nolint:gosec // dst is confined by safeJoin
	if err != nil {
		return err
	}
	_, err = io.Copy(f, r)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	return err
}

func safeJoin(root, name string) (string, error) {
	clean := filepath.Clean(filepath.FromSlash(name))
	if filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("moxrun: unsafe path %q in archive", name)
	}
	return filepath.Join(root, clean), nil
}

// Push copies hostDir into the container under containerDir/<base of hostDir>,
// owned by the mox user.
func Push(ctx context.Context, rt docker.Runtime, ar docker.ArchiveRuntime, containerID, hostDir, containerDir string) error {
	rc, err := rt.Exec(ctx, containerID, []string{"mkdir", "-p", "--", containerDir})
	if err != nil {
		return err
	}
	_, _ = io.Copy(io.Discard, rc)
	if err := rc.Close(); err != nil {
		return err
	}
	pr, pw := io.Pipe()
	go func() { _ = pw.CloseWithError(tarDir(hostDir, pw)) }()
	err = ar.CopyToContainer(ctx, containerID, containerDir, pr, 0)
	_ = pr.CloseWithError(err)
	return err
}

func tarDir(dir string, w io.Writer) error {
	tw := tar.NewWriter(w)
	base := filepath.Base(dir)
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && !d.Type().IsRegular() {
			return nil
		}
		rel, err := filepath.Rel(dir, p)
		if err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		h, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		h.Name = path.Join(base, filepath.ToSlash(rel))
		h.Uid, h.Gid, h.Uname, h.Gname = moxUID, moxUID, "", ""
		if d.IsDir() {
			h.Name += "/"
		}
		if err := tw.WriteHeader(h); err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		f, err := os.Open(p) //nolint:gosec // p comes from walking dir
		if err != nil {
			return err
		}
		_, err = io.Copy(tw, f)
		_ = f.Close()
		return err
	})
	if err != nil {
		return err
	}
	return tw.Close()
}

// WaitRunning polls until the container is running or the timeout passes.
func WaitRunning(ctx context.Context, rt docker.Runtime, name string, timeout time.Duration, ready func(docker.ContainerState) bool) error {
	deadline := time.Now().Add(timeout)
	for {
		st, err := rt.InspectByName(ctx, name)
		if err == nil && st != nil && st.Running && (ready == nil || ready(*st)) {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("mail container %s did not come back within %s", name, timeout)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
}
