package api

import (
	"context"
	"time"

	"github.com/GLINCKER/levelrail/internal/catalog"
	"github.com/GLINCKER/levelrail/internal/runtimesettings"
	"github.com/GLINCKER/levelrail/internal/store"
)

// applyFirstBootTimeout gives each service of a catalog template install the
// manifest's first_boot_seconds as its ready budget, clamped by the runtime
// ceiling. It only ever raises a budget the compose file already set.
func (rt *Router) applyFirstBootTimeout(ctx context.Context, templateID string, services []store.DesiredService) {
	tpl, ok := catalog.TemplateByID(templateID)
	if !ok || tpl.Manifest == nil {
		return
	}
	wait := time.Duration(tpl.Manifest.FirstBootWait(rt.runtimeSettings.Int(ctx, runtimesettings.KeyTemplatesFirstBootMaxSeconds))) * time.Second
	if wait <= 0 {
		return
	}
	for i := range services {
		if services[i].Health == nil {
			services[i].Health = &store.ServiceHealth{}
		}
		if services[i].Health.ReadyTimeout < wait {
			services[i].Health.ReadyTimeout = wait
		}
	}
}
