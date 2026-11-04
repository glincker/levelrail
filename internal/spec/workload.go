package spec

import (
	"fmt"
	"strings"
)

// Workload kinds. Reserved, not yet scheduled: the reconciler ignores these
// fields, so a spec that sets them validates and behaves like replicated.
const (
	WorkloadReplicated = "replicated"
	WorkloadPerNode    = "per-node"
	WorkloadOrdered    = "ordered"
)

// Per-node drain behavior and ordered start modes.
const (
	PerNodeDrainKeep  = "keep"
	PerNodeDrainEvict = "evict"
	OrderedSequential = "sequential"
	OrderedParallel   = "parallel"
)

// maxWorkloadParentName leaves room for the longest child suffix ("-n" plus
// 6 hex) inside the 63 character DNS label limit.
const maxWorkloadParentName = 55

// PerNode tunes a workload: per-node service.
type PerNode struct {
	OnDrain string `yaml:"onDrain,omitempty"`
}

// Ordered tunes a workload: ordered service.
type Ordered struct {
	Replicas  int    `yaml:"replicas,omitempty"`
	Start     string `yaml:"start,omitempty"`
	Partition int    `yaml:"partition,omitempty"`
}

// EffectiveWorkload returns the workload kind with the default applied.
func (svc *Service) EffectiveWorkload() string {
	if svc.Workload == "" {
		return WorkloadReplicated
	}
	return svc.Workload
}

// validateWorkload checks the per-node and ordered fields and the v1
// refusals. Nothing here changes scheduling.
func (svc *Service) validateWorkload(name string) error {
	kind := svc.EffectiveWorkload()
	switch kind {
	case WorkloadReplicated, WorkloadPerNode, WorkloadOrdered:
	default:
		return fmt.Errorf("spec: service %q: workload %q is not one of replicated, per-node, ordered", name, svc.Workload)
	}
	if kind != WorkloadPerNode && svc.PerNode != nil {
		return fmt.Errorf("spec: service %q: perNode requires workload: per-node", name)
	}
	if kind != WorkloadOrdered && svc.Ordered != nil {
		return fmt.Errorf("spec: service %q: ordered requires workload: ordered", name)
	}
	if kind == WorkloadReplicated {
		return nil
	}
	if len(name) > maxWorkloadParentName {
		return fmt.Errorf("spec: service %q: a %s service name is at most %d characters so instance names fit a DNS label", name, kind, maxWorkloadParentName)
	}
	if svc.Build.Type == "static" || svc.Build.Type == "compose" {
		return fmt.Errorf("spec: service %q: workload %s needs a single container build, not %s", name, kind, svc.Build.Type)
	}
	if len(svc.Domains) > 0 {
		return fmt.Errorf("spec: service %q: domains are not supported on a %s service yet", name, kind)
	}
	if svc.Rollout != nil && !rolloutValueIsZero(svc.Rollout.MaxSurge) {
		return fmt.Errorf("spec: service %q: rollout.max_surge must be 0 on a %s service", name, kind)
	}
	if kind == WorkloadPerNode {
		return svc.validatePerNode(name)
	}
	return svc.validateOrdered(name)
}

func (svc *Service) validatePerNode(name string) error {
	if svc.Replicas != 0 {
		return fmt.Errorf("spec: service %q: replicas is not supported on a per-node service, it runs once per eligible node", name)
	}
	if svc.Strategy != "" {
		return fmt.Errorf("spec: service %q: strategy is not supported on a per-node service", name)
	}
	if p := svc.PerNode; p != nil {
		switch p.OnDrain {
		case "", PerNodeDrainKeep, PerNodeDrainEvict:
		default:
			return fmt.Errorf("spec: service %q: perNode.onDrain %q is not one of keep, evict", name, p.OnDrain)
		}
	}
	return nil
}

func (svc *Service) validateOrdered(name string) error {
	o := svc.Ordered
	if o == nil || o.Replicas < 1 {
		return fmt.Errorf("spec: service %q: ordered.replicas must be at least 1", name)
	}
	switch o.Start {
	case "", OrderedSequential, OrderedParallel:
	default:
		return fmt.Errorf("spec: service %q: ordered.start %q is not one of sequential, parallel", name, o.Start)
	}
	if o.Partition < 0 || o.Partition > o.Replicas {
		return fmt.Errorf("spec: service %q: ordered.partition must be between 0 and ordered.replicas", name)
	}
	return nil
}

// rolloutValueIsZero reports whether a rollout value means none: empty, 0 or 0%.
func rolloutValueIsZero(v string) bool {
	v = strings.TrimSuffix(strings.TrimSpace(v), "%")
	return strings.Trim(v, "0") == ""
}
