package compose

import (
	"fmt"
	"sort"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/GLINCKER/levelrail/internal/spec"
)

// stringList accepts a single string or a list, as tmpfs: and dns: do.
type stringList []string

// UnmarshalYAML implements the string-or-list union.
func (l *stringList) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind == yaml.ScalarNode {
		*l = stringList{node.Value}
		return nil
	}
	var items []string
	if err := node.Decode(&items); err != nil {
		return fmt.Errorf("expected a string or a list of strings")
	}
	*l = items
	return nil
}

// hostList accepts extra_hosts as "host:ip" entries or a host: ip map.
type hostList []string

// UnmarshalYAML implements the list-or-map union.
func (h *hostList) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind == yaml.MappingNode {
		var m map[string]string
		if err := node.Decode(&m); err != nil {
			return fmt.Errorf("extra_hosts: expected host: ip pairs")
		}
		for k, v := range m {
			*h = append(*h, k+":"+v)
		}
		sort.Strings(*h)
		return nil
	}
	var items []string
	if err := node.Decode(&items); err != nil {
		return fmt.Errorf("extra_hosts: expected a list of host:ip entries")
	}
	*h = items
	return nil
}

// sysctlMap accepts sysctls: as a map or a list of "key=value" entries.
type sysctlMap map[string]string

// UnmarshalYAML implements the map-or-list union.
func (s *sysctlMap) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind == yaml.MappingNode {
		var m map[string]string
		if err := node.Decode(&m); err != nil {
			return fmt.Errorf("sysctls: expected key: value pairs")
		}
		*s = m
		return nil
	}
	var items []string
	if err := node.Decode(&items); err != nil {
		return fmt.Errorf("sysctls: expected a map or key=value list")
	}
	out := make(sysctlMap, len(items))
	for _, it := range items {
		k, v := splitKV(it)
		out[k] = v
	}
	*s = out
	return nil
}

func splitKV(s string) (string, string) {
	for i := 0; i < len(s); i++ {
		if s[i] == '=' {
			return s[:i], s[i+1:]
		}
	}
	return s, ""
}

// runtimeFromService maps the tuning keys onto the allowlisted options. A set
// that fails validation is not applied at all and the reason is returned, so a
// pasted file keeps deploying the way it did before these keys were read.
func runtimeFromService(svc Service) (*spec.RuntimeOptions, string) {
	r := &spec.RuntimeOptions{
		Tmpfs: svc.Tmpfs, Ulimits: svc.Ulimits, Init: svc.Init, ExtraHosts: svc.ExtraHosts,
		DNS: svc.DNS, Sysctls: svc.Sysctls, CapDrop: svc.CapDrop, PidsLimit: svc.PidsLimit,
	}
	if svc.ReadOnly {
		on := true
		r.ReadOnly = &on
	}
	if svc.StopGracePeriod != "" {
		d, err := time.ParseDuration(svc.StopGracePeriod)
		if err != nil || d < 0 {
			return nil, "stop_grace_period: not applied, expected a duration such as 30s"
		}
		secs := int(d.Seconds())
		r.StopTimeout = &secs
	}
	if len(r.ComposeKeys()) == 0 {
		return nil, ""
	}
	if err := r.Validate(); err != nil {
		return nil, fmt.Sprintf("%v; the runtime options (tmpfs, ulimits, sysctls, dns, extra_hosts, cap_drop, read_only, init, pids_limit, stop_grace_period) were not applied", err)
	}
	return r, ""
}
