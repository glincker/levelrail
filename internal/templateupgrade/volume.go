package templateupgrade

import (
	"sort"

	"github.com/GLINCKER/levelrail/internal/compose"
)

// Volume and port actions.
const (
	VolumeAdd       = "add"
	VolumeRemove    = "remove"
	VolumeRetarget  = "retarget"
	PortAdd         = "add"
	PortRemove      = "remove"
	ReasonRemoved   = "named volume no longer in the template"
	ReasonRetargets = "named volume mounted at a different path"
)

// VolumeChange covers named volumes only; bind mounts are user-owned.
// Remove and retarget are blockers.
type VolumeChange struct {
	Name   string `json:"name"`
	Action string `json:"action"`
	From   string `json:"from,omitempty"`
	To     string `json:"to,omitempty"`
}

func namedVolumes(vols []compose.Volume) map[string]string {
	m := map[string]string{}
	for _, v := range vols {
		if v.Name != "" {
			m[v.Name] = v.ContainerPath
		}
	}
	return m
}

func planVolumes(inst, tpl compose.Service, base *compose.Service, hasBaseline bool, sig *signals) []VolumeChange {
	iv, tv := namedVolumes(inst.Volumes), namedVolumes(tpl.Volumes)
	var bv map[string]string
	if base != nil {
		bv = namedVolumes(base.Volumes)
	}
	pathToName := func(m map[string]string, path string) string {
		for n, p := range m {
			if p == path {
				return n
			}
		}
		return ""
	}
	var out []VolumeChange
	for name, path := range iv {
		tp, inTpl := tv[name]
		switch {
		case inTpl && tp != path:
			out = append(out, VolumeChange{Name: name, Action: VolumeRetarget, From: path, To: tp})
		case inTpl:
		default:
			if _, ok := bv[name]; hasBaseline && !ok {
				continue
			}
			if other := pathToName(tv, path); other != "" {
				out = append(out, VolumeChange{Name: name, Action: VolumeRetarget, From: name, To: other})
			} else {
				out = append(out, VolumeChange{Name: name, Action: VolumeRemove, From: path})
			}
		}
	}
	for name, path := range tv {
		if _, ok := iv[name]; ok || pathToName(iv, path) != "" {
			continue
		}
		out = append(out, VolumeChange{Name: name, Action: VolumeAdd, To: path})
		sig.otherChange = true
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Name != out[j].Name {
			return out[i].Name < out[j].Name
		}
		return out[i].Action < out[j].Action
	})
	return out
}

func volumeBlockers(service string, sp ServicePlan) []Blocker {
	var out []Blocker
	for _, v := range sp.Volumes {
		switch v.Action {
		case VolumeRemove:
			out = append(out, Blocker{Code: BlockerVolumeRemoved, Service: service, Detail: ReasonRemoved + ": " + v.Name})
		case VolumeRetarget:
			out = append(out, Blocker{Code: BlockerVolumeRetargeted, Service: service, Detail: ReasonRetargets + ": " + v.Name})
		}
	}
	return out
}

// PortChange is a published or container port the template adds or drops.
type PortChange struct {
	Port     int    `json:"port"`
	Protocol string `json:"protocol,omitempty"`
	Action   string `json:"action"`
}

func planPorts(inst, tpl compose.Service, sig *signals) []PortChange {
	key := func(p compose.Port) PortChange { return PortChange{Port: p.ContainerPort, Protocol: p.Protocol} }
	set := func(ps []compose.Port) map[PortChange]bool {
		m := map[PortChange]bool{}
		for _, p := range ps {
			m[key(p)] = true
		}
		return m
	}
	is, ts := set(inst.Ports), set(tpl.Ports)
	var out []PortChange
	for k := range ts {
		if !is[k] {
			k.Action = PortAdd
			out = append(out, k)
		}
	}
	for k := range is {
		if !ts[k] {
			k.Action = PortRemove
			out = append(out, k)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Port != out[j].Port {
			return out[i].Port < out[j].Port
		}
		return out[i].Action < out[j].Action
	})
	if len(out) > 0 {
		sig.otherChange = true
	}
	return out
}
