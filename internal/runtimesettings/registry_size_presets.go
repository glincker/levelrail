package runtimesettings

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// KeyResourcesSizePresets names the preset table behind --size and the resources card.
const KeyResourcesSizePresets = "resources.size_presets"

// DefaultSizePresets seeds the table when nothing is saved or in env.
const DefaultSizePresets = "small=0.5:512Mi,medium=1:1Gi,large=2:2Gi,xlarge=4:4Gi"

// SizePreset is one named CPU and memory pair.
type SizePreset struct {
	Name        string  `json:"name"`
	CPU         float64 `json:"cpu"`
	MemoryBytes int64   `json:"memory_bytes"`
}

var (
	sizePresetName = regexp.MustCompile(`^[a-z][a-z0-9-]{0,31}$`)
	sizeMemory     = regexp.MustCompile(`^([0-9]+)(Mi|Gi)$`)
)

// ParseSizePresets reads "name=cpu:memory" entries separated by commas, e.g. small=0.5:512Mi.
func ParseSizePresets(raw string) ([]SizePreset, error) {
	var out []SizePreset
	seen := map[string]bool{}
	for _, entry := range strings.Split(raw, ",") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		name, rest, ok := strings.Cut(entry, "=")
		cpuText, memText, ok2 := strings.Cut(rest, ":")
		if !ok || !ok2 || !sizePresetName.MatchString(name) {
			return nil, fmt.Errorf("entry %q must look like name=cpu:memory, e.g. small=0.5:512Mi", entry)
		}
		if seen[name] {
			return nil, fmt.Errorf("preset %q is listed twice", name)
		}
		cpu, err := strconv.ParseFloat(cpuText, 64)
		if err != nil || cpu <= 0 || cpu > 1024 {
			return nil, fmt.Errorf("preset %q has an invalid cpu %q", name, cpuText)
		}
		m := sizeMemory.FindStringSubmatch(memText)
		if m == nil {
			return nil, fmt.Errorf("preset %q memory %q must be a whole number of Mi or Gi", name, memText)
		}
		n, err := strconv.ParseInt(m[1], 10, 64)
		if err != nil || n <= 0 || n > 1<<20 {
			return nil, fmt.Errorf("preset %q has an invalid memory %q", name, memText)
		}
		bytes := n << 20
		if m[2] == "Gi" {
			bytes = n << 30
		}
		seen[name] = true
		out = append(out, SizePreset{Name: name, CPU: cpu, MemoryBytes: bytes})
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("at least one preset is required")
	}
	return out, nil
}

func init() {
	Register(Definition{
		Key: KeyResourcesSizePresets, Category: CategoryDeploys, Kind: KindString,
		Default: DefaultSizePresets,
		Env:     "APP_RESOURCES_SIZE_PRESETS", Apply: ApplyLive,
		CheckValue:  func(v string) error { _, err := ParseSizePresets(v); return err },
		Title:       "Resource size presets",
		Description: "Named CPU and memory pairs offered by apps create --size, apps scale --size and the resources card, written name=cpu:memory and separated by commas, for example small=0.5:512Mi. A change applies to the next request.",
	})
}
