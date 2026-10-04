package api

import (
	"context"
	"log/slog"
	"os/exec"

	"github.com/GLINCKER/levelrail/internal/store"
)

// doctorNASDocsPath is the one guide a missing NFS/CIFS mount helper's
// Fix points at.
const doctorNASDocsPath = "/storage#network-share-client-tools"

// doctorCheckNASClientTools reports whether this node has the mount.nfs/
// mount.cifs helpers Docker's own `local` driver shells out to at mount
// time, turning a cryptic deploy-time Docker error into an actionable
// message up front. Only checked when a configured share actually uses
// that protocol, the same "only probe what's configured" shape
// doctorCheckRegistryReachability already establishes.
func (rt *Router) doctorCheckNASClientTools(ctx context.Context) []doctorCheckResource {
	shares, err := rt.networkShares.ListNetworkShares(ctx)
	if err != nil {
		rt.logger.Error("api: doctor: list network shares failed", slog.String("error", err.Error()))
		return nil
	}

	var needNFS, needCIFS bool
	for _, s := range shares {
		switch s.Protocol {
		case store.NetworkShareProtocolNFS:
			needNFS = true
		case store.NetworkShareProtocolCIFS:
			needCIFS = true
		}
	}

	var out []doctorCheckResource
	if needNFS {
		out = append(out, doctorCheckMountHelper(exec.LookPath, "mount.nfs", "NFS client tools", "nfs-common (Debian/Ubuntu) or nfs-utils (RHEL/Fedora/Arch)"))
	}
	if needCIFS {
		out = append(out, doctorCheckMountHelper(exec.LookPath, "mount.cifs", "CIFS/SMB client tools", "cifs-utils"))
	}
	return out
}

// doctorCheckMountHelper reports whether binary (a mount(8) filesystem
// helper) is on this node's PATH.
func doctorCheckMountHelper(lookPath func(string) (string, error), binary, name, installHint string) doctorCheckResource {
	code := "nas_" + binary
	if _, err := lookPath(binary); err != nil {
		return doctorCheckResource{
			Code: code, Name: name, Status: doctorStatusFail,
			Message:  binary + " not found on this node: mounting a network share over this protocol will fail with a cryptic Docker error instead of this message",
			Fix:      "Install " + installHint + " on this node, then retry the deploy or the share's test-mount.",
			DocsPath: doctorNASDocsPath,
		}
	}
	return doctorCheckResource{Code: code, Name: name, Status: doctorStatusOK, Message: binary + " is installed"}
}
