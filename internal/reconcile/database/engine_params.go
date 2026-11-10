package database

import (
	"log/slog"
	"sync"

	"github.com/GLINCKER/levelrail/internal/dbparams"
	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/store"
)

// engineParamsLabelKey records a hash of the applied tuning args so a newly
// applied set replaces the container through the normal sequential replace.
const engineParamsLabelKey = "db.engine-params"

// WithParamsMaxMemory supplies the live admin memory ceiling in MB, so a
// database without a memory limit is re-checked at render time too.
func WithParamsMaxMemory(fn func() int) Option {
	return func(c *Controller) { c.paramsMaxMemoryMB = fn }
}

// renderLimits re-checks memory sizes against 75 percent of the database's own
// memory limit, or the admin ceiling when it has none. adminMB <= 0 means no cap.
func renderLimits(desired *store.DesiredDatabase, adminMB int) dbparams.Limits {
	var limit int64
	if desired.Resources != nil {
		limit = desired.Resources.MemoryBytes
	}
	if limit <= 0 && adminMB <= 0 {
		return dbparams.Limits{}
	}
	return dbparams.Limits{MaxMemoryBytes: dbparams.MemoryCeiling(limit, adminMB)}
}

// engineParamsSpec appends the applied, re-validated tuning args to the start
// command. Images whose entrypoint prepends the server binary for flag-only
// commands (redis, valkey, mongo, mysql) keep working with a nil base command.
// The label hashes the stored set, so a changed ceiling alone never replaces
// the container; the clamp lands at the next replace or restart.
func (c *Controller) engineParamsSpec(desired *store.DesiredDatabase) func(*docker.ContainerSpec) {
	return func(spec *docker.ContainerSpec) {
		stored := desired.EngineParams.Map()
		storedArgs := dbparams.Args(desired.Engine, stored, dbparams.Limits{})
		if len(storedArgs) == 0 {
			return
		}
		adminMB := 0
		if c.paramsMaxMemoryMB != nil {
			adminMB = c.paramsMaxMemoryMB()
		}
		lim := renderLimits(desired, adminMB)
		rendered := make(map[string]string, len(stored))
		for k, v := range stored {
			if cv, clamped := dbparams.ClampMemory(desired.Engine, k, v, lim); clamped {
				logClampOnce(desired.Name, k)
				v = cv
			}
			rendered[k] = v
		}
		args := dbparams.Args(desired.Engine, rendered, lim)
		spec.Command = append(append([]string(nil), spec.Command...), args...)
		spec.Labels[engineParamsLabelKey] = dbparams.Fingerprint(storedArgs)
	}
}

// clampLogged keeps the clamp warning to one line per database and parameter
// per process, since every reconcile pass renders the spec again.
var clampLogged sync.Map

func logClampOnce(dbName, param string) {
	if _, seen := clampLogged.LoadOrStore(dbName+"/"+param, struct{}{}); seen {
		return
	}
	slog.Warn("database: memory parameter lowered to the current ceiling",
		slog.String("database", dbName), slog.String("param", param))
}
