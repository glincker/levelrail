package database

import (
	"github.com/GLINCKER/levelrail/internal/dbparams"
	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/store"
)

// engineParamsLabelKey records a hash of the applied tuning args so a newly
// applied set replaces the container through the normal sequential replace.
const engineParamsLabelKey = "db.engine-params"

// renderLimits re-checks memory sizes against the database's own memory limit
// at render time; the admin ceiling without a limit was enforced at stage time.
func renderLimits(desired *store.DesiredDatabase) dbparams.Limits {
	if desired.Resources == nil || desired.Resources.MemoryBytes <= 0 {
		return dbparams.Limits{}
	}
	return dbparams.Limits{MaxMemoryBytes: dbparams.MemoryCeiling(desired.Resources.MemoryBytes, 0)}
}

// engineParamsSpec appends the applied, re-validated tuning args to the start
// command. Images whose entrypoint prepends the server binary for flag-only
// commands (redis, valkey, mongo, mysql) keep working with a nil base command.
func engineParamsSpec(desired *store.DesiredDatabase) func(*docker.ContainerSpec) {
	return func(spec *docker.ContainerSpec) {
		args := dbparams.Args(desired.Engine, desired.EngineParams.Map(), renderLimits(desired))
		if len(args) == 0 {
			return
		}
		spec.Command = append(append([]string(nil), spec.Command...), args...)
		spec.Labels[engineParamsLabelKey] = dbparams.Fingerprint(args)
	}
}
