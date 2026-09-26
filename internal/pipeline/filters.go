package pipeline

import (
	"bytes"
	"errors"
	"fmt"

	"gopkg.in/yaml.v3"
)

// FilterView is the path filters and status reporting flag of a definition,
// as the editor and CLI show them.
type FilterView struct {
	Paths        []string `json:"paths"`
	PathsIgnore  []string `json:"paths_ignore"`
	ReportStatus bool     `json:"report_status"`
}

// FiltersOf reads the filters from the push trigger, falling back to the
// pull_request trigger when there is no push trigger.
func FiltersOf(def *Definition) FilterView {
	v := FilterView{Paths: []string{}, PathsIgnore: []string{}, ReportStatus: def.ReportsStatus()}
	switch {
	case def.On.Push != nil:
		v.Paths, v.PathsIgnore = nonNil(def.On.Push.Paths), nonNil(def.On.Push.PathsIgnore)
	case def.On.PullRequest != nil:
		v.Paths, v.PathsIgnore = nonNil(def.On.PullRequest.Paths), nonNil(def.On.PullRequest.PathsIgnore)
	}
	return v
}

func nonNil(l StringList) []string {
	if l == nil {
		return []string{}
	}
	return l
}

// ErrNoPathTrigger is returned by ApplyFilters when the pipeline has neither
// a push nor a pull_request trigger to attach path filters to.
var ErrNoPathTrigger = errors.New("pipeline: path filters need an on.push or on.pull_request trigger")

// ApplyFilters rewrites a pipeline's YAML so its push and pull_request
// triggers carry paths and paths_ignore, and sets report_status. A nil list
// or flag leaves that setting as it is; an empty non-nil list removes it.
func ApplyFilters(data []byte, paths, pathsIgnore []string, reportStatus *bool) ([]byte, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("pipeline: parse yaml: %w", err)
	}
	if doc.Kind != yaml.DocumentNode || len(doc.Content) != 1 || doc.Content[0].Kind != yaml.MappingNode {
		return nil, errors.New("pipeline: yaml root must be a mapping")
	}
	root := doc.Content[0]
	if paths != nil || pathsIgnore != nil {
		on := mapValue(root, "on")
		if on == nil || on.Kind != yaml.MappingNode {
			return nil, ErrNoPathTrigger
		}
		applied := false
		for _, key := range []string{"push", "pull_request"} {
			i := mapIndex(on, key)
			if i < 0 {
				continue
			}
			applied = true
			trig := on.Content[i+1]
			if trig.Kind != yaml.MappingNode {
				trig = &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
				on.Content[i+1] = trig
			}
			setList(trig, "paths", paths)
			setList(trig, "paths_ignore", pathsIgnore)
		}
		if !applied {
			return nil, ErrNoPathTrigger
		}
	}
	if reportStatus != nil {
		val := "false"
		if *reportStatus {
			val = "true"
		}
		setScalar(root, "report_status", &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!bool", Value: val})
	}
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(&doc); err != nil {
		return nil, fmt.Errorf("pipeline: encode yaml: %w", err)
	}
	if err := enc.Close(); err != nil {
		return nil, fmt.Errorf("pipeline: encode yaml: %w", err)
	}
	return buf.Bytes(), nil
}

func mapIndex(m *yaml.Node, key string) int {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			return i
		}
	}
	return -1
}

func mapValue(m *yaml.Node, key string) *yaml.Node {
	if i := mapIndex(m, key); i >= 0 {
		return m.Content[i+1]
	}
	return nil
}

func setScalar(m *yaml.Node, key string, val *yaml.Node) {
	if i := mapIndex(m, key); i >= 0 {
		m.Content[i+1] = val
		return
	}
	m.Content = append(m.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}, val)
}

func setList(m *yaml.Node, key string, list []string) {
	if list == nil {
		return
	}
	i := mapIndex(m, key)
	if len(list) == 0 {
		if i >= 0 {
			m.Content = append(m.Content[:i], m.Content[i+2:]...)
		}
		return
	}
	seq := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
	for _, g := range list {
		seq.Content = append(seq.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: g})
	}
	if i >= 0 {
		m.Content[i+1] = seq
		return
	}
	m.Content = append(m.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}, seq)
}
