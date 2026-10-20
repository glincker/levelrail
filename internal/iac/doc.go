// Package iac turns multi-document YAML resource files into a plan of
// changes against a control plane and applies it through the ordinary REST
// API, so per-resource authorization applies to every item.
package iac

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// SchemaVersion is the only resource document version this package accepts.
const SchemaVersion = 1

// Kind names a resource document type.
type Kind string

// Resource kinds, in dependency order.
const (
	KindProject      Kind = "Project"
	KindEnvironment  Kind = "Environment"
	KindTag          Kind = "Tag"
	KindDatabase     Kind = "Database"
	KindApp          Kind = "App"
	KindDomain       Kind = "Domain"
	KindLoadBalancer Kind = "LoadBalancer"
	KindPipeline     Kind = "Pipeline"
	KindAlertRule    Kind = "AlertRule"
)

// Kinds lists every kind in the order changes are applied.
var Kinds = []Kind{KindProject, KindEnvironment, KindTag, KindDatabase, KindApp, KindDomain, KindLoadBalancer, KindPipeline, KindAlertRule}

func kindRank(k Kind) int {
	for i, c := range Kinds {
		if c == k {
			return i
		}
	}
	return len(Kinds)
}

// Source is one input file, or stdin, before parsing.
type Source struct {
	Name string
	Data []byte
}

// Issue is one validation problem, located by file, line and dotted path.
type Issue struct {
	File    string `json:"file,omitempty"`
	Path    string `json:"path,omitempty"`
	Line    int    `json:"line"`
	Message string `json:"message"`
}

func (i Issue) String() string {
	loc := i.File
	if i.Line > 0 {
		loc = fmt.Sprintf("%s:%d", i.File, i.Line)
	}
	msg := i.Message
	if i.Path != "" {
		msg += " (" + i.Path + ")"
	}
	if loc == "" {
		return msg
	}
	return loc + ": " + msg
}

// Document is one parsed YAML document with its envelope decoded.
type Document struct {
	File     string
	Line     int
	Kind     Kind
	Name     string
	Labels   map[string]string
	Root     *yaml.Node
	SpecNode *yaml.Node
}

// Labels with a meaning of their own.
const (
	LabelManagedBy = "managed-by"
	tagManagedPre  = "managed-by:"
)

var yamlLineRe = regexp.MustCompile(`line (\d+)`)

// ParseDocuments splits sources into documents and decodes each envelope.
// It reports YAML syntax errors and missing kind or name, nothing deeper;
// Validate does the schema and semantic checks.
func ParseDocuments(sources []Source) ([]*Document, []Issue) {
	var docs []*Document
	var issues []Issue
	for _, src := range sources {
		dec := yaml.NewDecoder(bytes.NewReader(src.Data))
		for {
			var root yaml.Node
			err := dec.Decode(&root)
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				line := 0
				if m := yamlLineRe.FindStringSubmatch(err.Error()); m != nil {
					line, _ = strconv.Atoi(m[1])
				}
				issues = append(issues, Issue{File: src.Name, Line: line, Message: "invalid yaml: " + strings.TrimPrefix(err.Error(), "yaml: ")})
				break
			}
			if root.Kind == 0 {
				continue
			}
			top := &root
			if top.Kind == yaml.DocumentNode && len(top.Content) > 0 {
				top = top.Content[0]
			}
			if top.Kind == yaml.ScalarNode && top.Tag == "!!null" {
				continue
			}
			docs = append(docs, decodeEnvelope(src.Name, top, &issues))
		}
	}
	return docs, issues
}

func decodeEnvelope(file string, top *yaml.Node, issues *[]Issue) *Document {
	d := &Document{File: file, Line: top.Line, Root: top}
	if top.Kind != yaml.MappingNode {
		*issues = append(*issues, Issue{File: file, Line: top.Line, Message: "a resource document must be a mapping"})
		return d
	}
	if kn := mapValue(top, "kind"); kn != nil {
		d.Kind = Kind(kn.Value)
	}
	if md := mapValue(top, "metadata"); md != nil && md.Kind == yaml.MappingNode {
		if n := mapValue(md, "name"); n != nil {
			d.Name = n.Value
		}
		if l := mapValue(md, "labels"); l != nil && l.Kind == yaml.MappingNode {
			d.Labels = map[string]string{}
			for i := 0; i+1 < len(l.Content); i += 2 {
				d.Labels[l.Content[i].Value] = l.Content[i+1].Value
			}
		}
	}
	d.SpecNode = mapValue(top, "spec")
	return d
}

func mapValue(n *yaml.Node, key string) *yaml.Node {
	if n == nil || n.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(n.Content); i += 2 {
		if n.Content[i].Value == key {
			return n.Content[i+1]
		}
	}
	return nil
}

// lineFor resolves a dotted path (numeric segments index sequences) to a
// source line, falling back to the deepest ancestor that exists.
func lineFor(root *yaml.Node, segs []string) int {
	n := root
	line := n.Line
	for _, s := range segs {
		if s == "" || n == nil {
			continue
		}
		var next *yaml.Node
		switch n.Kind {
		case yaml.MappingNode:
			for i := 0; i+1 < len(n.Content); i += 2 {
				if n.Content[i].Value == s {
					next = n.Content[i+1]
					line = n.Content[i].Line
					break
				}
			}
		case yaml.SequenceNode:
			if idx, err := strconv.Atoi(s); err == nil && idx >= 0 && idx < len(n.Content) {
				next = n.Content[idx]
				line = next.Line
			}
		}
		if next == nil {
			return line
		}
		n = next
	}
	return line
}
