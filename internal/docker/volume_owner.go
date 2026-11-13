package docker

import (
	"archive/tar"
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/mount"
)

// A new named volume mounted at a path the image does not contain is created
// root-owned, so an image that runs as a non-root USER cannot write to it.
// chownFreshVolumes fixes that while the volume is empty, or later for a
// Levelrail-created volume whose root-owned root the user cannot write. A spec user overrides the image USER, so the volume is chowned to the user
// the process really runs as.
func (c *Client) chownFreshVolumes(ctx context.Context, containerID, image, user string, volumes []VolumeMount) error {
	if len(volumes) == 0 {
		return nil
	}
	if user == "" {
		inspect, err := c.cli.ImageInspect(ctx, image)
		if err != nil {
			return fmt.Errorf("inspect image %q: %w", image, err)
		}
		if inspect.Config == nil {
			return nil
		}
		user = inspect.Config.User
	}
	if isRootUser(user) {
		return nil
	}
	uid, gid, err := c.resolveImageUser(ctx, containerID, user)
	if err != nil {
		return err
	}
	for _, v := range volumes {
		if v.ReadOnly {
			continue
		}
		root, empty, err := c.volumeRoot(ctx, containerID, v.ContainerPath)
		if err != nil {
			return err
		}
		wanted := rootChownWanted(root, empty, false, uid, gid)
		if !wanted && rootChownWanted(root, empty, true, uid, gid) {
			wanted = c.volumeManaged(ctx, v.Name)
		}
		if !wanted {
			continue
		}
		if err := c.chownVolume(ctx, v.Name, uid, gid); err != nil {
			return err
		}
	}
	return nil
}

// rootChownWanted decides whether a volume root goes to the runtime user. A fresh
// root-owned root always does; one holding files (an earlier deploy as root, or
// image content copied in) only when Levelrail created the volume and the user cannot write it.
func rootChownWanted(root *tar.Header, empty, managed bool, uid, gid int) bool {
	if root == nil || root.Typeflag != tar.TypeDir || root.Uid != 0 || root.Gid != 0 {
		return false
	}
	if empty {
		return true
	}
	return managed && !dirWritableBy(uid, gid, root.Uid, root.Gid, root.Mode)
}

// dirWritableBy reports whether uid:gid may create entries in a directory with
// that owner and mode, from the owner, group and other bits (write and search).
func dirWritableBy(uid, gid, ownerUID, ownerGID int, mode int64) bool {
	const write, search = 0o2, 0o1
	bits := mode & 0o7
	switch {
	case uid == 0:
		return true
	case uid == ownerUID:
		bits = (mode >> 6) & 0o7
	case gid == ownerGID:
		bits = (mode >> 3) & 0o7
	}
	return bits&write != 0 && bits&search != 0
}

// volumeManaged reports whether Levelrail created the volume for an owner; an
// unreadable or unlabelled volume is left alone.
func (c *Client) volumeManaged(ctx context.Context, name string) bool {
	v, err := c.cli.VolumeInspect(ctx, name)
	if err != nil {
		return false
	}
	_, ok := v.Labels[VolumeOwnerAppLabel]
	return ok
}

func isRootUser(user string) bool {
	name, _, _ := strings.Cut(user, ":")
	return name == "" || name == "0" || name == "root"
}

// resolveImageUser turns an image USER ("101", "node", "node:staff") into
// numeric ids, reading /etc/passwd and /etc/group from the created container.
func (c *Client) resolveImageUser(ctx context.Context, containerID, user string) (uid, gid int, err error) {
	userPart, groupPart, hasGroup := strings.Cut(user, ":")

	uid, userErr := strconv.Atoi(userPart)
	gid = -1
	if userErr != nil {
		passwd, readErr := c.readContainerFile(ctx, containerID, "/etc/passwd")
		if readErr != nil {
			return 0, 0, fmt.Errorf("resolve user: %w", readErr)
		}
		var found bool
		uid, gid, found = lookupPasswd(passwd, userPart)
		if !found {
			return 0, 0, errors.New("resolve user: not in image /etc/passwd")
		}
	}
	if hasGroup {
		if g, convErr := strconv.Atoi(groupPart); convErr == nil {
			return uid, g, nil
		}
		groups, readErr := c.readContainerFile(ctx, containerID, "/etc/group")
		if readErr != nil {
			return 0, 0, fmt.Errorf("resolve group: %w", readErr)
		}
		g, found := lookupGroup(groups, groupPart)
		if !found {
			return 0, 0, errors.New("resolve group: not in image /etc/group")
		}
		return uid, g, nil
	}
	if gid < 0 {
		gid = uid
	}
	return uid, gid, nil
}

