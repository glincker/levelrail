package main

import (
	"fmt"
	"strconv"

	"github.com/GLINCKER/levelrail/internal/spec"
	"gopkg.in/yaml.v3"
)

// buildAppYAML renders m as a Levelrail app.yaml document with a single
// service, m.ServiceName. Built as a yaml.Node tree rather than
// yaml.Marshal(spec.Spec{...}) directly: spec.EnvVar has no MarshalYAML,
// so marshaling it as a plain Go struct would emit every zero-value
// field (value/from/secret/required) alongside secret: true, violating
// the schema's additionalProperties: false on envValue's object shape.
//
// Env values are never written literally here, regardless of whether
// real values were fetched from Coolify: every key becomes
// { secret: true }. A real fetched value lives only in
// mappedService.EnvLiteral, consumed by writeSecretsEnvFile (a separate
// file, never app.yaml) or applyMigration's PUT .../secrets/{key} calls,
// never by this function.
func buildAppYAML(m mappedApp) ([]byte, error) {
	svc := m.Service

	buildPairs := []*yaml.Node{scalarNode("type"), scalarNode(svc.BuildType)}
	if svc.BuildPath != "" {
		buildPairs = append(buildPairs, scalarNode("path"), scalarNode(svc.BuildPath))
	}

	servicePairs := []*yaml.Node{scalarNode("build"), mappingNode(buildPairs...)}
	if len(svc.Domains) > 0 {
		servicePairs = append(servicePairs, scalarNode("domains"), stringSeqNode(svc.Domains))
	}
	if svc.Port > 0 {
		servicePairs = append(servicePairs, scalarNode("port"), intNode(svc.Port))
	}
	if svc.Health != nil {
		servicePairs = append(servicePairs, scalarNode("health"), healthNode(*svc.Health))
	}
	if svc.Memory != "" || svc.CPU > 0 {
		servicePairs = append(servicePairs, scalarNode("resources"), resourcesNode(svc))
	}
	if len(svc.EnvKeys) > 0 {
		servicePairs = append(servicePairs, scalarNode("env"), envNode(svc.EnvKeys))
	}

	doc := mappingNode(
		scalarNode("version"), intNode(1),
		scalarNode("services"), mappingNode(
			scalarNode(m.ServiceName), mappingNode(servicePairs...),
		),
	)

	data, err := yaml.Marshal(doc)
	if err != nil {
		return nil, fmt.Errorf("render app.yaml for %q: %w", m.ServiceName, err)
	}

	if _, err := spec.Parse(data); err != nil {
		return nil, fmt.Errorf("generated app.yaml for %q failed validation: %w", m.ServiceName, err)
	}
	return data, nil
}

func healthNode(h mappedHealth) *yaml.Node {
	return mappingNode(
		scalarNode("readiness"), probeNode(h),
		scalarNode("liveness"), probeNode(h),
	)
}

func probeNode(h mappedHealth) *yaml.Node {
	pairs := []*yaml.Node{scalarNode("path"), scalarNode(h.Path)}
	if h.Interval > 0 {
		pairs = append(pairs, scalarNode("interval"), scalarNode(fmt.Sprintf("%ds", h.Interval)))
	}
	if h.Timeout > 0 {
		pairs = append(pairs, scalarNode("timeout"), scalarNode(fmt.Sprintf("%ds", h.Timeout)))
	}
	if h.Failures > 0 {
		pairs = append(pairs, scalarNode("failures"), intNode(h.Failures))
	}
	return mappingNode(pairs...)
}

func resourcesNode(svc mappedService) *yaml.Node {
	var pairs []*yaml.Node
	if svc.Memory != "" {
		pairs = append(pairs, scalarNode("memory"), scalarNode(svc.Memory))
	}
	if svc.CPU > 0 {
		pairs = append(pairs, scalarNode("cpu"), floatNode(svc.CPU))
	}
	return mappingNode(pairs...)
}

func envNode(keys []string) *yaml.Node {
	var pairs []*yaml.Node
	for _, k := range keys {
		pairs = append(pairs, scalarNode(k), mappingNode(scalarNode("secret"), boolNode(true)))
	}
	return mappingNode(pairs...)
}

func mappingNode(pairs ...*yaml.Node) *yaml.Node {
	return &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map", Content: pairs}
}

func stringSeqNode(values []string) *yaml.Node {
	n := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
	for _, v := range values {
		n.Content = append(n.Content, scalarNode(v))
	}
	return n
}

func scalarNode(s string) *yaml.Node {
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: s}
}

func intNode(i int) *yaml.Node {
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!int", Value: strconv.Itoa(i)}
}

func floatNode(f float64) *yaml.Node {
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!float", Value: strconv.FormatFloat(f, 'g', -1, 64)}
}

func boolNode(b bool) *yaml.Node {
	v := "false"
	if b {
		v = "true"
	}
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!bool", Value: v}
}
