package application

import (
	"github.com/glincker/levelrail/internal/docker"
	"github.com/glincker/levelrail/internal/store"
)

// appLabelValue is what the reconciler stamps under spec.AppLabelKey; grant
// sockets scope by the same value so the two can never drift.
func appLabelValue(desired *store.DesiredService) string { return volumeOwner(desired, "").App }

// volumeOwner is the owner stamped on a volume the app declares.
func volumeOwner(desired *store.DesiredService, volName string) docker.VolumeOwner {
	return docker.VolumeOwner{App: desired.Name, Name: volName}
}
