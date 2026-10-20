package backup

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"

	"github.com/GLINCKER/levelrail/internal/docker"
)

// volumeHelperImage is the image used to back up or restore a named
// Docker volume's contents: a volume has no dump tool of its own running
// inside it, unlike a database container, so this mounts it into a
// small, disposable container instead. Pinned to a specific tag, not
// "latest": a helper image drifting out from under a backup/restore path
// is exactly the kind of silent behavior change this feature must not
// risk.
const volumeHelperImage = "alpine:3.20"

// volumeMountPath is where the target volume is mounted inside the
// helper container, shared by the archiver and the restorer.
const volumeMountPath = "/vol"

// createVolumeHelper creates and starts a short-lived container mounting
// volumeName at volumeMountPath, the shared setup ContainerVolumeArchiver
// and ContainerVolumeRestorer both need. Its only foreground process is
// a long sleep; real work happens via Exec/ExecWithInput against it
// afterward. The caller owns removing it (docker.Runtime.Remove, force)
// once done, success or failure alike.
func createVolumeHelper(ctx context.Context, rt docker.Runtime, volumeName, namePrefix string, readOnly bool) (string, error) {
	suffix, err := randomHelperSuffix()
	if err != nil {
		return "", fmt.Errorf("generate helper container name: %w", err)
	}

	id, err := rt.Create(ctx, docker.ContainerSpec{
		Name:    namePrefix + "-" + suffix,
		Image:   volumeHelperImage,
		Command: []string{"sleep", "86400"},
		Volumes: []docker.VolumeMount{{Name: volumeName, ContainerPath: volumeMountPath, ReadOnly: readOnly}},
	})
	if err != nil {
		return "", fmt.Errorf("create helper container: %w", err)
	}
	if err := rt.Start(ctx, id); err != nil {
		_ = rt.Remove(context.Background(), id, true)
		return "", fmt.Errorf("start helper container: %w", err)
	}
	return id, nil
}

func randomHelperSuffix() (string, error) {
	buf := make([]byte, 6)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}
