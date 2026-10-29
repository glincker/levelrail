// Package templateupgrade plans a guarded upgrade of a deployed catalog
// template. It is pure: callers parse the stored and catalog compose
// bodies, the planner returns a JSON-serializable Plan and never touches
// a runtime, a store or a secret value.
package templateupgrade

import (
	"sort"
	"strings"

	"github.com/GLINCKER/levelrail/internal/compose"
)

// Risk grades how much attention an upgrade needs.
type Risk string

// Risk levels, from "nothing to do" to "needs a human".
const (
	RiskNone   Risk = "none"
	RiskLow    Risk = "low"
	RiskMedium Risk = "medium"
	RiskHigh   Risk = "high"
)

// Service statuses in a Plan.
const (
	ServiceUnchanged = "unchanged"
	ServiceChanged   = "changed"
	ServiceAdded     = "added"
	ServiceRemoved   = "removed"
)

// Blocker codes: any blocker stops an automatic upgrade.
const (
	BlockerVolumeRemoved    = "volume_removed"
	BlockerVolumeRetargeted = "volume_retargeted"
	BlockerServiceRemoved   = "service_removed_with_data"
)

// Input is everything the planner reads. Baseline is optional: the
// compose body of the first template revision of the app. With it, a
// value that differs from the baseline is a user edit; without it every
// value that differs from the catalog is treated as one.
type Input struct {
	AppID      string
	TemplateID string
	Installed  *compose.File
	Baseline   *compose.File
	Template   *compose.File
}

// Plan is the upgrade a later executor may apply: back up first, apply
// Services in order, roll back on failure. It carries names and tags only,
// never environment values.
type Plan struct {
	AppID       string        `json:"app_id"`
	TemplateID  string        `json:"template_id"`
	FromVersion string        `json:"from_version"`
	ToVersion   string        `json:"to_version"`
	Risk        Risk          `json:"risk"`
	HasChanges  bool          `json:"has_changes"`
	Automatic   bool          `json:"automatic"`
	Blockers    []Blocker     `json:"blockers"`
	Services    []ServicePlan `json:"services"`
}

// Blocker is one reason an upgrade must not run unattended.
type Blocker struct {
	Code    string `json:"code"`
	Service string `json:"service"`
	Detail  string `json:"detail"`
}

// ServicePlan is the change set for one service key.
type ServicePlan struct {
	Name        string             `json:"name"`
	Status      string             `json:"status"`
	Image       *ImageChange       `json:"image,omitempty"`
	Env         []EnvChange        `json:"env,omitempty"`
	Volumes     []VolumeChange     `json:"volumes,omitempty"`
	Ports       []PortChange       `json:"ports,omitempty"`
	Healthcheck *HealthcheckChange `json:"healthcheck,omitempty"`
}

// changedServices counts services the executor has something to do for.
func (p Plan) changedServices() int {
	n := 0
	for _, s := range p.Services {
		if s.Status != ServiceUnchanged {
			n++
		}
	}
	return n
}

// Build computes the plan. Nil Installed or Template yields an empty plan.
func Build(in Input) Plan {
	p := Plan{AppID: in.AppID, TemplateID: in.TemplateID, Risk: RiskNone, Blockers: []Blocker{}, Services: []ServicePlan{}}
	if in.Installed == nil || in.Template == nil {
		return p
	}
	p.FromVersion = Version(in.Installed)
	p.ToVersion = Version(in.Template)

	names := map[string]struct{}{}
	for n := range in.Installed.Services {
		names[n] = struct{}{}
	}
	for n := range in.Template.Services {
		names[n] = struct{}{}
	}
	sorted := make([]string, 0, len(names))
	for n := range names {
		sorted = append(sorted, n)
	}
	sort.Strings(sorted)

	var sig signals
	for _, name := range sorted {
		inst, hasInst := in.Installed.Services[name]
		tpl, hasTpl := in.Template.Services[name]
		var base *compose.Service
		if in.Baseline != nil {
			if b, ok := in.Baseline.Services[name]; ok {
				base = &b
			}
		}
		switch {
		case hasInst && hasTpl:
			sp := planService(name, inst, tpl, base, in.Baseline != nil, &sig)
			p.Blockers = append(p.Blockers, volumeBlockers(name, sp)...)
			p.Services = append(p.Services, sp)
		case hasTpl:
			sig.newService = true
			p.Services = append(p.Services, ServicePlan{Name: name, Status: ServiceAdded, Image: &ImageChange{To: tpl.Image, Kind: KindNew}})
		default:
			if in.Baseline != nil && base == nil {
				continue // added by the user, not by the template
			}
			sig.removedService = true
			sp := ServicePlan{Name: name, Status: ServiceRemoved, Image: &ImageChange{From: inst.Image, Kind: KindRemoved}}
			for _, v := range inst.Volumes {
				if v.Name != "" {
					p.Blockers = append(p.Blockers, Blocker{Code: BlockerServiceRemoved, Service: name, Detail: "service owns volume " + v.Name})
				}
			}
			p.Services = append(p.Services, sp)
		}
	}
	sort.Slice(p.Blockers, func(i, j int) bool {
		a, b := p.Blockers[i], p.Blockers[j]
		if a.Code != b.Code {
			return a.Code < b.Code
		}
		if a.Service != b.Service {
			return a.Service < b.Service
		}
		return a.Detail < b.Detail
	})
	p.HasChanges = p.changedServices() > 0
	p.Risk = sig.risk(p.HasChanges, len(p.Blockers) > 0)
	p.Automatic = p.HasChanges && len(p.Blockers) == 0 && p.Risk != RiskHigh
	return p
}

// signals collects what drives the risk level while services are planned.
type signals struct {
	major, unclassified, removedEnv bool
	newService, removedService      bool
	otherChange                     bool
}

func (s signals) risk(changes, blocked bool) Risk {
	switch {
	case !changes:
		return RiskNone
	case blocked, s.major, s.unclassified, s.removedEnv, s.newService, s.removedService:
		return RiskHigh
	case s.otherChange:
		return RiskMedium
	default:
		return RiskLow
	}
}

// Version is the template's version stamp: its image tags, since catalog
// templates carry no version field of their own.
func Version(f *compose.File) string {
	if f == nil {
		return ""
	}
	parts := make([]string, 0, len(f.Services))
	for name, s := range f.Services {
		parts = append(parts, name+"="+s.Image)
	}
	sort.Strings(parts)
	return strings.Join(parts, ",")
}

func planService(name string, inst, tpl compose.Service, base *compose.Service, hasBaseline bool, sig *signals) ServicePlan {
	sp := ServicePlan{Name: name, Status: ServiceUnchanged}
	sp.Image = planImage(inst, tpl, base, sig)
	sp.Env = planEnv(inst, tpl, base, hasBaseline, sig)
	sp.Volumes = planVolumes(inst, tpl, base, hasBaseline, sig)
	sp.Ports = planPorts(inst, tpl, sig)
	sp.Healthcheck = planHealthcheck(inst, tpl, base, sig)
	if actionable(sp) {
		sp.Status = ServiceChanged
	}
	return sp
}

// actionable ignores entries the planner keeps for the user to see but the
// executor must not apply (pinned tags, edited values).
func actionable(sp ServicePlan) bool {
	if sp.Image != nil && !sp.Image.Preserved {
		return true
	}
	if sp.Healthcheck != nil && !sp.Healthcheck.Preserved {
		return true
	}
	if len(sp.Volumes) > 0 || len(sp.Ports) > 0 {
		return true
	}
	for _, e := range sp.Env {
		if e.Action != EnvKeepUser {
			return true
		}
	}
	return false
}
