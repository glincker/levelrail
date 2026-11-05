package runtimesettings

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Registered keys for moving a volume's files in and out as an archive.
const (
	KeyVolumesTransferMaxBytes = "volumes.transfer_max_bytes"
	KeyVolumesTransferTimeout  = "volumes.transfer_timeout"
	KeyVolumesImportOverwrite  = "volumes.import_overwrite"
	// KeyVolumesTransferMaxConcurrent bounds uploads staged at once on a control plane.
	KeyVolumesTransferMaxConcurrent = "volumes.transfer_max_concurrent"
	// KeyVolumesTransferSpoolDir names the folder under the data dir that stages import archives.
	KeyVolumesTransferSpoolDir   = "volumes.transfer_spool_dir"
	volumesTransferMaxFloor      = 1 << 20
	volumesTransferMaxCeiling    = 1 << 42
	minVolumesTransferTimeout    = time.Minute
	maxVolumesTransferTimeout    = 24 * time.Hour
	volumesTransferConcurrentMax = 16
	// DefaultVolumesTransferSpoolDir is the spool folder name under the data dir.
	DefaultVolumesTransferSpoolDir = "volume-transfer"
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
		Env: "APP_VOLUMES_IMPORT_OVERWRITE", Apply: ApplyLive, Risk: RiskWeakensSecurity, TrueIsWeaker: true,
		Title:       "Allow volume import over existing files",
		Description: "When on, an import with the overwrite flag may replace a volume that already holds files. Off by default: imports only fill an empty volume. Take a backup first when you turn it on.",
	})
	registerLimit(Definition{
		Key: KeyVolumesTransferMaxConcurrent, Category: CategoryDeploys, Default: "1", Env: "APP_VOLUMES_TRANSFER_MAX_CONCURRENT", Apply: ApplyLive,
		Title:       "Concurrent volume imports",
		Description: "How many volume imports this control plane stages at once, from 1 to 16. A further import is refused with HTTP 429 until one finishes. A change applies to the next import.",
	}, 1, volumesTransferConcurrentMax)
	Register(Definition{
		Key: KeyVolumesTransferSpoolDir, Category: CategoryDeploys, Kind: KindString, Default: DefaultVolumesTransferSpoolDir,
		Env: "APP_VOLUMES_TRANSFER_SPOOL_DIR", Apply: ApplyLive, CheckValue: spoolDirCheck,
		Title:       "Volume import spool folder",
		Description: "Folder name under the data directory where an import archive is staged while it is checked. A plain name, no slashes. Leftover files in it are removed when the control plane starts. A change applies to the next import.",
	})
}

// spoolDirCheck accepts one plain folder name so the spool can never leave the data dir.
func spoolDirCheck(v string) error {
	if v == "" || v == "." || v == ".." || strings.ContainsAny(v, "/\\\x00") {
		return fmt.Errorf("value must be a plain folder name without slashes")
	}
	return nil
}
