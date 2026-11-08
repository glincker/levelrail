package deploy

import (
	"github.com/GLINCKER/levelrail/internal/spec"
	"github.com/GLINCKER/levelrail/internal/store"
)

// applyRuntimeOptions stores the runtime block and routes shm_size and
// read_only through the resource fields that already carry drift handling.
func applyRuntimeOptions(d *store.DesiredService, r *spec.RuntimeOptions) {
	if r == nil {
		return
	}
	d.Runtime = r
	if shm := r.ShmSizeBytes(); shm > 0 {
		if d.Resources == nil {
			d.Resources = &store.ServiceResources{}
		}
		d.Resources.ShmSizeBytes = shm
	}
	if r.ReadOnly != nil {
		if d.Resources == nil {
			d.Resources = &store.ServiceResources{}
		}
		on := *r.ReadOnly
		d.Resources.ReadOnlyRootfs = &on
	}
}
