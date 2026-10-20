package database

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"

	"github.com/GLINCKER/levelrail/internal/docker"
)

const (
	// certsMountPath is where a TLS-enabled database's cert/key volume
	// is mounted inside its own container, shared by every TLS-capable
	// engine (only Postgres and Redis today, see SupportsTLS).
	certsMountPath = "/certs"
	tlsCertFile    = "tls.crt"
	tlsKeyFile     = "tls.key"

	// dbTLSUID/dbTLSGID match the official postgres and redis Docker
	// images' own runtime service account (both create a uid/gid 999
	// user), so the key file this writes is already owned by the exact
	// user that reads it: Postgres refuses to start with a private key
	// file readable by anyone other than its own effective user, and
	// this sidesteps that check entirely rather than needing a chown
	// step inside the database image itself.
	dbTLSUID = 999
	dbTLSGID = 999
)

// certsHelperImage mirrors internal/backup's volumeHelperImage: a small,
// disposable container with no dump tool of its own running inside a
// volume, pinned to a specific tag rather than "latest" for the same
// reason that file's own doc comment gives.
const certsHelperImage = "alpine:3.20"

// certsHelperMountPath is where the target certs volume is mounted
// inside the helper container itself, distinct from certsMountPath
// (where the database's own container later mounts the same volume):
// this constant only matters to provisionCerts and the tar command it
// runs, not to any consumer of the finished volume.
const certsHelperMountPath = "/vol"

func certsVolumeName(dbName string) string { return "db-" + dbName + "-certs" }

// provisionCerts writes material's certificate and key into dbName's
// certs volume via a short-lived helper container, the same
// create-helper-then-exec shape internal/backup's volume archiver/
// restorer already establish for writing into a named volume with no
// tool of its own. Only called once, right before a database's first
// container create (reconcileEngine's own doc comment on why): the
// volume is stable across replacements, so nothing here ever needs to
// run again for the same database.
func (c *Controller) provisionCerts(ctx context.Context, dbName string, material *TLSMaterial) error {
	volName := certsVolumeName(dbName)
	id, err := createCertsHelper(ctx, c.runtime, volName)
	if err != nil {
		return fmt.Errorf("database/%s: provision tls certs: %w", dbName, err)
	}
	defer func() {
		_ = c.runtime.Remove(context.Background(), id, true)
	}()

	archive, err := tlsTarArchive(material)
	if err != nil {
		return fmt.Errorf("database/%s: provision tls certs: %w", dbName, err)
	}
	rc, err := c.runtime.ExecWithInput(ctx, id, []string{"tar", "-xf", "-", "-C", certsHelperMountPath}, archive)
	if err != nil {
		return fmt.Errorf("database/%s: provision tls certs: write archive: %w", dbName, err)
	}
	defer func() {
		_ = rc.Close()
	}()
	if _, err := io.Copy(io.Discard, rc); err != nil {
		return fmt.Errorf("database/%s: provision tls certs: %w", dbName, err)
	}
	return nil
}

func createCertsHelper(ctx context.Context, rt docker.Runtime, volName string) (string, error) {
	suffix, err := randomCertsHelperSuffix()
	if err != nil {
		return "", fmt.Errorf("generate helper container name: %w", err)
	}
	id, err := rt.Create(ctx, docker.ContainerSpec{
		Name:    "dbcerts-" + suffix,
		Image:   certsHelperImage,
		Command: []string{"sleep", "300"},
		Volumes: []docker.VolumeMount{{Name: volName, ContainerPath: certsHelperMountPath}},
	})
	if err != nil {
		return "", fmt.Errorf("create certs helper container: %w", err)
	}
	if err := rt.Start(ctx, id); err != nil {
		_ = rt.Remove(context.Background(), id, true)
		return "", fmt.Errorf("start certs helper container: %w", err)
	}
	return id, nil
}

func randomCertsHelperSuffix() (string, error) {
	buf := make([]byte, 6)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

// tlsTarArchive tar-encodes material's cert and key so extracting it
// (tar -xf - -C, run as root inside the helper container) lands the key
// file at mode 0600 owned by dbTLSUID/dbTLSGID: standard tar behavior
// preserves a header's numeric Uid/Gid when the extracting process is
// root, which is exactly what Postgres's own private-key permission
// check requires (see this file's doc comment on dbTLSUID).
func tlsTarArchive(material *TLSMaterial) (*bytes.Buffer, error) {
	buf := &bytes.Buffer{}
	tw := tar.NewWriter(buf)
	entries := []struct {
		name string
		mode int64
		data []byte
	}{
		{tlsCertFile, 0o644, material.CertPEM},
		{tlsKeyFile, 0o600, material.KeyPEM},
	}
	for _, e := range entries {
		hdr := &tar.Header{
			Name: e.name,
			Mode: e.mode,
			Size: int64(len(e.data)),
			Uid:  dbTLSUID,
			Gid:  dbTLSGID,
		}
		if err := tw.WriteHeader(hdr); err != nil {
			return nil, fmt.Errorf("write tar header for %q: %w", e.name, err)
		}
		if _, err := tw.Write(e.data); err != nil {
			return nil, fmt.Errorf("write tar data for %q: %w", e.name, err)
		}
	}
	if err := tw.Close(); err != nil {
		return nil, fmt.Errorf("close tls tar archive: %w", err)
	}
	return buf, nil
}
