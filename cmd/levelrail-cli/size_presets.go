package main

import (
	"context"
	"strings"
)

// applySizePreset sets the preset's CPU and memory on res, leaving swap and cpuset alone.
func applySizePreset(ctx context.Context, client *Client, name string, res *serviceResources) error {
	presets, err := client.SizePresets(ctx)
	if err != nil {
		return err
	}
	names := make([]string, 0, len(presets))
	for _, p := range presets {
		if p.Name == name {
			res.MemoryBytes = p.MemoryBytes
			res.NanoCPUs = int64(p.CPU * 1e9)
			return nil
		}
		names = append(names, p.Name)
	}
	return newValidationError("--size %q is not a preset, choose one of: %s", name, strings.Join(names, ", "))
}
