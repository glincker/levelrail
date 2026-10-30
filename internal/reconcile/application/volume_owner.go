package application

import (
	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/store"
)

// volumeOwner is the owner stamped on a volume the app declares.
func volumeOwner(desired *store.DesiredService, volName string) docker.VolumeOwner {
	return docker.VolumeOwner{App: desired.Name, Name: volName}
}
