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
	StagingDir = DataDir + "/tmp/backup"
	RestoreDir = DataDir + "/tmp/restore"
	moxUID     = 1000
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
	b, err := io.ReadAll(io.LimitReader(rc, 1<<20))
	return string(b), err
}

// RemoveStaging deletes one staging directory inside the container.
func RemoveStaging(ctx context.Context, rt Execer, containerID, dir string) error {
	if !strings.HasPrefix(dir, StagingDir+"/") && !strings.HasPrefix(dir, RestoreDir+"/") {
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

// swapScript moves the live data aside into tmp/pre-restore-<id> and moves
// the staged restore into place. The old data stays for a manual rollback.
const swapScript = `set -e
cd /mox/data
s="tmp/restore/$1/data"; p="tmp/pre-restore-$1"
test -d "$s"
mkdir -p "$p"
for f in * .[!.]*; do [ -e "$f" ] || continue; [ "$f" = tmp ] && continue; mv "$f" "$p/"; done
for f in "$s"/* "$s"/.[!.]*; do [ -e "$f" ] || continue; mv "$f" .; chown -R 1000:1000 "./$(basename "$f")"; done
rm -rf "tmp/restore/$1"
`

// Swap runs swapScript in a helper container that mounts only the data volume.
// The mail container must be stopped first; image is the mail image itself,
// so the node needs no extra pull.
func Swap(ctx context.Context, rt docker.Runtime, image, dataVolume, id string) error {
	if !ValidID(id) {
		return fmt.Errorf("moxrun: invalid backup id %q", id)
	}
	hid, err := rt.Create(ctx, docker.ContainerSpec{
		Name:       "levelrail-mail-restore-" + strings.ToLower(id),
		Image:      image,
		Entrypoint: []string{"sh", "-c"},
		Command:    []string{"sleep 900"},
		Volumes:    []docker.VolumeMount{{Name: dataVolume, ContainerPath: DataDir}},
	})
	if err != nil {
		return fmt.Errorf("create restore helper: %w", err)
	}
	defer func() { _ = rt.Remove(context.Background(), hid, true) }()
	if err := rt.Start(ctx, hid); err != nil {
		return fmt.Errorf("start restore helper: %w", err)
	}
	rc, err := rt.Exec(ctx, hid, []string{"sh", "-c", swapScript, "swap", id})
	if err != nil {
		return err
	}
	_, err = io.Copy(io.Discard, rc)
	if cerr := rc.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return fmt.Errorf("swap restored data into place: %w", err)
	}
	return nil
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
