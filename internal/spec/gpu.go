package spec

import (
	"encoding/json"
	"fmt"
	"strconv"

	"gopkg.in/yaml.v3"
)

// GPUAll is GPU.Count's "every GPU on the node" value.
const GPUAll = -1

// GPU is resources.gpu: `all`, a count, or {count, devices} where devices
// are NVIDIA indexes or UUIDs (devices wins over count).
type GPU struct {
	Count   int      `yaml:"count,omitempty" json:"count,omitempty"`
	Devices []string `yaml:"devices,omitempty" json:"devices,omitempty"`
}

func parseGPUCount(v string) (int, error) {
	if v == "all" {
		return GPUAll, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 1 {
		return 0, fmt.Errorf("resources.gpu: count must be \"all\" or a positive integer, got %q", v)
	}
	return n, nil
}

// UnmarshalYAML accepts `all`, an integer, or a mapping.
func (g *GPU) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind == yaml.ScalarNode {
		n, err := parseGPUCount(node.Value)
		if err != nil {
			return err
		}
		*g = GPU{Count: n}
		return nil
	}
	var raw struct {
		Count   yaml.Node `yaml:"count"`
		Devices []string  `yaml:"devices"`
	}
	if err := node.Decode(&raw); err != nil {
		return fmt.Errorf("resources.gpu: %w", err)
	}
	out := GPU{Devices: raw.Devices}
	if raw.Count.Kind == yaml.ScalarNode {
		n, err := parseGPUCount(raw.Count.Value)
		if err != nil {
			return err
		}
		out.Count = n
	}
	*g = out
	return nil
}

// UnmarshalJSON accepts "all", a number, or an object.
func (g *GPU) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		n, err := parseGPUCount(s)
		if err != nil {
			return err
		}
		*g = GPU{Count: n}
		return nil
	}
	var n int
	if err := json.Unmarshal(data, &n); err == nil {
		c, err := parseGPUCount(strconv.Itoa(n))
		if err != nil {
			return err
		}
		*g = GPU{Count: c}
		return nil
	}
	type plain GPU
	var p plain
	if err := json.Unmarshal(data, &p); err != nil {
		return fmt.Errorf("resources.gpu: %w", err)
	}
	*g = GPU(p)
	return nil
}

// Requested reports whether any GPU is being asked for.
func (g GPU) Requested() bool { return g.Count != 0 || len(g.Devices) > 0 }
