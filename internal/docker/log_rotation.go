package docker

import (
	"strconv"
	"sync/atomic"

	"github.com/docker/docker/api/types/container"
)

// LogRotation is the json-file rotation applied to containers created from now on. Zero in either field leaves the daemon default.
type LogRotation struct {
	MaxSizeMB int
	MaxFiles  int
}

var logRotationSource atomic.Pointer[func() LogRotation]

// SetLogRotationSource makes new containers follow a live setting; nil clears it.
func SetLogRotationSource(fn func() LogRotation) {
	if fn == nil {
		logRotationSource.Store(nil)
		return
	}
	logRotationSource.Store(&fn)
}

func currentLogRotation() LogRotation {
	if p := logRotationSource.Load(); p != nil {
		return (*p)()
	}
	return LogRotation{}
}

// applyLogRotation sets the log driver options only when both limits are positive, so the default of 0 changes nothing.
func applyLogRotation(hc *container.HostConfig, lr LogRotation) {
	if lr.MaxSizeMB <= 0 || lr.MaxFiles <= 0 || hc.LogConfig.Type != "" {
		return
	}
	hc.LogConfig = container.LogConfig{
		Type:   "json-file",
		Config: map[string]string{"max-size": strconv.Itoa(lr.MaxSizeMB) + "m", "max-file": strconv.Itoa(lr.MaxFiles)},
	}
}