func lookupPasswd(passwd []byte, name string) (uid, gid int, found bool) {
	sc := bufio.NewScanner(bytes.NewReader(passwd))
	for sc.Scan() {
		f := strings.Split(sc.Text(), ":")
		if len(f) < 4 || f[0] != name {
			continue
		}
		u, uErr := strconv.Atoi(f[2])
		g, gErr := strconv.Atoi(f[3])
		if uErr != nil || gErr != nil {
			return 0, 0, false
		}
		return u, g, true
	}
	return 0, 0, false
}

func lookupGroup(groups []byte, name string) (gid int, found bool) {
	sc := bufio.NewScanner(bytes.NewReader(groups))
	for sc.Scan() {
		f := strings.Split(sc.Text(), ":")
		if len(f) < 3 || f[0] != name {
			continue
		}
		g, err := strconv.Atoi(f[2])
		if err != nil {
			return 0, false
		}
		return g, true
	}
	return 0, false
}

func (c *Client) readContainerFile(ctx context.Context, containerID, path string) ([]byte, error) {
	rc, _, err := c.cli.CopyFromContainer(ctx, containerID, path)
	if err != nil {
		return nil, fmt.Errorf("copy %s from container: %w", path, err)
	}
	defer func() { _ = rc.Close() }()
	tr := tar.NewReader(rc)
	if _, err := tr.Next(); err != nil {
		return nil, fmt.Errorf("read %s archive: %w", path, err)
	}
	data, err := io.ReadAll(io.LimitReader(tr, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	return data, nil
}

// volumeRoot reads the mount point's own tar header and whether it holds
// nothing else; the stream is closed after at most two headers.
func (c *Client) volumeRoot(ctx context.Context, containerID, dir string) (*tar.Header, bool, error) {
	rc, _, err := c.cli.CopyFromContainer(ctx, containerID, dir)
	if err != nil {
		return nil, false, fmt.Errorf("inspect volume dir %s: %w", dir, err)
	}
	defer func() { _ = rc.Close() }()
	tr := tar.NewReader(rc)
	root, err := tr.Next()
	if err != nil {
		return nil, false, fmt.Errorf("read volume dir %s: %w", dir, err)
	}
	_, err = tr.Next()
	return root, errors.Is(err, io.EOF), nil
}

// chownHelperImage is the throwaway image that runs chown against the volume:
// the app's own image may be distroless or run as a user that cannot chown.
const chownHelperImage = "alpine:3.20"

func (c *Client) chownVolume(ctx context.Context, volumeName string, uid, gid int) error {
	if err := c.ensureImage(ctx, chownHelperImage, nil, false, nil); err != nil {
		return err
	}
	resp, err := c.cli.ContainerCreate(ctx,
		&container.Config{Image: chownHelperImage, Cmd: []string{"chown", fmt.Sprintf("%d:%d", uid, gid), "/v"}, Labels: c.withInstanceLabel(nil)},
		&container.HostConfig{Mounts: []mount.Mount{{Type: mount.TypeVolume, Source: volumeName, Target: "/v"}}},
		nil, nil, "")
	if err != nil {
		return fmt.Errorf("create chown helper for volume %q: %w", volumeName, err)
	}
	defer func() {
		_ = c.cli.ContainerRemove(context.Background(), resp.ID, container.RemoveOptions{Force: true})
	}()
	if err := c.cli.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		return fmt.Errorf("start chown helper for volume %q: %w", volumeName, err)
	}
	waitCh, errCh := c.cli.ContainerWait(ctx, resp.ID, container.WaitConditionNotRunning)
	select {
	case res := <-waitCh:
		if res.StatusCode != 0 {
			return fmt.Errorf("chown helper for volume %q exited %d", volumeName, res.StatusCode)
		}
	case err := <-errCh:
		return fmt.Errorf("wait for chown helper on volume %q: %w", volumeName, err)
	}
	return nil
}
