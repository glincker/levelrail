package runtimesettings

import "context"

// Registered keys for managed container log rotation.
const (
	KeyContainersLogMaxSizeMB = "containers.log_max_size_mb"
	KeyContainersLogMaxFiles  = "containers.log_max_files"
)

func init() {
	Register(Definition{
		Key: KeyContainersLogMaxSizeMB, Category: CategoryDeploys, Kind: KindInt, Default: "0", Env: "APP_CONTAINER_LOG_MAX_SIZE_MB", Apply: ApplyLive,
		CheckValue:  intRangeCheck(0, 10240),
		Title:       "Containers: log file size (MB)",
		Description: "Largest json-file log before Docker rotates it, for containers created or recreated after the change. 0 leaves the Docker default and recreates nothing. Rotation needs both this and the file count above 0.",
	})
	Register(Definition{
		Key: KeyContainersLogMaxFiles, Category: CategoryDeploys, Kind: KindInt, Default: "0", Env: "APP_CONTAINER_LOG_MAX_FILES", Apply: ApplyLive,
		CheckValue:  intRangeCheck(0, 100),
		Title:       "Containers: rotated log files",
		Description: "Rotated log files Docker keeps per container, for containers created or recreated after the change. 0 leaves the Docker default.",
	})
}

// ContainerLogRotation returns the live rotation limits; 0 means unchanged.
func (s *Service) ContainerLogRotation(ctx context.Context) (sizeMB, files int) {
	return s.Int(ctx, KeyContainersLogMaxSizeMB), s.Int(ctx, KeyContainersLogMaxFiles)
}
