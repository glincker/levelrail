package mcptools

import (
	"encoding/json"
	"testing"
)

func TestProjectFields(t *testing.T) {
	tests := []struct {
		name   string
		in     string
		fields []string
		want   string
	}{
		{"object with items", `{"items":[{"name":"a","image":"x","port":1}],"next_cursor":"c"}`, []string{"name", "port"}, `{"items":[{"name":"a","port":1}],"next_cursor":"c"}`},
		{"top level array", `[{"id":"1","status":"ok","extra":true}]`, []string{"id"}, `[{"id":"1"}]`},
		{"unknown field drops to empty object", `[{"id":"1"}]`, []string{"nope"}, `[{}]`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := projectFields([]byte(tt.in), tt.fields)
			if !ok || string(got) != tt.want {
				t.Fatalf("got %s (ok=%v), want %s", got, ok, tt.want)
			}
		})
	}
	if _, ok := projectFields([]byte(`"scalar"`), []string{"a"}); ok {
		t.Fatal("a scalar result has nothing to project")
	}
}

func TestSplitFields(t *testing.T) {
	got := splitFields(" name , status,, ")
	if b, _ := json.Marshal(got); string(b) != `["name","status"]` {
		t.Fatalf("got %s", b)
	}
}

func TestProjectableToolsExistAndAcceptFields(t *testing.T) {
	byName := map[string]bool{}
	for _, tool := range listAll(t, Options{Mode: ModeFull, AllowWrite: true}) {
		byName[tool.Name] = true
		if _, ok := projectableTools[tool.Name]; !ok {
			continue
		}
		b, _ := json.Marshal(tool.InputSchema)
		if !json.Valid(b) || !containsFields(b) {
			t.Errorf("%s is projectable but its input schema has no fields argument", tool.Name)
		}
	}
	for name := range projectableTools {
		if !byName[name] {
			t.Errorf("projectable tool %q is not registered", name)
		}
	}
}

func containsFields(schema []byte) bool {
	var s struct {
		Properties map[string]json.RawMessage `json:"properties"`
	}
	if json.Unmarshal(schema, &s) != nil {
		return false
	}
	_, ok := s.Properties["fields"]
	return ok
}
