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
// chownFreshVolumes fixes that once, while the volume is still empty.
func (c *Client) chownFreshVolumes(ctx context.Context, containerID, image string, volumes []VolumeMount) error {
	if len(volumes) == 0 {
		return nil
	}
	inspect, err := c.cli.ImageInspect(ctx, image)
	if err != nil {
		return fmt.Errorf("inspect image %q: %w", image, err)
	}
	if inspect.Config == nil || isRootUser(inspect.Config.User) {
		return nil
	}
	uid, gid, err := c.resolveImageUser(ctx, containerID, inspect.Config.User)
	if err != nil {
		return err
	}
	for _, v := range volumes {
		if v.ReadOnly {
			continue
		}
		fresh, err := c.volumeIsFreshRootOwned(ctx, containerID, v.ContainerPath)
		if err != nil {
			return err
		}
		if !fresh {
			continue
		}
		if err := c.chownVolume(ctx, v.Name, uid, gid); err != nil {
			return err
		}
	}
	return nil
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
			return 0, 0, fmt.Errorf("resolve user %q: %w", userPart, readErr)
		}
		var found bool
		uid, gid, found = lookupPasswd(passwd, userPart)
		if !found {
			return 0, 0, fmt.Errorf("resolve user %q: not in image /etc/passwd", userPart)
		}
	}
	if hasGroup {
		if g, convErr := strconv.Atoi(groupPart); convErr == nil {
			return uid, g, nil
		}
		groups, readErr := c.readContainerFile(ctx, containerID, "/etc/group")
		if readErr != nil {
			return 0, 0, fmt.Errorf("resolve group %q: %w", groupPart, readErr)
		}
		g, found := lookupGroup(groups, groupPart)
		if !found {
			return 0, 0, fmt.Errorf("resolve group %q: not in image /etc/group", groupPart)
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

// volumeIsFreshRootOwned reports whether dir is a root-owned, empty
// directory: the only state in which chowning cannot touch real data.
func (c *Client) volumeIsFreshRootOwned(ctx context.Context, containerID, dir string) (bool, error) {
	rc, _, err := c.cli.CopyFromContainer(ctx, containerID, dir)
	if err != nil {
		return false, fmt.Errorf("inspect volume dir %s: %w", dir, err)
	}
	defer func() { _ = rc.Close() }()
	tr := tar.NewReader(rc)
	root, err := tr.Next()
	if err != nil {
		return false, fmt.Errorf("read volume dir %s: %w", dir, err)
	}
	if root.Typeflag != tar.TypeDir || root.Uid != 0 || root.Gid != 0 {
		return false, nil
	}
	if _, err := tr.Next(); !errors.Is(err, io.EOF) {
		return false, nil
	}
	return true, nil
}

// chownHelperImage is the throwaway image that runs chown against the volume:
// the app's own image may be distroless or run as a user that cannot chown.
const chownHelperImage = "alpine:3.20"

func (c *Client) chownVolume(ctx context.Context, volumeName string, uid, gid int) error {
	if err := c.ensureImage(ctx, chownHelperImage, nil, false); err != nil {
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
