package hostprovider

// PortRule is one inbound rule to create.
type PortRule struct {
	Port     int    `json:"port"`
	Protocol string `json:"protocol"`
	Source   string `json:"source"`
}

// Guidance is the exact manual procedure for one provider's firewall.
type Guidance struct {
	ID      ID         `json:"id"`
	Name    string     `json:"name"`
	Source  string     `json:"source"`
	DocsURL string     `json:"docs_url"`
	Ports   []PortRule `json:"ports"`
	Steps   []string   `json:"steps"`
	Note    string     `json:"note,omitempty"`
}

func webRules() []PortRule {
	return []PortRule{{80, "tcp", "0.0.0.0/0 and ::/0"}, {443, "tcp", "0.0.0.0/0 and ::/0"}}
}

// For returns the guidance for id, tagged with how it was detected.
// A bad override file is ignored here; LoadData reports it as a warning.
func For(id ID, source string) Guidance {
	data, _ := LoadData()
	g := Guidance{ID: id, Source: source, Ports: webRules()}
	if e, ok := data[string(id)]; ok {
		g.Name, g.DocsURL, g.Steps, g.Note = e.Name, e.DocsURL, e.Steps, e.Note
	}
	return g
}
