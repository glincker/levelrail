package projectarchive

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"sort"

	"gopkg.in/yaml.v3"
)

// FilterForProject keeps only the documents an import may create inside
// project: its own project, environments, apps and databases, plus the
// domains, cron jobs, alert rules, pipelines and load balancers of apps
// contained in the archive, and the tags those apps use. Everything else is
// returned in skipped as "Kind name" so the plan can show it.
func FilterForProject(files map[string][]byte, project string) (map[string][]byte, []string, error) {
	type parsed struct {
		file string
		node *yaml.Node
		kind string
		name string
	}
	names := make([]string, 0, len(files))
	for n := range files {
		names = append(names, n)
	}
	sort.Strings(names)
	var docs []parsed
	apps := map[string]bool{}
	tags := map[string]bool{}
	for _, f := range names {
		dec := yaml.NewDecoder(bytes.NewReader(files[f]))
		for {
			var doc yaml.Node
			err := dec.Decode(&doc)
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				return nil, nil, fmt.Errorf("%s: %w", f, err)
			}
			root := &doc
			if root.Kind == yaml.DocumentNode && len(root.Content) == 1 {
				root = root.Content[0]
			}
			p := parsed{file: f, node: &doc, kind: scalar(mapValue(root, "kind")), name: scalar(mapValue(mapValue(root, "metadata"), "name"))}
			if p.kind == "App" && scalar(mapValue(mapValue(root, "spec"), "project")) == project {
				apps[p.name] = true
				if t := mapValue(mapValue(root, "spec"), "tags"); t != nil && t.Kind == yaml.SequenceNode {
					for _, n := range t.Content {
						tags[n.Value] = true
					}
				}
			}
			docs = append(docs, p)
		}
	}
	bufs := map[string]*bytes.Buffer{}
	encs := map[string]*yaml.Encoder{}
	var skipped []string
	for _, d := range docs {
		root := d.node
		if root.Kind == yaml.DocumentNode && len(root.Content) == 1 {
			root = root.Content[0]
		}
		spec := mapValue(root, "spec")
		keep := false
		switch d.kind {
		case "Project":
			keep = d.name == project
		case "Environment", "Database", "App":
			keep = scalar(mapValue(spec, "project")) == project
		case "Domain", "CronJob", "AlertRule", "Pipeline":
			keep = apps[scalar(mapValue(spec, "app"))]
		case "LoadBalancer":
			keep = apps[d.name]
		case "Tag":
			keep = tags[d.name]
		}
		if !keep {
			skipped = append(skipped, d.kind+" "+d.name)
			continue
		}
		if encs[d.file] == nil {
			bufs[d.file] = &bytes.Buffer{}
			encs[d.file] = yaml.NewEncoder(bufs[d.file])
			encs[d.file].SetIndent(2)
		}
		if err := encs[d.file].Encode(d.node); err != nil {
			return nil, nil, err
		}
	}
	out := map[string][]byte{}
	for f, e := range encs {
		if err := e.Close(); err != nil {
			return nil, nil, err
		}
		out[f] = bufs[f].Bytes()
	}
	return out, skipped, nil
}

func scalar(n *yaml.Node) string {
	if n == nil {
		return ""
	}
	return n.Value
}
