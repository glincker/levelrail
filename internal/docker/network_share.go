package docker

import (
	"context"
	"fmt"

	"github.com/docker/docker/api/types/volume"
)

// Network share protocols NetworkShareDriverOpts understands. Mirrors
// store.NetworkShareProtocolNFS/CIFS; this package never imports
// internal/store, so the caller translates.
const (
	NetworkShareProtocolNFS  = "nfs"
	NetworkShareProtocolCIFS = "cifs"
)

// NetworkShareVolumeOpts is the resolved input NetworkShareDriverOpts
// needs: a CIFS share's Password comes from internal/secrets, already
// decrypted by the caller, and is never logged or stored by this
// package.
type NetworkShareVolumeOpts struct {
	Protocol     string
	Host         string
	RemotePath   string
	MountOptions string
	Username     string
	Password     string
}

// NetworkShareDriverOpts translates opts into the driver options Docker's
// own `local` volume driver expects for its built-in NFS/CIFS passthrough
// (the same options `docker volume create --driver local --opt type=nfs
// ...` would pass on the CLI), so EnsureNetworkVolume never needs to
// shell out: the Engine API's volume-create endpoint accepts these
// directly.
func NetworkShareDriverOpts(opts NetworkShareVolumeOpts) (map[string]string, error) {
	switch opts.Protocol {
	case NetworkShareProtocolNFS:
		o := "addr=" + opts.Host
		if opts.MountOptions != "" {
			o += "," + opts.MountOptions
		}
		return map[string]string{
			"type":   "nfs",
			"o":      o,
			"device": ":" + opts.RemotePath,
		}, nil
	case NetworkShareProtocolCIFS:
		o := fmt.Sprintf("username=%s,password=%s,addr=%s", opts.Username, opts.Password, opts.Host)
		if opts.MountOptions != "" {
			o += "," + opts.MountOptions
		}
		return map[string]string{
			"type":   "cifs",
			"o":      o,
			"device": "//" + opts.Host + opts.RemotePath,
		}, nil
	default:
		return nil, fmt.Errorf("docker: unsupported network share protocol %q", opts.Protocol)
	}
}

// EnsureNetworkVolume creates a `local`-driver Docker volume backed by an
// NFS or CIFS network share, the same idempotent-by-name contract
// EnsureVolume documents for a plain local volume: Docker's VolumeCreate
// returns the existing volume, not an error, when name already exists.
func (c *Client) EnsureNetworkVolume(ctx context.Context, name string, opts NetworkShareVolumeOpts) error {
	driverOpts, err := NetworkShareDriverOpts(opts)
	if err != nil {
		return err
	}
	if _, err := c.cli.VolumeCreate(ctx, volume.CreateOptions{
		Name:       name,
		Driver:     "local",
		DriverOpts: driverOpts,
		Labels:     c.withInstanceLabel(nil),
	}); err != nil {
		return fmt.Errorf("docker: ensure network volume %q: %w", name, err)
	}
	return nil
}
