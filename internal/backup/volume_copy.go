package backup

import (
	"context"
	"fmt"

	"github.com/GLINCKER/levelrail/internal/docker"
)

// CopyVolume copies every file of src into dst, keeping modes and owners.
// Both volumes must not be mounted by a running database.
func CopyVolume(ctx context.Context, rt docker.Runtime, src, dst string) error {
	suffix, err := randomHelperSuffix()
	if err != nil {
		return err
	}
	id, err := rt.Create(ctx, docker.ContainerSpec{
		Name: "volcopy-" + suffix, Image: volumeHelperImage, Command: []string{"sleep", "86400"},
		Volumes: []docker.VolumeMount{{Name: src, ContainerPath: snapshotSrcMount, ReadOnly: true}, {Name: dst, ContainerPath: snapshotDstMount}},
	})
	if err != nil {
		return fmt.Errorf("create copy helper: %w", err)
	}
	defer func() { _ = rt.Remove(context.WithoutCancel(ctx), id, true) }()
	if err := rt.Start(ctx, id); err != nil {
		return fmt.Errorf("start copy helper: %w", err)
	}
	_, err = execOutput(ctx, rt, id, []string{"sh", "-c", "cp -a " + snapshotSrcMount + "/. " + snapshotDstMount + "/"})
	if err != nil {
		return fmt.Errorf("copy %q to %q: %w", src, dst, err)
	}
	return nil
}

// WipeVolume deletes every file in a volume, dotfiles included.
func WipeVolume(ctx context.Context, rt docker.Runtime, name string) error {
	id, err := createVolumeHelper(ctx, rt, name, "volwipe", false)
	if err != nil {
		return fmt.Errorf("open volume %q: %w", name, err)
	}
	defer func() { _ = rt.Remove(context.WithoutCancel(ctx), id, true) }()
	cmd := []string{"sh", "-c", "cd " + volumeMountPath + " && rm -rf -- ..?* .[!.]* *"}
	if _, err := execOutput(ctx, rt, id, cmd); err != nil {
		return fmt.Errorf("wipe volume %q: %w", name, err)
	}
	return nil
}
