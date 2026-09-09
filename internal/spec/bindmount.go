package spec

import "github.com/GLINCKER/levelrail/internal/bindmount"

// validateBindMountHostPath delegates to internal/bindmount, the copy
// shared with internal/compose (see that package's own doc comment for
// why neither compose nor spec can hold this directly).
func validateBindMountHostPath(hostPath string) error {
	return bindmount.ValidateHostPath(hostPath)
}
