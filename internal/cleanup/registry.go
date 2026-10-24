package cleanup

import (
	"fmt"
	"strings"
	"sync"

	"github.com/GLINCKER/levelrail/internal/runtimesettings"
)

// Registry holds the registered targets in registration order.
type Registry struct {
	mu              sync.RWMutex
	targets         []Target
	registerSetting func(runtimesettings.Definition)
}

// NewRegistry builds a registry. registerSetting receives each target's mode setting; nil skips registration, for tests.
func NewRegistry(registerSetting func(runtimesettings.Definition)) *Registry {
	return &Registry{registerSetting: registerSetting}
}

// Default is the process-wide registry whose mode settings join the runtime settings registry.
var Default = NewRegistry(runtimesettings.Register)

// Register adds t to the default registry.
func Register(t Target) { Default.Register(t) }

// Targets lists the default registry's targets.
func Targets() []Target { return Default.Targets() }

// Get finds a target in the default registry.
func Get(id string) (Target, bool) { return Default.Get(id) }

// ModeKey is the runtime setting holding a target's mode.
func ModeKey(id string) string { return "cleanup." + id + ".mode" }

// DefaultMode is the mode a target starts in: unchanged behavior for existing sweepers, preview for new ones.
func DefaultMode(o Origin) Mode {
	if o == OriginNew {
		return ModePreview
	}
	return ModeOn
}

// Register adds t and its mode setting. It panics on a malformed or duplicate target so a bad registration fails at start.
func (r *Registry) Register(t Target) {
	m := t.Meta()
	if strings.TrimSpace(m.ID) == "" || m.Category == "" || m.Title == "" {
		panic("cleanup: target needs an id, category and title")
	}
	if m.Origin != OriginNew && m.Origin != OriginExisting {
		panic("cleanup: target " + m.ID + " has no origin")
	}
	if !m.ReadOnly && m.PolicySetting == "" {
		panic("cleanup: target " + m.ID + " has no policy setting")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, e := range r.targets {
		if e.Meta().ID == m.ID {
			panic("cleanup: duplicate target " + m.ID)
		}
	}
	if !m.ReadOnly && r.registerSetting != nil {
		r.registerSetting(ModeDefinition(m))
	}
	r.targets = append(r.targets, t)
}

// ModeDefinition is the runtime setting for m's mode.
func ModeDefinition(m Meta) runtimesettings.Definition {
	return runtimesettings.Definition{
		Key: ModeKey(m.ID), Category: runtimesettings.CategoryRetention, Kind: runtimesettings.KindString,
		Default: string(DefaultMode(m.Origin)), AllowedStrings: []string{string(ModeOff), string(ModePreview), string(ModeOn)},
		Apply:    runtimesettings.ApplyLive,
		TitleKey: "cleanup:mode.settingTitle", DescKey: "cleanup:mode.settingDescription",
		Title:       fmt.Sprintf("Cleanup mode: %s", m.Title),
		Description: "Off skips the target, preview records what a scheduled run would delete, on deletes.",
	}
}

// Targets returns a copy of the registered targets.
func (r *Registry) Targets() []Target {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return append([]Target(nil), r.targets...)
}

// Get finds a target by id.
func (r *Registry) Get(id string) (Target, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, t := range r.targets {
		if t.Meta().ID == id {
			return t, true
		}
	}
	return nil, false
}
