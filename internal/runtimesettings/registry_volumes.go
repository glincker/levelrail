package runtimesettings

import (
	"strconv"
	"time"
)

// Registered keys for moving a volume's files in and out as an archive.
const (
	KeyVolumesTransferMaxBytes = "volumes.transfer_max_bytes"
	KeyVolumesTransferTimeout  = "volumes.transfer_timeout"
	KeyVolumesImportOverwrite  = "volumes.import_overwrite"
	volumesTransferMaxFloor    = 1 << 20
	volumesTransferMaxCeiling  = 1 << 42
	minVolumesTransferTimeout  = time.Minute
	maxVolumesTransferTimeout  = 24 * time.Hour
)

func init() {
	registerLimit(Definition{
		Key: KeyVolumesTransferMaxBytes, Category: CategoryDeploys, Default: strconv.Itoa(10 << 30), Env: "APP_VOLUMES_TRANSFER_MAX_BYTES", Apply: ApplyLive,
		Title:       "Volume export and import size limit",
		Description: "Largest volume archive in bytes that an export may produce or an import may unpack; a bigger transfer stops with HTTP 413. Between 1 MiB and 4 TiB. A change applies to the next transfer.",
	}, volumesTransferMaxFloor, volumesTransferMaxCeiling)
	Register(Definition{
		Key: KeyVolumesTransferTimeout, Category: CategoryDeploys, Kind: KindDuration, Default: "30m",
		Env: "APP_VOLUMES_TRANSFER_TIMEOUT", Apply: ApplyLive, CheckValue: durationRangeCheck(minVolumesTransferTimeout, maxVolumesTransferTimeout),
		Title:       "Volume export and import time limit",
		Description: "How long one volume export or import may run before it is cancelled, from 1m to 24h. A change applies to the next transfer.",
	})
	Register(Definition{
		Key: KeyVolumesImportOverwrite, Category: CategoryDeploys, Kind: KindBool, Default: "false",
		Env: "APP_VOLUMES_IMPORT_OVERWRITE", Apply: ApplyLive, Risk: RiskWeakensSecurity,
		Title:       "Allow volume import over existing files",
		Description: "When on, an import with the overwrite flag may replace a volume that already holds files. Off by default: imports only fill an empty volume. Take a backup first when you turn it on.",
	})
}
