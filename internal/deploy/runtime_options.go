package deploy

import (
	"github.com/GLINCKER/levelrail/internal/spec"
	"github.com/GLINCKER/levelrail/internal/store"
)

func applyRuntimeOptions(d *store.DesiredService, r *spec.RuntimeOptions) {
	store.ApplyRuntimeOptions(d, r)
}
