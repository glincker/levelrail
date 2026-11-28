package deploy

import (
	"github.com/glincker/levelrail/internal/spec"
	"github.com/glincker/levelrail/internal/store"
)

func applyRuntimeOptions(d *store.DesiredService, r *spec.RuntimeOptions) {
	store.ApplyRuntimeOptions(d, r)
}
