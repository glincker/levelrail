package mcptools

import (
	"encoding/json"
	"os"
	"slices"
	"sort"
	"testing"
)

func captureAllOutputFields(t *testing.T) map[string][]string {
	t.Helper()
	got := map[string][]string{}
	captureFullOutputFields = func(tool string, props []string) { got[tool] = props }
	t.Cleanup(func() { captureFullOutputFields = nil })
	listAll(t, Options{Mode: ModeFull, AllowWrite: true})
	return got
}

func TestOutputFieldsSnapshot(t *testing.T) {
	got := captureAllOutputFields(t)
	if os.Getenv("UPDATE_OUTPUT_FIELDS") != "" {
		b, err := json.MarshalIndent(got, "", " ")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile("output_fields.json", append(b, 0x0a), 0o600); err != nil {
			t.Fatal(err)
		}
		return
	}
	var frozen map[string][]string
	if err := json.Unmarshal(outputFieldsJSON, &frozen); err != nil {
		t.Fatal(err)
	}
	var missing, stale []string
	for tool := range got {
		if _, ok := frozen[tool]; !ok {
			missing = append(missing, tool)
		}
	}
	for tool, fields := range frozen {
		cur, ok := got[tool]
		if !ok {
			stale = append(stale, tool)
			continue
		}
		for _, f := range fields {
			if !slices.Contains(cur, f) {
				stale = append(stale, tool+"."+f)
			}
		}
	}
	sort.Strings(missing)
	sort.Strings(stale)
	if len(missing) > 0 || len(stale) > 0 {
		t.Errorf("output_fields.json is out of date (new tools: %v; removed fields: %v): run UPDATE_OUTPUT_FIELDS=1 go test ./internal/mcptools -run TestOutputFieldsSnapshot", missing, stale)
	}
}

func TestNewAPIFieldsDoNotGrowOutputSchema(t *testing.T) {
	tools := listAll(t, Options{Mode: ModeFull, AllowWrite: true})
	var frozen map[string][]string
	if err := json.Unmarshal(outputFieldsJSON, &frozen); err != nil {
		t.Fatal(err)
	}
	for _, tool := range tools {
		b, _ := json.Marshal(tool.OutputSchema)
		var m map[string]any
		_ = json.Unmarshal(b, &m)
		props, _ := m["properties"].(map[string]any)
		if items, ok := m["items"].(map[string]any); ok {
			props, _ = items["properties"].(map[string]any)
		}
		for name := range props {
			if !slices.Contains(frozen[tool.Name], name) {
				t.Errorf("%s advertises %q which is not in output_fields.json", tool.Name, name)
			}
		}
	}
}
