package runtimesettings

import (
	"fmt"
	"strconv"
	"strings"
)

// KeySizePresets holds the named resource sizes offered for apps and databases.
const KeySizePresets = "apps.size_presets"

// DefaultSizePresets seeds the table when nothing is saved or in env.
const DefaultSizePresets = "small=0.5:512Mi,medium=1:1Gi,large=2:2Gi,xlarge=4:4Gi"

// SizePreset is one named CPU and memory size.
type SizePreset struct {
	Name        string  `json:"name"`
	CPU         float64 `json:"cpu"`
	MemoryBytes int64   `json:"memory_bytes"`
}

func init() {
	Register(Definition{
		Key: KeySizePresets, Category: CategoryDeploys, Kind: KindString, Default: DefaultSizePresets, Env: "APP_SIZE_PRESETS", Apply: ApplyLive,
		Title:       "Resource size presets",
		Description: "Named sizes for apps and databases as name=cpu:memory pairs, for example small=0.5:512Mi,large=2:2Gi. Memory ends in Mi or Gi. Apps already running keep their limits until scaled.",
		CheckValue: func(v string) error {
			_, err := ParseSizePresets(v)
			return err
		},
	})
}

// ParseSizePresets reads name=cpu:memory pairs; it rejects duplicates, zero sizes and bad units.
func ParseSizePresets(v string) ([]SizePreset, error) {
	var out []SizePreset
	seen := map[string]bool{}
	for _, part := range strings.Split(v, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		name, rest, ok := strings.Cut(part, "=")
		cpuStr, memStr, ok2 := strings.Cut(rest, ":")
		name = strings.ToLower(strings.TrimSpace(name))
		if !ok || !ok2 || name == "" {
			return nil, fmt.Errorf("preset %q must look like name=cpu:memory", part)
		}
		if seen[name] {
			return nil, fmt.Errorf("preset %q is listed twice", name)
		}
		seen[name] = true
		cpu, err := strconv.ParseFloat(strings.TrimSpace(cpuStr), 64)
		if err != nil || cpu <= 0 {
			return nil, fmt.Errorf("preset %q: cpu must be a positive number", name)
		}
		mem, err := parseSizeMemory(strings.TrimSpace(memStr))
		if err != nil {
			return nil, fmt.Errorf("preset %q: %w", name, err)
		}
		out = append(out, SizePreset{Name: name, CPU: cpu, MemoryBytes: mem})
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("at least one preset is required")
	}
	return out, nil
}

func parseSizeMemory(s string) (int64, error) {
	unit := int64(0)
	switch {
	case strings.HasSuffix(s, "Mi"):
		unit = 1 << 20
	case strings.HasSuffix(s, "Gi"):
		unit = 1 << 30
	default:
		return 0, fmt.Errorf("memory %q must end in Mi or Gi", s)
	}
	n, err := strconv.ParseInt(s[:len(s)-2], 10, 64)
	if err != nil || n <= 0 {
		return 0, fmt.Errorf("memory %q must be a positive whole number", s)
	}
	return n * unit, nil
}
