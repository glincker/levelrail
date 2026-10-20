package api

import (
	"context"
	"log/slog"
	"strings"

	"github.com/GLINCKER/levelrail/internal/catalog"
	"github.com/GLINCKER/levelrail/internal/compose"
	"github.com/GLINCKER/levelrail/internal/store"
)

// templateMemoryHeadroom: a recommendation is a working-set estimate, so the cap leaves room for peaks (a Node app aborted at exactly the recommended size).
const templateMemoryHeadroom = 2

type templateDefaultsStore interface {
	SaveServiceTemplateDefaults(ctx context.Context, d store.ServiceTemplateDefaults) error
	GetServiceTemplateDefaults(ctx context.Context, serviceName string) (*store.ServiceTemplateDefaults, error)
}

func (rt *Router) templateDefaults() templateDefaultsStore {
	if s, ok := rt.appCompose.(templateDefaultsStore); ok {
		return s
	}
	s, _ := rt.apps.(templateDefaultsStore)
	return s
}

// planTemplateDefaults applies a catalog template's recommended memory as the
// default limit and collects the strategy note for each service. A limit the
// service already has is kept, never lowered or replaced.
func (rt *Router) planTemplateDefaults(ctx context.Context, appName, templateID string, file *compose.File, services []store.DesiredService, previous []store.DesiredService) []store.ServiceTemplateDefaults {
	var recommended int64
	if tpl, ok := catalog.TemplateByID(templateID); ok {
		recommended = tpl.RecommendedMemoryBytes
	}
	prev := make(map[string]store.DesiredService, len(previous))
	for _, p := range previous {
		prev[p.Name] = p
	}
	defs := rt.templateDefaults()
	var out []store.ServiceTemplateDefaults
	for i := range services {
		svc := &services[i]
		d := store.ServiceTemplateDefaults{ServiceName: svc.Name, TemplateID: templateID, Strategy: svc.Strategy}
		if d.Strategy != "" {
			d.StrategyReason = file.Services[strings.TrimPrefix(svc.Name, appName+"-")].StrategyReason
		}
		if recommended > 0 && (svc.Resources == nil || svc.Resources.MemoryBytes == 0) {
			old, existed := prev[svc.Name]
			var limit int64
			switch {
			case existed && old.Resources != nil && old.Resources.MemoryBytes > 0:
				limit = old.Resources.MemoryBytes
				if defs != nil {
					if row, err := defs.GetServiceTemplateDefaults(ctx, svc.Name); err == nil && row != nil && row.MemoryBytes == old.Resources.MemoryBytes {
						d.MemoryBytes = row.MemoryBytes
					}
				}
			case !existed:
				limit = recommended * templateMemoryHeadroom
				d.MemoryBytes = limit
			}
			if limit > 0 {
				if svc.Resources == nil {
					svc.Resources = &store.ServiceResources{}
				}
				svc.Resources.MemoryBytes = limit
			}
		}
		out = append(out, d)
	}
	return out
}

func (rt *Router) saveTemplateDefaults(ctx context.Context, defaults []store.ServiceTemplateDefaults) {
	s := rt.templateDefaults()
	if s == nil {
		return
	}
	for _, d := range defaults {
		if err := s.SaveServiceTemplateDefaults(ctx, d); err != nil {
			rt.logger.Warn("api: deploy compose: save template defaults failed", slog.String("error", err.Error()), slog.String("service", d.ServiceName))
		}
	}
}

type templateNotes struct {
	strategyReason     string
	memoryFromTemplate bool
}

// templateNotesFor reports template-set defaults only while the service still has the value the template set.
func (rt *Router) templateNotesFor(ctx context.Context, svc store.DesiredService) templateNotes {
	s := rt.templateDefaults()
	if s == nil {
		return templateNotes{}
	}
	row, err := s.GetServiceTemplateDefaults(ctx, svc.Name)
	if err != nil || row == nil {
		return templateNotes{}
	}
	var n templateNotes
	if row.Strategy != "" && row.Strategy == svc.Strategy {
		n.strategyReason = row.StrategyReason
	}
	n.memoryFromTemplate = row.MemoryBytes > 0 && svc.Resources != nil && svc.Resources.MemoryBytes == row.MemoryBytes
	return n
}
