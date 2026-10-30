package dashboards

import (
	"encoding/json"
	"errors"
	"testing"
)

func fleetDoc(typ, extra string) string {
	return `{"version":1,"name":"a","widgets":[{"id":"w","type":"` + typ + `","size":"S"` + extra + `}]}`
}

func TestParseImportSchema(t *testing.T) {
	tests := []struct {
		name, doc string
		ok        bool
	}{
		{"valid", `{"version":1,"name":"a","time_range":"6h","refresh_seconds":30,"widgets":[{"id":"w1","type":"log_tail","size":"L","target":{"kind":"app","name":"web"}}]}`, true},
		{"no widgets key", `{"version":1,"name":"a"}`, false},
		{"future version", `{"version":2,"name":"a","widgets":[]}`, false},
		{"bad size", `{"version":1,"name":"a","widgets":[{"id":"w1","type":"log_tail","size":"XL","target":{"kind":"app","name":"web"}}]}`, false},
		{"unknown widget type", `{"version":1,"name":"a","widgets":[{"id":"w1","type":"iframe","size":"S"}]}`, false},
		{"unknown field", `{"version":1,"name":"a","widgets":[],"script":"x"}`, false},
		{"bad refresh", `{"version":1,"name":"a","refresh_seconds":1,"widgets":[]}`, false},
		{"bad widget id", `{"version":1,"name":"a","widgets":[{"id":"Bad Id","type":"resource_counts","size":"S"}]}`, false},
		{"node widget on app", `{"version":1,"name":"a","widgets":[{"id":"w","type":"node_capacity","size":"S","target":{"kind":"app","name":"web"}}]}`, false},
		{"metric chart without metric", `{"version":1,"name":"a","widgets":[{"id":"w","type":"metric_chart","size":"S","target":{"kind":"app","name":"web"}}]}`, false},
		{"fleet_health no target", fleetDoc("fleet_health", ""), true},
		{"fleet_traffic no target", fleetDoc("fleet_traffic", ""), true},
		{"top_apps no target", fleetDoc("top_apps", ""), true},
		{"deploy_activity no target", fleetDoc("deploy_activity", ""), true},
		{"fleet_capacity no target", fleetDoc("fleet_capacity", ""), true},
		{"attention no target", fleetDoc("attention", ""), true},
		{"resource_counts with target", fleetDoc("resource_counts", `,"target":{"kind":"app","name":"web"}`), true},
		{"fleet_health with target", fleetDoc("fleet_health", `,"target":{"kind":"app","name":"web"}`), true},
		{"not json", `nope`, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Parse([]byte(tt.doc))
			if (err == nil) != tt.ok {
				t.Fatalf("err = %v, want ok=%v", err, tt.ok)
			}
			if err != nil && !errors.Is(err, ErrInvalid) {
				t.Fatalf("err %v does not wrap ErrInvalid", err)
			}
		})
	}
}

func TestTemplatesValidateOnceTargetsFilled(t *testing.T) {
	for _, tpl := range Templates() {
		doc := tpl.Doc
		for i := range doc.Widgets {
			if doc.Widgets[i].Target != nil {
				doc.Widgets[i].Target.Name = "web"
			}
		}
		raw, _ := json.Marshal(doc)
		if _, err := Parse(raw); err != nil {
			t.Errorf("template %s: %v", tpl.Key, err)
		}
	}
}

func TestLinkWidget(t *testing.T) {
	doc := func(w string) []byte {
		return []byte(`{"version":1,"name":"Links","widgets":[` + w + `]}`)
	}
	tests := []struct {
		name    string
		widget  string
		wantErr bool
	}{
		{"https link", `{"id":"runbook","type":"link","size":"S","title":"Runbook","url":"https://wiki.example.com/runbook"}`, false},
		{"missing title", `{"id":"a","type":"link","size":"S","url":"https://example.com"}`, true},
		{"missing url", `{"id":"a","type":"link","size":"S","title":"x"}`, true},
		{"javascript scheme", `{"id":"a","type":"link","size":"S","title":"x","url":"javascript:alert(1)"}`, true},
		{"data scheme", `{"id":"a","type":"link","size":"S","title":"x","url":"data:text/html,hi"}`, true},
		{"no host", `{"id":"a","type":"link","size":"S","title":"x","url":"https://"}`, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Parse(doc(tt.widget))
			if (err != nil) != tt.wantErr {
				t.Fatalf("Parse error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
