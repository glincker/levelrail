package store

import "github.com/GLINCKER/levelrail/internal/spec"

// ApplyRuntimeOptions stores the runtime block and routes shm_size and
// read_only through the resource fields that already carry drift handling.
// Call it after the service's Resources are built so they are not overwritten.
func ApplyRuntimeOptions(d *DesiredService, r *spec.RuntimeOptions) {
	if r == nil {
		return
	}
	d.Runtime = r
	if shm := r.ShmSizeBytes(); shm > 0 {
		if d.Resources == nil {
			d.Resources = &ServiceResources{}
		}
		d.Resources.ShmSizeBytes = shm
	}
	if r.ReadOnly != nil {
		if d.Resources == nil {
			d.Resources = &ServiceResources{}
		}
		on := *r.ReadOnly
		d.Resources.ReadOnlyRootfs = &on
	}
}
