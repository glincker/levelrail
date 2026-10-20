package iac

import "gopkg.in/yaml.v3"

func specString(d *Document, key string) string {
	if n := mapValue(d.SpecNode, key); n != nil {
		return n.Value
	}
	return ""
}

// FilterProject keeps the documents that belong to one project: the project
// itself, its environments, apps and databases, the domains, load balancers,
// pipelines and alert rules of those apps, and the tags those apps use.
func FilterProject(docs []*Document, project string) []*Document {
	apps := map[string]bool{}
	tags := map[string]bool{}
	for _, d := range docs {
		if d.Kind == KindApp && specString(d, "project") == project {
			apps[d.Name] = true
			if t := mapValue(d.SpecNode, "tags"); t != nil && t.Kind == yaml.SequenceNode {
				for _, n := range t.Content {
					tags[n.Value] = true
				}
			}
		}
	}
	var out []*Document
	for _, d := range docs {
		keep := false
		switch d.Kind {
		case KindProject:
			keep = d.Name == project
		case KindEnvironment, KindDatabase, KindApp:
			keep = specString(d, "project") == project
		case KindDomain, KindPipeline, KindAlertRule:
			keep = apps[specString(d, "app")]
		case KindLoadBalancer:
			keep = apps[d.Name]
		case KindTag:
			keep = tags[d.Name]
		}
		if keep {
			out = append(out, d)
		}
	}
	return out
}
