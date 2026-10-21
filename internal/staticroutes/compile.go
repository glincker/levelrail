package staticroutes

import (
	"github.com/GLINCKER/levelrail/internal/spec"
)

// Input is what a deploy hands to Compile.
type Input = spec.StaticInput

// Compile validates every source and returns the ruleset.
func Compile(in Input, l Limits) (*Ruleset, error) {
	rs, err := spec.CompileStatic(in, l)
	if err != nil {
		return nil, err
	}
	return &Ruleset{*rs}, nil
}

// Prepare compiles the matchers of a ruleset read back from JSON.
func (rs *Ruleset) Prepare() error { return rs.StaticRuleset.Prepare() }

// SidecarPath is where the ruleset of the tree at rootDir is stored.
func SidecarPath(rootDir string) string { return spec.StaticSidecarPath(rootDir) }

// WriteSidecar stores rs beside the tree at rootDir, replacing any earlier one.
func WriteSidecar(rootDir string, rs *Ruleset) error {
	return spec.WriteStaticSidecar(rootDir, &rs.StaticRuleset)
}

// LoadSidecar reads the ruleset of the tree at rootDir; none yields nil without an error.
func LoadSidecar(rootDir string) (*Ruleset, error) {
	rs, err := spec.LoadStaticSidecar(rootDir)
	if err != nil || rs == nil {
		return nil, err
	}
	return &Ruleset{*rs}, nil
}

func validateOptions(o *Options) error { return spec.ValidateStaticOptions(o) }
